package sitemapxml

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

type Kind string

const (
	KindIndex  Kind = "index"
	KindURLSet Kind = "urlset"
)

type Parsed struct {
	Kind Kind
	Locs []string
}

func Parse(data []byte) (*Parsed, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				return nil, fmt.Errorf("empty xml")
			}
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch strings.ToLower(se.Name.Local) {
		case "sitemapindex":
			var x sitemapIndexXML
			if err := dec.DecodeElement(&x, &se); err != nil {
				return nil, err
			}
			out := &Parsed{Kind: KindIndex}
			for _, s := range x.Sitemaps {
				if strings.TrimSpace(s.Loc) != "" {
					out.Locs = append(out.Locs, strings.TrimSpace(s.Loc))
				}
			}
			return out, nil
		case "urlset":
			var x urlSetXML
			if err := dec.DecodeElement(&x, &se); err != nil {
				return nil, err
			}
			out := &Parsed{Kind: KindURLSet}
			for _, u := range x.URLs {
				if strings.TrimSpace(u.Loc) != "" {
					out.Locs = append(out.Locs, strings.TrimSpace(u.Loc))
				}
			}
			return out, nil
		default:
			return nil, fmt.Errorf("unsupported root element <%s>", se.Name.Local)
		}
	}
}

type sitemapIndexXML struct {
	Sitemaps []struct {
		Loc string `xml:"loc"`
	} `xml:"sitemap"`
}

type urlSetXML struct {
	URLs []struct {
		Loc string `xml:"loc"`
	} `xml:"url"`
}
