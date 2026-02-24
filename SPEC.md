# Sitemaper Specification

## Overview

`sitemaper` is a CLI tool written in Go that builds a complete sitemap tree for a domain/authority by starting from:

- `https://<authority>/sitemap.xml` (default)

It recursively follows sitemap index files to sub-sitemaps, downloads all referenced sitemap XML files, and builds a local tree representation of the sitemap structure.

The tool does **not** visit or crawl actual webpage URLs found in leaf sitemaps. It only downloads and parses sitemap XML documents.

## Goals

- Build the full sitemap tree for a domain/authority.
- Fetch sitemaps in parallel to improve speed.
- Support optional HTTP/HTTPS proxy usage.
- Cache the built tree locally with configurable cache TTL.
- On re-run, reuse cached tree when valid (avoid network requests).
- Return/query tree contents using a path-style interface similar to directory listing (`ls`-like).
- Keep the implementation easy to change and extend in future versions.

## Non-Goals

- Crawling webpage content.
- Rendering JavaScript pages.
- Discovering URLs outside sitemap XML references.
- Website auditing/SEO validation (unless added later).

## Core Behavior

### 1. Input

The tool accepts:

- A domain/authority (`host[:port]`) (required)
- A query path into the sitemap tree (optional)
- Cache controls (optional)
- Proxy configuration (optional)

Example intent:

- Build/load sitemap tree for `example.com`
- Query a subtree path and list its children

### 2. Sitemap Discovery

1. Start from `<scheme>://<authority>/sitemap.xml` (default scheme `https`)
2. Download and parse XML
3. If the file is a sitemap index (`<sitemapindex>`):
   - Extract child sitemap URLs
   - Fetch them recursively
4. If the file is a URL set (`<urlset>`):
   - Treat as a leaf sitemap
   - Store contained URLs as leaf entries
   - Do **not** request page URLs in `<loc>` (they are stored as final leaf entries only)

### 3. Parallel Fetching

- Sitemap fetches should run in parallel for performance.
- Concurrency should be bounded (worker pool or semaphore) to avoid overwhelming remote servers or local resources.
- Failed sitemap fetches should be captured as errors on their node (or in a global error list), without crashing the entire run when possible.

### 4. Tree Model

Represent the sitemap structure as a tree (logical parent/child query structure):

- Root node: root sitemap for the authority/target specified to the command
- Intermediate nodes: sitemap index / child sitemap references
- Leaf sitemap nodes: URL sitemaps (`urlset`) containing URL entries
- Final leaf nodes: non-sitemap page URLs found inside `urlset` entries

Each node should store enough metadata to support:

- Original sitemap URL
- Node type (`index`, `urlset`, `url`)
- Child nodes (for sitemap/index relationships)
- Fetch/parsing status and errors
- Timestamps (fetch time / cache time)

Implementation note (v1): if the same sitemap URL is referenced from multiple parents, query/listing behavior should preserve each parent-child edge in the logical tree. Implementations may still deduplicate fetch/parsing work internally by URL.

### 5. Query / Output Behavior

After the tree is built (or loaded from cache), `sitemaper` returns the result for the requested query path.

The query behaves like a tree path:

- `::` = root of the sitemap tree
- `foo.xml` (example local segment) = inspect a specific local-authority sitemap node
- Querying a node lists its immediate children (similar to `ls`)

Output should be a directory-style listing of what is inside the queried node, not a recursive dump by default.
Listings should be ordered alphabetically for deterministic output.
Default v1 plain-text listing format should print one canonical child selector per line (additional formats/columns may be added later).

### Query Path Resolution Rules

- If no query path is provided, return the root directory listing (top-level sitemaps/sub-sitemaps).
- `::` by itself means the root of the sitemap tree.
- Queries may be a single node selector or a chain of node selectors joined by `::`.
- Query traversal is edge-based and filesystem-like: each `::segment` is resolved relative to the current node by matching one of that node's immediate children (similar to traversing one directory level at a time).
- For sitemap paths on the same domain/authority (`host[:port]`) as the root CLI command target (the first authority / root authority), the query may omit the authority and use a shortened local path segment (for convenience).
- For sitemap paths on a different authority than the root CLI command target (the first authority / root authority), that segment must use a fully qualified URL form (including scheme and authority) to avoid ambiguity.
- Cross-authority child sitemap nodes are allowed in the logical tree and are queried as mounted subtrees (filesystem analogy), but the selector for that segment must use the fully qualified URL form.
- Fully qualified segments are still accepted for local-authority sitemap nodes.
- Querying any node with no children returns a successful empty listing (like an empty directory), not a "not found" error.

### Query Grammar (Canonical)

