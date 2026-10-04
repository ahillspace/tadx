package cli_test

import (
	searchaction "github.com/ahillspace/tadx/actions/search"
	"github.com/ahillspace/tadx/internal/cli"
	"reflect"
	"testing"
)

func TestTopLevelSearchMapsCanonicalFlags(t *testing.T) {
	s := &searcher{}
	deps := dependencies(&lister{}, &getter{}, &renderer{})
	deps.Searcher = s
	root := cli.NewRoot(deps)
	root.SetArgs([]string{"search", "revenue", "--environment", "production", "--type", "workbook", "--cache", "--cursor", "next", "--limit", "12"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	want := searchaction.Input{Terms: "revenue", Environment: "production", Type: "workbook", Cache: true, Cursor: "next", Limit: 12}
	if !reflect.DeepEqual(s.input, want) {
		t.Fatalf("search input = %#v, want %#v", s.input, want)
	}
	if child, _, err := root.Find([]string{"cache", "search"}); err == nil && child.Name() == "search" {
		t.Fatal("obsolete cache search command is mounted")
	}
}
