package query

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"sitemaper/internal/model"
)

var ErrNotFound = errors.New("query path not found")

type Segment struct {
	Raw            string
	FullyQualified bool
}

func Parse(raw string) ([]Segment, bool, error) {
	if raw == "" || raw == "::" {
		return nil, true, nil
	}
	parts := strings.Split(raw, "::")
	if len(parts) > 0 && parts[0] == "" {
		// Allow leading :: to mean "root then".
		parts = parts[1:]
	}
	segs := make([]Segment, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			return nil, false, fmt.Errorf("invalid empty query segment")
		}
		s := Segment{Raw: p}
		if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
			u, err := url.Parse(p)
			if err != nil || u.Scheme == "" || u.Host == "" {
				return nil, false, fmt.Errorf("invalid fully qualified segment %q", p)
			}
			s.FullyQualified = true
		}
		segs = append(segs, s)
	}
	return segs, false, nil
}

func Resolve(root *model.Node, rawQuery string) (*model.Node, error) {
	segs, isRoot, err := Parse(rawQuery)
	if err != nil {
		return nil, err
	}
	if isRoot {
		return root, nil
	}
	rootAuthority := rootAuthorityFromNode(root)
	cur := root
	for _, seg := range segs {
		lookup := seg.Raw
		if seg.FullyQualified && rootAuthority != "" {
			if sel, err := model.CanonicalSelectorForURL(seg.Raw, rootAuthority); err == nil {
				lookup = sel
			}
		}
		next := findChild(cur, lookup)
		if next == nil {
			return nil, ErrNotFound
		}
		cur = next
	}
	return cur, nil
}

func findChild(n *model.Node, selector string) *model.Node {
	for _, ch := range n.Children {
		if ch.Selector == selector {
			return ch
		}
	}
	return nil
}

func rootAuthorityFromNode(n *model.Node) string {
	if n == nil || n.URL == "" {
		return ""
	}
	u, err := url.Parse(n.URL)
	if err != nil {
		return ""
	}
	return u.Host
}