To avoid ambiguity (because sitemap paths naturally contain `/`), `sitemaper` uses `::` as the tree path separator between sitemap nodes, and requires a URL scheme for fully qualified node selectors.

- Root query: `::`
- Single local-authority node query: `<path>` (e.g. `products/sitemap.xml` or `foo.xml`)
- Single fully qualified node query: `<scheme>://<authority>/<path>` (e.g. `https://example.com/products/sitemap.xml`)
- Multi-node query chain: `<segment>::<segment>[::<segment>...]`

Parsing rules:

- If no query is provided, treat it as `::`
- `::` by itself means "root of the sitemap tree"
- Otherwise, split the query on `::` into one or more node-selector segments
- v1 parser splits the raw query string on literal `::` and does not define a separate escape syntax for the separator
- A segment may be:
  - local path form: `<path>` (same authority as the root CLI command target / root authority)
  - fully qualified form: `<scheme>://<authority>/<path>`
- Fully qualified segments must include scheme (`http://` or `https://`) to avoid ambiguity with local path segments
- If a fully qualified segment contains `::` inside its query string, that value must be percent-encoded in the input (`%3A%3A`) so it is not interpreted as a path separator
- `authority` means `host[:port]` (important for local multi-port test fixtures)
- Segments for a different authority must use the fully qualified form
- Fully qualified segments may be minimized to local form only when they refer to the same authority as the root CLI command target (the first authority / root authority), not the previous segment in the query chain
- For multi-segment queries, resolution proceeds left-to-right, and each segment must match an immediate child of the node resolved by the previous segment

Implementation note:

- Full URL node selectors (e.g. `https://otherdomain.com/products/sitemap.xml`) are already part of the canonical grammar above and should be normalized/stored internally in the same canonical node form used by the query parser

### Query Examples (conceptual)

- Query `::` -> list top-level sitemap children
- Query `foo.xml` -> query local sitemap node
- Query `https://example.com/foo.xml` -> equivalent to `foo.xml` when `example.com` is the local authority
- Query `https://example.com/foo.xml::https://example.com/other/bar.xml` -> equivalent to `foo.xml::other/bar.xml`
- Query `https://example.com/foo.xml::https://example2.com/foo.xml` -> may only be minimized to `foo.xml::https://example2.com/foo.xml`
- No query provided -> same as querying `::` (root listing)
- Cross-authority child sitemap in a chain -> segment must be fully qualified with scheme (e.g. `https://otherdomain.com/nested/sitemap.xml`)
- Querying a leaf `url` node (or any node with no children) -> successful empty listing
- When compressed sitemap support is enabled in a future version, querying a `.xml.gz` sitemap node lists its parsed child sitemap/url entries normally (logical XML contents), not compressed byte data

Path mapping and query selector normalization must be defined and kept stable so the same sitemap node always maps to the same query selector.

### Query Selector Normalization (v1)

The v1 query system uses a minimal, explicit normalization policy:

- Trailing slash normalization: treat URL paths that differ only by a trailing slash as the same node (e.g. `/a` and `/a/` are equivalent).
- Canonical selector printing rule for trailing-slash-equivalent paths: print the non-trailing-slash form, except `/` remains `/`.
- Query strings are preserved and are part of node identity (e.g. `sitemap.xml?v=1` and `sitemap.xml?v=2` are different nodes/selectors).
- URL fragments are ignored for node identity and query mapping.
- Scheme is part of node identity (`http` and `https` are different if referenced that way).
- Authority is part of node identity exactly as used (`host[:port]`); differing ports are different nodes.
- URL path/encoding should use minimal normalization in v1 (preserve parsed URL representation as much as practical, aside from the rules above).
- `ls`-style output must print canonical query selectors.
- Canonical query selectors printed by `ls` must be accepted as valid query input without modification.

## Caching

### Cache Requirements

- Cache the built tree locally only after a successful full build of the sitemap tree
- Cache TTL (time-to-live) is configurable by user
- If the cache TTL option is omitted, use the tool's default cache behavior/TTL
- `--cache-ttl 0` means "do not use cache for this run" (no cache read/reuse and no cache write)
- If cache is present and not expired:
  - Load tree from cache
  - Skip network sitemap fetches
- If `--cache-ttl 0` is specified:
  - Skip cache read/reuse
  - Rebuild from network
  - Do not write/update cache for that run
- If cache is expired or absent:
  - Rebuild from network
  - Replace/update cache
- If a build fails before the full tree is built, do not write/update the cache for that run (return an error instead)
- Expired cache files should not be reused; implementation may delete expired cache files before rebuild (recommended) or overwrite them during rebuild

