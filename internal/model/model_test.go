package model

import "testing"

func TestCanonicalSitemapURL_Normalization(t *testing.T) {
	got, err := CanonicalSitemapURL("https://example.com/a/#frag")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.com/a" {
		t.Fatalf("got %q", got)
	}

	got, err = CanonicalSitemapURL("https://example.com/a/?x=1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.com/a?x=1" {
		t.Fatalf("got %q", got)
	}
}

func TestCanonicalSelectorForURL(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		root   string
		expect string
	}{
		{"local path", "https://example.com/blog/sitemap.xml", "example.com", "blog/sitemap.xml"},
		{"local root path", "https://example.com/", "example.com", "/"},
		{"local query", "https://example.com/sitemap.xml?v=1", "example.com", "sitemap.xml?v=1"},
		{"cross authority", "https://other.example.com/x.xml", "example.com", "https://other.example.com/x.xml"},
		{"trailing slash normalized", "https://example.com/a/", "example.com", "a"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CanonicalSelectorForURL(tc.raw, tc.root)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.expect {
				t.Fatalf("got %q want %q", got, tc.expect)
			}
		})
	}
}
