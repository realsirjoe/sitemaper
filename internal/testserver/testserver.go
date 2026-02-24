package testserver

import (
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
)

type Suite struct {
	NoSitemap  *Server
	RootOnly   *Server
	RobotsOnly *Server
	DeepA      *Server
	DeepB      *Server
	Malformed  *Server

	PageHitsDeepA atomic.Int64
	PageHitsDeepB atomic.Int64
	HitsRootOnly  atomic.Int64
}

func NewSuite() *Suite {
	s := &Suite{}

	s.NoSitemap = newServer(http.NotFoundHandler())

	s.RootOnly = newServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sitemap.xml":
			s.HitsRootOnly.Add(1)
			fmt.Fprintf(w, `<?xml version="1.0"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>%s/page-b</loc></url>
  <url><loc>%s/page-a</loc></url>
</urlset>`, s.RootOnly.URL, s.RootOnly.URL)
		case "/page-a", "/page-b":
			// Should never be requested by sitemaper.
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))

	s.RobotsOnly = newServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sitemap.xml":
			http.NotFound(w, r)
		case "/robots.txt":
			fmt.Fprintf(w, "User-agent: *\nDisallow:\nSitemap: %s/robots-products.xml\n", s.RobotsOnly.URL)
		case "/robots-products.xml":
			fmt.Fprintf(w, `<?xml version="1.0"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>%s/products/b</loc></url>
  <url><loc>%s/products/a</loc></url>
</urlset>`, s.RobotsOnly.URL, s.RobotsOnly.URL)
		case "/products/a", "/products/b":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))

	var deepB *Server
	deepB = newServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/x/child.xml":
			fmt.Fprintf(w, `<?xml version="1.0"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>%s/pages/z</loc></url>
  <url><loc>%s/pages/a/</loc></url>
</urlset>`, deepB.URL, deepB.URL)
		case "/pages/z", "/pages/a":
			s.PageHitsDeepB.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	s.DeepB = deepB

	var deepA *Server
	deepA = newServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sitemap.xml":
			fmt.Fprintf(w, `<?xml version="1.0"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>%s/nested/index.xml</loc></sitemap>
  <sitemap><loc>%s/x/child.xml</loc></sitemap>
</sitemapindex>`, deepA.URL, s.DeepB.URL)
		case "/nested/index.xml":
			fmt.Fprintf(w, `<?xml version="1.0"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>%s/nested/leaf.xml</loc></sitemap>
</sitemapindex>`, deepA.URL)
		case "/nested/leaf.xml":
			fmt.Fprintf(w, `<?xml version="1.0"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>%s/page/b</loc></url>
  <url><loc>%s/page/a</loc></url>
</urlset>`, deepA.URL, deepA.URL)
		case "/page/a", "/page/b":
			s.PageHitsDeepA.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	s.DeepA = deepA

	s.Malformed = newServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<sitemapindex><sitemap><loc>http://bad`)) // malformed
		default:
			http.NotFound(w, r)
		}
	}))

	return s
}

type Server struct {
	URL string
	srv *http.Server
	ln  net.Listener
}

func (s *Server) Close() {
	if s == nil {
		return
	}
	_ = s.srv.Close()
	_ = s.ln.Close()
}

func newServer(h http.Handler) *Server {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	srv := &http.Server{Handler: h}
	go func() {
		_ = srv.Serve(ln)
	}()
	return &Server{
		URL: "http://" + ln.Addr().String(),
		srv: srv,
		ln:  ln,
	}
}

func (s *Suite) Close() {
	if s.NoSitemap != nil {
		s.NoSitemap.Close()
	}
	if s.RootOnly != nil {
		s.RootOnly.Close()
	}
	if s.DeepA != nil {
		s.DeepA.Close()
	}
	if s.RobotsOnly != nil {
		s.RobotsOnly.Close()
	}
	if s.DeepB != nil {
		s.DeepB.Close()
	}
	if s.Malformed != nil {
		s.Malformed.Close()
	}
}
