package app

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sitemaper/internal/testserver"
)

func TestNoSitemap(t *testing.T) {
	suite := testserver.NewSuite()
	defer suite.Close()

	var out, errb bytes.Buffer
	code := Run(context.Background(), []string{
		"--target", suite.NoSitemap.URL,
		"--cache-ttl", "0",
	}, &out, &errb, Config{Now: func() time.Time { return time.Unix(0, 0) }})
	if code == 0 {
		t.Fatalf("expected failure, stdout=%q stderr=%q", out.String(), errb.String())
	}
}

func TestDeepNestedCrossAuthorityQueryAndNoPageFetch(t *testing.T) {
	suite := testserver.NewSuite()
	defer suite.Close()

	cacheDir := t.TempDir()
	now := time.Date(2026, 2, 23, 12, 0, 0, 0, time.UTC)
	run := func(args ...string) (int, string, string) {
		var out, errb bytes.Buffer
		all := append([]string{"--target", suite.DeepA.URL, "--cache-dir", cacheDir, "--cache-ttl", "3600"}, args...)
		code := Run(context.Background(), all, &out, &errb, Config{Now: func() time.Time { return now }})
		return code, strings.TrimSpace(out.String()), errb.String()
	}

	code, out, errStr := run("--query", "::")
	if code != 0 {
		t.Fatalf("root query failed: %s", errStr)
	}
	lines := splitLines(out)
	if len(lines) != 2 {
		t.Fatalf("root listing lines=%v", lines)
	}
	if !(strings.HasPrefix(lines[0], "http://") && lines[1] == "nested/index.xml") {
		t.Fatalf("unexpected root listing %v", lines)
	}
	if !strings.Contains(lines[0], "/x/child.xml") {
		t.Fatalf("missing cross-authority child in %v", lines)
	}

	code, out, errStr = run("--query", "nested/index.xml::nested/leaf.xml")
	if code != 0 {
		t.Fatalf("nested query failed: %s", errStr)
	}
	if got := splitLines(out); len(got) != 2 || got[0] != "page/a" || got[1] != "page/b" {
		t.Fatalf("unexpected nested leaf listing %v", got)
	}

	// Same-authority selectors should work in full URL form anywhere in the chain.
	fullIdx := fmt.Sprintf("%s/nested/index.xml", suite.DeepA.URL)
	fullLeaf := fmt.Sprintf("%s/nested/leaf.xml", suite.DeepA.URL)

	code, out, errStr = run("--query", fullIdx)
	if code != 0 {
		t.Fatalf("full local first-segment query failed: %s", errStr)
	}
	if got := splitLines(out); len(got) != 1 || got[0] != "nested/leaf.xml" {
		t.Fatalf("unexpected full local first-segment listing %v", got)
	}

	code, out, errStr = run("--query", "nested/index.xml::"+fullLeaf)
	if code != 0 {
		t.Fatalf("mixed chain (local then full local) failed: %s", errStr)
	}
	if got := splitLines(out); len(got) != 2 || got[0] != "page/a" || got[1] != "page/b" {
		t.Fatalf("unexpected mixed-chain listing %v", got)
	}

	code, out, errStr = run("--query", fullIdx+"::nested/leaf.xml")
	if code != 0 {
		t.Fatalf("mixed chain (full local then local) failed: %s", errStr)
	}
	if got := splitLines(out); len(got) != 2 || got[0] != "page/a" || got[1] != "page/b" {
		t.Fatalf("unexpected mixed-chain listing %v", got)
	}

	code, out, errStr = run("--query", fullIdx+"::"+fullLeaf)
	if code != 0 {
		t.Fatalf("full local chain failed: %s", errStr)
	}
	if got := splitLines(out); len(got) != 2 || got[0] != "page/a" || got[1] != "page/b" {
		t.Fatalf("unexpected full-local-chain listing %v", got)
	}

	crossSelector := fmt.Sprintf("%s/x/child.xml", suite.DeepB.URL)
	code, out, errStr = run("--query", crossSelector)
	if code != 0 {
		t.Fatalf("cross query failed: %s", errStr)
	}
	gotCross := splitLines(out)
	if len(gotCross) != 2 || !strings.Contains(gotCross[0], "/pages/a") || !strings.Contains(gotCross[1], "/pages/z") {
		t.Fatalf("unexpected cross listing %v", gotCross)
	}

	// Cross-authority nodes must not allow simplified selectors (root authority minimization only).
	code, _, _ = run("--query", "x/child.xml")
	if code == 0 {
		t.Fatalf("expected simplified cross-authority selector to fail")
	}
	code, _, _ = run("--query", crossSelector+"::pages/a")
	if code == 0 {
		t.Fatalf("expected simplified cross-authority child selector to fail")
	}

	code, out, errStr = run("--query", "nested/index.xml::nested/leaf.xml::page/a")
	if code != 0 {
		t.Fatalf("leaf url query failed: %s", errStr)
	}
	if out != "" {
		t.Fatalf("expected empty listing for leaf, got %q", out)
	}

	if suite.PageHitsDeepA.Load() != 0 || suite.PageHitsDeepB.Load() != 0 {
		t.Fatalf("page URLs were fetched: A=%d B=%d", suite.PageHitsDeepA.Load(), suite.PageHitsDeepB.Load())
	}
}

