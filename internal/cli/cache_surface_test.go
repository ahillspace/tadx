package cli_test

import (
	"context"
	"strings"
	"testing"

	cacheaction "github.com/ahillspace/tadx/actions/cache"
	"github.com/ahillspace/tadx/internal/cli"
)

type cacheRefreshStub struct{}

func (cacheRefreshStub) RefreshCache(context.Context, cacheaction.RefreshInput) (cacheaction.RefreshOutput, error) {
	return cacheaction.RefreshOutput{}, nil
}

type cacheStatusStub struct{}

func (cacheStatusStub) ReadCacheStatus(context.Context, cacheaction.StatusInput) (cacheaction.StatusOutput, error) {
	return cacheaction.StatusOutput{}, nil
}

func TestLocalCacheCommandsKeepDistinctRegistrations(t *testing.T) {
	deps := dependencies(&lister{}, &getter{}, &renderer{})
	deps.CacheRefresher, deps.CacheStatuser = cacheRefreshStub{}, cacheStatusStub{}
	root := cli.NewRoot(deps)
	registrations, err := cli.RegisteredCommands(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, verb := range []string{"refresh", "status"} {
		found := false
		for _, registration := range registrations {
			if registration.CapabilityID == "cache."+verb {
				found = strings.Join(registration.CommandPath, " ") == "cache "+verb
			}
		}
		if !found {
			t.Fatalf("missing cache %s registration: %#v", verb, registrations)
		}
	}
}

func TestSearchCacheFlagSelectsLocalReadWithoutLegacyAlias(t *testing.T) {
	for _, flag := range []string{"--cache", "--cch", "--catalog", "--cat"} {
		t.Run(flag, func(t *testing.T) {
			search := &searcher{}
			deps := dependencies(&lister{}, &getter{}, &renderer{})
			deps.Searcher = search
			root := cli.NewRoot(deps)
			root.SetArgs([]string{"search", "Revenue", "--type", "workbook", flag})
			err := root.ExecuteContext(context.Background())
			if flag == "--cache" || flag == "--cch" {
				if err != nil || !search.input.Cache {
					t.Fatalf("cache search: input=%#v error=%v", search.input, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "unknown flag") || search.input.Cache {
				t.Fatalf("legacy flag was not rejected: input=%#v error=%v", search.input, err)
			}
		})
	}
}
