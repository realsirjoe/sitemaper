package robots

import "strings"

// ParseSitemaps extracts sitemap URLs from robots.txt contents.
func ParseSitemaps(data []byte) []string {
	lines := strings.Split(string(data), "\n")
	out := make([]string, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Remove whole-line comments and inline comments (good enough for v1).
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		if !strings.Contains(line, ":") {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(k), "Sitemap") {
			continue
		}
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
