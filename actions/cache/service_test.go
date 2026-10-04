package cache_test

import (
	"context"
	"reflect"
	"testing"

	cacheaction "github.com/ahillspace/tadx/actions/cache"
	"github.com/ahillspace/tadx/internal/config"
)

type cacheServiceProvider struct {
	resolved  int
	hydrators int
	checked   [][]string
	events    []string
}

func (p *cacheServiceProvider) ResolveEnvironment(alias string) (config.Environment, error) {
	p.resolved++
	p.events = append(p.events, "resolve")
	return config.Environment{Alias: alias, SiteContentURL: "exact-site"}, nil
}
func (p *cacheServiceProvider) CheckScopes(scopes []string) error {
	p.checked = append(p.checked, append([]string(nil), scopes...))
	p.events = append(p.events, "check")
	return nil
}
func (p *cacheServiceProvider) Hydrator(config.Environment, bool) cacheaction.Hydrator {
	p.hydrators++
	p.events = append(p.events, "hydrate")
	return &recordingHydrator{result: completeResult()}
}
func (p *cacheServiceProvider) StatusSource(config.Environment) cacheaction.StatusSource {
	return nil
}

func TestCacheServiceValidatesLocallyBeforeProviderAndPreviewStaysLocal(t *testing.T) {
	provider := &cacheServiceProvider{}
	service := cacheaction.New(provider)
	if _, err := service.RefreshCache(t.Context(), cacheaction.RefreshInput{Environment: "dev", Scopes: []string{"unknown"}}); err == nil {
		t.Fatal("invalid scope accepted")
	}
	if provider.resolved != 0 || provider.hydrators != 0 || len(provider.checked) != 0 {
		t.Fatalf("invalid input used provider: %+v", provider)
	}
	output, err := service.RefreshCache(t.Context(), cacheaction.RefreshInput{Environment: "dev", Preview: true})
	if err != nil || output.Status != "preview" || output.Plan == nil || output.Plan.Site != "exact-site" {
		t.Fatalf("preview output=%#v error=%v", output, err)
	}
	if provider.resolved != 1 || provider.hydrators != 0 || len(provider.checked) != 0 {
		t.Fatalf("preview used remote dependency: %+v", provider)
	}
}

func TestCacheServicePreservesDefaultCollectorPreflightBeforeTarget(t *testing.T) {
	provider := &cacheServiceProvider{}
	_, _ = cacheaction.New(provider).RefreshCache(context.Background(), cacheaction.RefreshInput{Environment: "dev"})
	want := []string{"users", "groups", "projects", "workbooks", "datasources", "flows", "views"}
	if len(provider.checked) != 1 || !reflect.DeepEqual(provider.checked[0], want) || !reflect.DeepEqual(provider.events, []string{"check", "resolve", "hydrate"}) || provider.resolved != 1 || provider.hydrators != 1 {
		t.Fatalf("preflight/target order = %+v", provider)
	}
}