### Cache Format and Runtime Model

- Cache should use a format that is human-readable while remaining structured and unambiguous for machine parsing.
- Cache format for v1 is JSON-only.
- Metadata is stored as fixed-name key/value fields at the root JSON object, together with a `tree` field containing the sitemap tree structure.
- Cache is stored as one file per root domain/authority.
- The cached sitemap tree is expected to be small enough to read fully into memory for normal usage.
- On startup, when cache is valid, `sitemaper` loads the entire cached tree into memory and serves queries from memory.
- `sitemaper` does not support "querying the cache file directly" as a separate mode; all queries operate on the in-memory tree representation.
- Optimizations for large on-disk partial reads / indexed cache queries are out of scope for the initial version.

### Cache Location and Naming

- Cache files are stored in a cache directory.
- The cache directory is configurable by CLI option.
- A reasonable default cache directory must be provided.
- Recommended default (OS-appropriate): user cache dir + `sitemaper` subdirectory (e.g. via Go `os.UserCacheDir()`).
- Cache filename should be derived from the root scheme + domain/authority plus a fixed extension (e.g. `.sitemaper.json`) to avoid collisions between `http` and `https` roots.
- Filename generation must be deterministic and safe for filesystem use.

Example (illustrative):

- `https_example.com.sitemaper.json`
- `http_localhost_8081.sitemaper.json` (if port is part of root identity)

### Cache JSON Shape (v1)

The cache file is a single JSON object. The root object contains fixed metadata field names and a `tree` field holding the actual sitemap tree structure.

Requirements:

- JSON root object contains structured metadata fields (machine-unambiguous)
- JSON root object contains `tree` key with the sitemap tree
- The `tree` value is a nested tree structure (not a separate file or sidecar)
- Metadata field names at the JSON root are fixed for the cache version
- Node objects may reference children explicitly (recommended) to keep relationships clear and extensible

Recommended root metadata fields (v1):

- `version`
- `root_url`
- `root_authority`
- `built_at`
- `expires_at`
- `tree`

Example (illustrative only):

```json
{
  "version": 1,
  "root_url": "https://example.com/sitemap.xml",
  "root_authority": "example.com",
  "built_at": "2026-02-23T12:34:56Z",
  "expires_at": "2026-02-24T12:34:56Z",
  "tree": {
    "kind": "index",
    "url": "https://example.com/sitemap.xml",
    "children": [
      {
        "kind": "urlset",
        "url": "https://example.com/products/sitemap.xml",
        "children": [
          {
            "kind": "url",
            "loc": "https://example.com/products/a"
          }
        ]
      }
    ]
  }
}
```

## Proxy Support

`sitemaper` supports an optional proxy for HTTP(S) requests.

Requirements:

- User can provide proxy URL
- v1 must support `http://host:port` proxy configuration for sitemap fetch requests
- `socks5://host:port` support is optional (implementation-dependent)
- All sitemap fetch requests use the configured proxy
- Clear error message if proxy is invalid or unreachable

## CLI Requirements

Minimum CLI behavior:

- Accept target argument (`host[:port]`)
- Accept optional query path
- Accept cache TTL option
- Accept proxy option

Compatibility (optional):

- `--domain` may be supported as an alias for `--target`

### Example CLI Shape (proposed)

```bash
sitemaper --target example.com --query ::
sitemaper --target example.com --query blog/sitemap.xml --cache-ttl 24h
sitemaper --target example.com --proxy http://127.0.0.1:8080
```

Alternative positional style is also acceptable if kept consistent.

## Error Handling

- Network failures should be reported clearly
- XML parse errors should identify the sitemap URL that failed
- Query path not found should return a non-zero exit code and message
- Cache read/write corruption should trigger rebuild (with warning) when possible

## Testing Requirements

Testing should be extensive and include unit tests plus integration tests against a dedicated local test server setup.

### Test Server (Integration Testing)

Provide a local test server harness used by automated tests. The harness should expose multiple test sites (recommended: different ports) with known sitemap behaviors.

Required scenarios:

- `no sitemap`: host/port returns no `sitemap.xml` (404 or equivalent)
- `root only`: host/port serves only a root sitemap with no nested sitemap indexes
- `deep nested`: host/port serves deeply nested sitemap indexes and many leaf URL entries
- (optional but recommended) malformed XML sitemap
- (optional but recommended) cross-authority sitemap references across test ports (if omitted from integration fixtures, cross-authority query resolution must still be covered by unit tests)

Because the test data is fully controlled and known, tests must verify exact parsing results (not just success/failure).

### What Tests Must Validate