func TestCacheReuseExpiryAndNoCacheFlag(t *testing.T) {
	var sitemapHits atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sitemap.xml" {
			http.NotFound(w, r)
			return
		}
		sitemapHits.Add(1)
		fmt.Fprint(w, `<?xml version="1.0"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.test/a</loc></url>
</urlset>`)
	}))
	defer srv.Close()

	target := srv.URL
	cacheDir := t.TempDir()
	now := time.Date(2026, 2, 23, 12, 0, 0, 0, time.UTC)

	runAt := func(ts time.Time, extra ...string) (int, string, string) {
		args := []string{"--target", target, "--cache-dir", cacheDir}
		args = append(args, extra...)
		var out, errb bytes.Buffer
		code := Run(context.Background(), args, &out, &errb, Config{Now: func() time.Time { return ts }})
		return code, out.String(), errb.String()
	}

	if code, _, errStr := runAt(now, "--cache-ttl", "3600", "--query", "::"); code != 0 {
		t.Fatalf("run1 failed: %s", errStr)
	}
	if sitemapHits.Load() != 1 {
		t.Fatalf("hits after run1 = %d", sitemapHits.Load())
	}

	if code, _, errStr := runAt(now.Add(30*time.Minute), "--cache-ttl", "3600", "--query", "::"); code != 0 {
		t.Fatalf("run2 failed: %s", errStr)
	}
	if sitemapHits.Load() != 1 {
		t.Fatalf("expected cache reuse, hits=%d", sitemapHits.Load())
	}

	if code, _, errStr := runAt(now.Add(2*time.Hour), "--cache-ttl", "3600", "--query", "::"); code != 0 {
		t.Fatalf("run3 failed: %s", errStr)
	}
	if sitemapHits.Load() != 2 {
		t.Fatalf("expected refetch after expiry, hits=%d", sitemapHits.Load())
	}

	// No-cache mode should not write cache.
	cacheDir2 := t.TempDir()
	if code, _, errStr := func() (int, string, string) {
		var out, errb bytes.Buffer
		code := Run(context.Background(), []string{
			"--target", target,
			"--cache-dir", cacheDir2,
			"--cache-ttl", "0",
			"--query", "::",
		}, &out, &errb, Config{Now: func() time.Time { return now }})
		return code, out.String(), errb.String()
	}(); code != 0 {
		t.Fatalf("no-cache run failed: %s", errStr)
	}
	files, _ := filepath.Glob(filepath.Join(cacheDir2, "*.sitemaper.json"))
	if len(files) != 0 {
		t.Fatalf("expected no cache file in no-cache mode, got %v", files)
	}
}

func TestRootOnlyFixtureAlphabeticalListing(t *testing.T) {
	suite := testserver.NewSuite()
	defer suite.Close()

	var out, errb bytes.Buffer
	code := Run(context.Background(), []string{
		"--target", suite.RootOnly.URL,
		"--cache-ttl", "0",
		"--query", "::",
	}, &out, &errb, Config{Now: func() time.Time { return time.Unix(0, 0) }})
	if code != 0 {
		t.Fatalf("run failed: %s", errb.String())
	}
	got := splitLines(out.String())
	if len(got) != 2 || got[0] != "page-a" || got[1] != "page-b" {
		t.Fatalf("unexpected listing: %v", got)
	}
}

func TestRobotsTxtFallbackWhenRootSitemapMissing(t *testing.T) {
	suite := testserver.NewSuite()
	defer suite.Close()

	var out, errb bytes.Buffer
	code := Run(context.Background(), []string{
		"--target", suite.RobotsOnly.URL,
		"--cache-ttl", "0",
		"--query", "::",
	}, &out, &errb, Config{Now: func() time.Time { return time.Unix(0, 0) }})
	if code != 0 {
		t.Fatalf("robots fallback root query failed: %s", errb.String())
	}
	got := splitLines(out.String())
	if len(got) != 2 || got[0] != "products/a" || got[1] != "products/b" {
		t.Fatalf("unexpected root listing from robots fallback: %v", got)
	}

	out.Reset()
	errb.Reset()
	code = Run(context.Background(), []string{
		"--target", suite.RobotsOnly.URL,
		"--cache-ttl", "0",
		"--query", "products/a",
	}, &out, &errb, Config{Now: func() time.Time { return time.Unix(0, 0) }})
	if code != 0 {
		t.Fatalf("robots fallback child query failed: %s", errb.String())
	}
	got = splitLines(out.String())
	if len(got) != 0 {
		t.Fatalf("unexpected robots sitemap listing: %v", got)
	}
}

func TestMalformedFixtureFails(t *testing.T) {
	suite := testserver.NewSuite()
	defer suite.Close()

	var out, errb bytes.Buffer
	code := Run(context.Background(), []string{
		"--target", suite.Malformed.URL,
		"--cache-ttl", "0",
	}, &out, &errb, Config{Now: func() time.Time { return time.Unix(0, 0) }})
	if code == 0 {
		t.Fatalf("expected malformed fixture to fail")
	}
}

func splitLines(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return lines
}
