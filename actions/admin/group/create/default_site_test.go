package create

import (
	"context"
	"testing"
)

type defaultSiteBackend struct{ reads, writes int }

func (b *defaultSiteBackend) FindGroups(context.Context, string) ([]Group, error) {
	b.reads++
	return nil, nil
}

func (b *defaultSiteBackend) CreateGroup(context.Context, Request) (Group, error) {
	b.writes++
	return Group{LUID: "group-1"}, nil
}

func TestDefaultSiteRequiresResolvedTarget(t *testing.T) {
	for _, resolved := range []bool{false, true} {
		b := &defaultSiteBackend{}
		out, err := New(b, b).Execute(context.Background(), Input{Environment: "dev", TargetResolved: resolved, Name: "Group"}, true)
		if (err == nil) != resolved || b.writes != 0 {
			t.Fatalf("resolved=%t output=%#v err=%v backend=%#v", resolved, out, err, b)
		}
		if !resolved && b.reads != 0 {
			t.Fatalf("unresolved target reached Tableau: %d reads", b.reads)
		}
	}
}
