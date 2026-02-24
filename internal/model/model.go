package model

import (
	"net/url"
	"sort"
	"strings"
)

type NodeKind string

const (
	NodeIndex  NodeKind = "index"
	NodeURLSet NodeKind = "urlset"
	NodeURL    NodeKind = "url"
)

type Node struct {
	Kind      NodeKind `json:"kind"`
	URL       string   `json:"url,omitempty"`
	Loc       string   `json:"loc,omitempty"`
	Selector  string   `json:"selector,omitempty"`
	Error     string   `json:"error,omitempty"`
	FetchedAt string   `json:"fetched_at,omitempty"`
	Children  []*Node  `json:"children,omitempty"`
}

func (n *Node) ChildSelectors() []string {
	out := make([]string, 0, len(n.Children))
	for _, ch := range n.Children {
		out = append(out, ch.Selector)
	}
	sort.Strings(out)
	return out
}

func CanonicalSitemapURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	u.Fragment = ""
	u.Path = normalizePath(u.Path)
	return u.String(), nil
}

func CanonicalPageURL(raw string) (string, error) {
	// Same v1 normalization rules as sitemap URLs.
	return CanonicalSitemapURL(raw)
}

func CanonicalSelectorForURL(rawURL, rootAuthority string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	u.Fragment = ""
	u.Path = normalizePath(u.Path)

	if equalAuthority(u.Host, rootAuthority) {
		p := strings.TrimPrefix(u.EscapedPath(), "/")
		if p == "" {
			p = "/"
		}
		if u.RawQuery != "" {
			p += "?" + u.RawQuery
		}
		return p, nil
	}
	return u.String(), nil
}

func normalizePath(p string) string {
	if p == "" {
		return "/"
	}
	if p != "/" && strings.HasSuffix(p, "/") {
		return strings.TrimRight(p, "/")
	}
	return p
}

func equalAuthority(a, b string) bool {
	return strings.EqualFold(a, b)
}
