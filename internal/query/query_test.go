package query

import (
	"reflect"
	"testing"

	"sitemaper/internal/model"
)

func TestParseAndResolve(t *testing.T) {
	root := &model.Node{
		Kind: model.NodeIndex,
		Children: []*model.Node{
			{Kind: model.NodeIndex, Selector: "b"},
			{Kind: model.NodeIndex, Selector: "a", Children: []*model.Node{
				{Kind: model.NodeURLSet, Selector: "a1"},
			}},
		},
	}

	got := root.ChildSelectors()
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("sorted selectors = %#v", got)
	}

	n, err := Resolve(root, "a::a1")
	if err != nil {
		t.Fatal(err)
	}
	if n.Selector != "a1" {
		t.Fatalf("resolved %q", n.Selector)
	}

	if _, err := Resolve(root, "::"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Parse("a::::b"); err == nil {
		t.Fatal("expected parse error for empty segment")
	}
}

func TestResolveAcceptsFullyQualifiedLocalSelector(t *testing.T) {
	root := &model.Node{
		Kind: model.NodeIndex,
		URL:  "http://127.0.0.1:52054/sitemap.xml",
		Children: []*model.Node{
			{Kind: model.NodeIndex, Selector: "nested/index.xml"},
		},
	}
	n, err := Resolve(root, "http://127.0.0.1:52054/nested/index.xml")
	if err != nil {
		t.Fatal(err)
	}
	if n.Selector != "nested/index.xml" {
		t.Fatalf("got %q", n.Selector)
	}
}
