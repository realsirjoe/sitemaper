# sitemaper

sitemaper is a Go CLI that builds a sitemap tree for a target authority and lets you query it with path-like selectors. It fetches and parses sitemap XML documents only; it never crawls page URLs.

See `SPEC.md` for the full v1 specification.

**Features**
- Recursively follows sitemap indexes starting from `/sitemap.xml`.
- Bounded parallel fetches with configurable concurrency and timeout.
- Optional HTTP proxy for sitemap requests.
- On-disk JSON cache with TTL (reuse within TTL, rebuild after expiry).
- `ls`-style query output with deterministic sorting.
- Robots.txt fallback when `/sitemap.xml` is missing (uses the first `Sitemap:` entry).

**Non-Goals**
- Crawling or fetching page URLs found in `urlset` entries.
- JavaScript rendering or site auditing.

**Install / Build**
```bash
make build
```

Or build directly:
```bash
go build -o sitemaper ./cmd/sitemaper
```

**Usage**
```bash
sitemaper --target <host[:port]|url> [--query <selector>] [options]
```

Examples:
```bash
sitemaper --target example.com --query ::
sitemaper --target example.com --query blog/sitemap.xml
sitemaper --target http://127.0.0.1:8081 --query :: --cache-ttl 0
```

Flags:
- `--target` required target `host[:port]` or URL (scheme is honored if present).
- `--domain` alias for `--target`.
- `--query` selector or `::` for root (default: root).
- `--cache-ttl` cache TTL in seconds (default `86400`, `0` disables cache for the run).
- `--cache-dir` cache directory (default: OS user cache dir + `sitemaper`).
- `--proxy` proxy URL (e.g. `http://127.0.0.1:8080`).
- `--concurrency` parallel fetch limit (default `8`).
- `--timeout` HTTP timeout in seconds (default `60`).

**Query Syntax**
sitemaper treats the sitemap tree like a filesystem and uses `::` as the path separator.

Selectors:
- Root: `::`
- Local segment: `foo.xml` or `nested/index.xml`
- Fully qualified segment: `https://example.com/foo.xml`
- Chains: `foo.xml::nested/leaf.xml`

Resolution rules:
- Empty query or `::` means the root.
- For nodes on the root authority, local segments are allowed and preferred.
- For nodes on other authorities, segments must be fully qualified (`http://` or `https://`).
- Output selectors are canonical and can be reused as input.

Normalization rules (v1):
- Trailing slashes are normalized (`/a` and `/a/` are the same).
- Fragments are ignored.
- Query strings are preserved and part of identity.
- Scheme and authority are part of identity.

**Output**
Queries return an `ls`-style listing of immediate children, one selector per line, sorted alphabetically. Leaf nodes produce an empty listing.

**Cache**
- Cache is stored as JSON in `~/.cache/sitemaper` (platform-dependent).
- Cache filenames are derived from the root scheme and authority.
- Cache is written only after a successful full build.
- `--cache-ttl 0` disables cache read/write for that run.

**Robots.txt Fallback**
If the root `/sitemap.xml` returns 403, 404, or 410, sitemaper will try `https://<authority>/robots.txt` and use the first `Sitemap:` entry found.

**Test Fixtures**
Run local fixture servers:
```bash
go run ./cmd/testfixtures
```

The command prints URLs for multiple test sites (no-sitemap, root-only, deep nested, robots-only, malformed).

**Development**
```bash
make test
make fmt
```

**Limitations**
- No `.xml.gz` or gzip-compressed sitemap support yet.
- No automatic HTTP fallback when HTTPS fails (you must pass `http://` explicitly).
- No recursive output mode; listings are single-level.
