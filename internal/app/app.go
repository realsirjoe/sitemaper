package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"sitemaper/internal/build"
	"sitemaper/internal/cache"
	"sitemaper/internal/model"
	"sitemaper/internal/query"
	"sitemaper/internal/robots"
)

type Config struct {
	Now func() time.Time
}

func Run(ctx context.Context, args []string, stdout, stderr io.Writer, cfg Config) int {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}

	fs := flag.NewFlagSet("sitemaper", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "sitemaper builds a sitemap tree for a target and makes it queryable with path-like selectors.")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Usage:")
		fmt.Fprintln(stderr, "  sitemaper --target <host[:port]|url> [--query <selector>] [options]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Examples:")
		fmt.Fprintln(stderr, "  sitemaper --target example.com --query ::")
		fmt.Fprintln(stderr, "  sitemaper --target http://127.0.0.1:8081 --query :: --cache-ttl 0")
		fmt.Fprintln(stderr, "  sitemaper --target example.com --query blog/sitemap.xml")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Flags:")
		fs.PrintDefaults()
	}
	var (
		target      string
		targetAlias string
		rawQuery    string
		cacheTTL    string
		cacheDir    string
		proxyURL    string
		concurrency int
		timeoutSec  int
	)
	fs.StringVar(&target, "target", "", "target host[:port]")
	fs.StringVar(&targetAlias, "domain", "", "alias for --target")
	fs.StringVar(&rawQuery, "query", "", "query selector or :: for root")
	fs.StringVar(&cacheTTL, "cache-ttl", "86400", "cache ttl in seconds, or 0 to disable cache (default 24h)")
	fs.StringVar(&cacheDir, "cache-dir", "", "cache directory")
	fs.StringVar(&proxyURL, "proxy", "", "proxy URL")
	fs.IntVar(&concurrency, "concurrency", 8, "parallel sitemap fetch concurrency")
	fs.IntVar(&timeoutSec, "timeout", 20, "http timeout in seconds")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if target == "" {
		target = targetAlias
	}
	if target == "" {
		fmt.Fprintln(stderr, "missing required --target")
		return 2
	}
	rootScheme := "https"
	rootAuthority := target
	rootURL := rootScheme + "://" + rootAuthority + "/sitemap.xml"

	if strings.Contains(target, "://") {
		if u, err := url.Parse(target); err == nil && u.Host != "" {
			if u.Scheme != "" {
				rootScheme = u.Scheme
			}
			rootAuthority = u.Host
			rootURL = rootScheme + "://" + rootAuthority + "/sitemap.xml"
		}
	}

	var ttl time.Duration
	cacheDisabled := false
	if cacheTTL == "0" {
		cacheDisabled = true
	} else {
		secs, err := strconv.ParseInt(cacheTTL, 10, 64)
		if err != nil || secs < 0 {
			fmt.Fprintf(stderr, "invalid --cache-ttl %q\n", cacheTTL)
			return 2
		}
		ttl = time.Duration(secs) * time.Second
	}
	if cacheDir == "" {
		var err error
		cacheDir, err = cache.DefaultDir()
		if err != nil {
			fmt.Fprintf(stderr, "cache dir error: %v\n", err)
			return 1
		}
	}

	now := cfg.Now()
	cachePath := cache.FilePath(cacheDir, rootScheme, rootAuthority)
	var root *model.Node
	if !cacheDisabled {
		if cf, fresh, err := cache.Load(cachePath, now); err == nil && fresh && cf.Tree != nil {
			root = cf.Tree
		} else if err == nil && !fresh {
			_ = os.Remove(cachePath)
		}
	}
	if root == nil {
		b, err := build.New(rootAuthority, build.Options{
			Concurrency: concurrency,
			ProxyURL:    proxyURL,
			Timeout:     time.Duration(timeoutSec) * time.Second,
		})
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		tree, err := b.Build(ctx, rootURL)
		if err != nil {
			if isRootSitemapMissing(err) {
				if fallbackTree, ferr := buildFromRobotsFallback(ctx, rootScheme, rootAuthority, proxyURL, time.Duration(timeoutSec)*time.Second, b); ferr == nil {
					tree = fallbackTree
					err = nil
				} else {
					err = fmt.Errorf("root sitemap failed (%v); robots fallback failed (%v)", err, ferr)
				}
			}
		}
		if err != nil {
			if strings.Contains(err.Error(), "server gave HTTP response to HTTPS client") && rootScheme == "https" {
				fmt.Fprintf(stderr, "build error: %v\nhint: target appears to be HTTP; try --target http://%s\n", err, rootAuthority)
				return 1
			}
			fmt.Fprintf(stderr, "build error: %v\n", err)
			return 1
		}
		root = tree
		if !cacheDisabled {
			if err := cache.Save(cachePath, rootURL, rootAuthority, ttl, tree, now); err != nil {
				fmt.Fprintf(stderr, "cache write warning: %v\n", err)
			}
		}
	}

	n, err := query.Resolve(root, rawQuery)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			fmt.Fprintln(stderr, "query path not found")
			return 1
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	for _, line := range n.ChildSelectors() {
		fmt.Fprintln(stdout, line)
	}
	return 0
}

func isRootSitemapMissing(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	// Treat forbidden responses like missing to allow robots.txt fallback.
	return strings.Contains(s, "http 404") || strings.Contains(s, "http 410") || strings.Contains(s, "http 403")
}

func buildFromRobotsFallback(ctx context.Context, rootScheme, rootAuthority, proxyURL string, timeout time.Duration, b *build.Builder) (*model.Node, error) {
	robotsURL := rootScheme + "://" + rootAuthority + "/robots.txt"
	body, err := fetchText(ctx, robotsURL, proxyURL, timeout)
	if err != nil {
		return nil, err
	}
	sitemaps := robots.ParseSitemaps(body)
	if len(sitemaps) == 0 {
		return nil, fmt.Errorf("no sitemap URLs found in robots.txt")
	}
	// If multiple sitemaps are declared, use the first one.
	return b.Build(ctx, sitemaps[0])
}

func fetchText(ctx context.Context, rawURL, proxyURL string, timeout time.Duration) ([]byte, error) {
	tr := &http.Transport{}
	if proxyURL != "" {
		pu, err := url.Parse(proxyURL)
		if err != nil {
			return nil, err
		}
		tr.Proxy = http.ProxyURL(pu)
	}
	client := &http.Client{Transport: tr, Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %d for %s", resp.StatusCode, rawURL)
	}
	return io.ReadAll(resp.Body)
}