- Correct recursive traversal of sitemap indexes
- Correct classification of sitemap index vs `urlset`
- Correct extraction of child sitemap URLs
- Correct extraction of leaf URL entries
- URL entries found in `urlset` are stored in the tree but not fetched
- Correct tree shape and node counts for known fixtures
- Correct query path resolution (short local paths and fully qualified cross-authority URL segments)
- Correct root listing when no query path is provided
- Correct alphabetical ordering of `ls`-style output
- Correct cache reuse within TTL (no network refetch)
- Correct cache expiration behavior (refetch occurs)
- Correct proxy wiring (at least via transport-level tests or integration harness)
- Correct handling of missing sitemap on hosts with no `sitemap.xml`

### Suggested Test Structure (Go)

- Unit tests:
  - XML parsing (index/urlset/malformed)
  - URL normalization (per v1 query selector normalization rules) and deduplication
  - Query path resolution
  - Alphabetical listing/sort behavior for `ls` output
  - Cache TTL logic
  - Cache expiry invalidation behavior (expired cache is not reused and rebuild is triggered)
- Integration tests:
  - Build tree against local multi-port test server fixtures
  - Query listings against known expected outputs
  - Cache hit/miss behavior using temporary cache directory

### Test Fixture Design Notes

- Store expected tree outputs and URL lists as fixtures to compare against parser results.
- Keep fixture sitemap XML deterministic and small enough for fast tests, with one larger nested fixture for performance/concurrency coverage.
- Use explicit ports/hostnames in tests to validate cross-authority path rules (different ports can simulate different domains/origins in local testing).
- Test fixtures should validate exact tree structure: no missing nodes and no nodes attached under the wrong parent.
- Tests should validate normalization behavior matches the v1 query selector normalization rules.
- Tests should validate `ls`-style output shows the correct immediate child nodes for a queried selector.
- Tests should include isolated cases for alphabetical output ordering.
- Tests should include isolated cache TTL/expiry cases to confirm cache reuse before expiry and cache invalidation/rebuild after expiry.

## Performance Expectations

- Parallel fetch should reduce total build time vs sequential fetching
- Concurrency must be bounded and configurable (recommended)
- Avoid duplicate fetches for the same sitemap URL (deduplicate URLs)

## Security / Safety

- Only fetch sitemap XML URLs discovered from sitemap documents (and root sitemap)
- Do not fetch page URLs found in `<urlset><url><loc>`
- Enforce reasonable timeouts on HTTP client requests
- Limit maximum sitemap count / depth optionally to prevent abuse or runaway recursion

## Suggested Implementation Notes (Go)

- Use `encoding/xml` for sitemap parsing
- Use `net/http` with configurable timeout and proxy transport
- Use goroutines + semaphore/worker pool for bounded parallelism
- Use JSON for local cache serialization (human-readable, structured, and unambiguous)
- Normalize sitemap URLs only according to the defined v1 rules (e.g. trailing slash equivalence, fragment ignoring) to avoid duplicate fetches without over-normalizing

## Maintainability / Extensibility Requirements

- The codebase should be organized into small, well-defined modules (fetching, parsing, tree model, cache, query, CLI) with clear interfaces.
- Core logic should be decoupled from CLI argument parsing so features can be reused in tests and future integrations.
- Query parsing and path mapping should be implemented in a dedicated component to make future syntax changes low-risk.
- Cache file format (JSON-only) should use fixed root field names and versioned schema metadata to support backward-compatible evolution.
- Behavior that may change later (concurrency limits, cache policy, output formatting) should be configurable rather than hard-coded where practical.
- Tests should cover module boundaries so refactors can be done safely.

## Open Design Decisions (TBD)

- Output format (plain text only vs JSON option)
- Scheme fallback behavior (`https` only vs retry `http`)
- Default concurrency level
- Compressed sitemap support (`.xml.gz` URLs and/or gzip-compressed sitemap responses); if added, preserve the original sitemap URL/selector (including `.gz`) while parsing/decompressing internally before XML processing
- Whether to add explicit "filesystem recursion" CLI behavior in future (e.g. recursive listings/walks and how repeated references/loop-like structures should be displayed), beyond v1 single-node `ls`-style listings

## Acceptance Criteria

- Given a domain with nested sitemap indexes, `sitemaper` builds the full sitemap tree recursively.
- Sitemap fetches occur in parallel with bounded concurrency.
- Leaf sitemap XML files (`urlset`) are fetched and parsed; page URLs inside `<loc>` are stored but not requested.
- Running again within cache TTL uses local cache and avoids network fetches.
- A query path (including `::` for root) returns an `ls`-style listing of the corresponding tree node.
- Proxy option routes sitemap requests through the configured proxy.
