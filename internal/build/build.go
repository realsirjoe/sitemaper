package build

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"sitemaper/internal/model"
	"sitemaper/internal/sitemapxml"
)

type Options struct {
	Concurrency int
	ProxyURL    string
	Timeout     time.Duration
}

type Builder struct {
	rootAuthority string
	client        *http.Client
	sem           chan struct{}

	mu      sync.Mutex
	visited map[string]*memo
}

type memo struct {
	done chan struct{}
	node *model.Node
	err  error
}

func New(rootAuthority string, opts Options) (*Builder, error) {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 8
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 20 * time.Second
	}
	tr := &http.Transport{
		DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
	}
	if opts.ProxyURL != "" {
		pu, err := url.Parse(opts.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy: %w", err)
		}
		tr.Proxy = http.ProxyURL(pu)
	}
	return &Builder{
		rootAuthority: rootAuthority,
		client:        &http.Client{Transport: tr, Timeout: opts.Timeout},
		sem:           make(chan struct{}, opts.Concurrency),
		visited:       map[string]*memo{},
	}, nil
}

func (b *Builder) Build(ctx context.Context, rootURL string) (*model.Node, error) {
	return b.buildSitemap(ctx, rootURL)
}

func (b *Builder) buildSitemap(ctx context.Context, rawURL string) (*model.Node, error) {
	canon, err := model.CanonicalSitemapURL(rawURL)
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	if m, ok := b.visited[canon]; ok {
		b.mu.Unlock()
		<-m.done
		return m.node, m.err
	}
	m := &memo{done: make(chan struct{})}
	b.visited[canon] = m
	b.mu.Unlock()

	node, err := b.fetchAndParse(ctx, canon)
	m.node, m.err = node, err
	close(m.done)
	return node, err
}

func (b *Builder) fetchAndParse(ctx context.Context, sitemapURL string) (*model.Node, error) {
	body, err := b.fetch(ctx, sitemapURL)
	if err != nil {
		n := &model.Node{URL: sitemapURL, Error: err.Error()}
		if sel, e := model.CanonicalSelectorForURL(sitemapURL, b.rootAuthority); e == nil {
			n.Selector = sel
		}
		return n, err
	}
	parsed, err := sitemapxml.Parse(body)
	if err != nil {
		n := &model.Node{URL: sitemapURL, Error: err.Error()}
		if sel, e := model.CanonicalSelectorForURL(sitemapURL, b.rootAuthority); e == nil {
			n.Selector = sel
		}
		return n, err
	}

	n := &model.Node{
		URL:       sitemapURL,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}
	n.Selector, _ = model.CanonicalSelectorForURL(sitemapURL, b.rootAuthority)

	switch parsed.Kind {
	case sitemapxml.KindIndex:
		n.Kind = model.NodeIndex
		children := make([]*model.Node, len(parsed.Locs))
		errs := make(chan error, len(parsed.Locs))
		var wg sync.WaitGroup
		for i, loc := range parsed.Locs {
			i, loc := i, loc
			wg.Add(1)
			go func() {
				defer wg.Done()
				child, cerr := b.buildSitemap(ctx, loc)
				if cerr != nil {
					errs <- cerr
				}
				children[i] = child
			}()
		}
		wg.Wait()
		close(errs)
		for _, ch := range children {
			if ch != nil {
				n.Children = append(n.Children, ch)
			}
		}
		// Return first child error if any while still preserving tree.
		for e := range errs {
			if e != nil {
				if n.Error == "" {
					n.Error = e.Error()
				}
				return n, e
			}
		}
	case sitemapxml.KindURLSet:
		n.Kind = model.NodeURLSet
		for _, loc := range parsed.Locs {
			canonLoc, cerr := model.CanonicalPageURL(loc)
			if cerr != nil {
				canonLoc = loc
			}
			ch := &model.Node{Kind: model.NodeURL, Loc: canonLoc}
			ch.Selector, _ = model.CanonicalSelectorForURL(canonLoc, b.rootAuthority)
			n.Children = append(n.Children, ch)
		}
	default:
		return nil, fmt.Errorf("unsupported parsed kind %q", parsed.Kind)
	}
	return n, nil
}

func (b *Builder) fetch(ctx context.Context, rawURL string) ([]byte, error) {
	select {
	case b.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-b.sem }()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %d for %s", resp.StatusCode, rawURL)
	}
	return io.ReadAll(resp.Body)
}
