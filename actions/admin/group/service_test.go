package group_test

import (
	"context"
	"testing"
	"time"

	group "github.com/ahillspace/tadx/actions/admin/group"
	"github.com/ahillspace/tadx/internal/readsource"
)

type groupProviderSpy struct {
	opens      int
	cacheReads int
}

func (*groupProviderSpy) CacheTarget(string) (group.Target, error) {
	return group.Target{Environment: "canonical", Site: "site"}, nil
}
func (p *groupProviderSpy) CachedList(group.Target) group.CachedListReader {
	return cachedGroupPage{provider: p}
}
func (*groupProviderSpy) CachedInspect(group.Target) group.CachedInspectResolver { return nil }
func (*groupProviderSpy) LegacyInventoryCursor(string) bool                      { return false }
func (*groupProviderSpy) ListFilter(group.ListInput) (string, error)             { return "", nil }
func (p *groupProviderSpy) Open(context.Context, string, string, string, bool) (group.LiveSession, error) {
	p.opens++
	return group.LiveSession{}, nil
}
func (*groupProviderSpy) ValidateComplete(bool, *readsource.Metadata) error { return nil }
func (*groupProviderSpy) Now() time.Time                                    { return time.Time{} }

type cachedGroupPage struct{ provider *groupProviderSpy }

func (r cachedGroupPage) ListGroups(_ context.Context, in group.ListPageRequest) (group.ListPage, error) {
	r.provider.cacheReads++
	items := []group.Record{{LUID: "g1", Name: "A"}, {LUID: "g2", Name: "B"}}
	return group.ListPage{Number: in.PageNumber, Size: in.PageSize, Total: len(items), Groups: items[in.PageNumber-1 : in.PageNumber]}, nil
}
func (cachedGroupPage) Source() *readsource.Metadata { return nil }

func TestServiceRejectsInvalidGroupInputBeforeOpeningProvider(t *testing.T) {
	for name, invoke := range map[string]func(*group.Service) error{
		"list": func(s *group.Service) error {
			_, err := s.ListAdminGroups(t.Context(), group.ListInput{All: true, Limit: 1})
			return err
		},
		"inspect": func(s *group.Service) error {
			_, err := s.InspectAdminGroup(t.Context(), group.InspectInput{})
			return err
		},
		"create": func(s *group.Service) error {
			_, err := s.CreateAdminGroup(t.Context(), group.CreateInput{Name: "A"}, true)
			return err
		},
		"update": func(s *group.Service) error {
			_, err := s.UpdateAdminGroup(t.Context(), group.UpdateInput{}, true)
			return err
		},
		"delete": func(s *group.Service) error {
			_, err := s.DeleteAdminGroup(t.Context(), group.DeleteInput{}, true)
			return err
		},
		"member add": func(s *group.Service) error {
			_, err := s.AddAdminGroupMember(t.Context(), group.MembershipInput{}, true)
			return err
		},
		"member remove": func(s *group.Service) error {
			_, err := s.RemoveAdminGroupMember(t.Context(), group.MembershipInput{}, true)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			provider := &groupProviderSpy{}
			if err := invoke(group.New(provider)); err == nil || provider.opens != 0 || provider.cacheReads != 0 {
				t.Fatalf("error=%v opens=%d cacheReads=%d", err, provider.opens, provider.cacheReads)
			}
		})
	}
}

func TestServiceBindsCachedGroupContinuationToCanonicalTarget(t *testing.T) {
	provider := &groupProviderSpy{}
	service := group.New(provider)
	first, err := service.ListAdminGroups(t.Context(), group.ListInput{Environment: "alias", Cache: true, Limit: 1})
	if err != nil || first.Environment != "canonical" || first.Site != "site" || first.Page.NextCursor == "" {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := service.ListAdminGroups(t.Context(), group.ListInput{Environment: "alias", Cache: true, Cursor: first.Page.NextCursor})
	if err != nil || len(second.Groups) != 1 || second.Groups[0].LUID != "g2" || provider.opens != 0 || provider.cacheReads != 2 {
		t.Fatalf("second=%+v err=%v opens=%d cacheReads=%d", second, err, provider.opens, provider.cacheReads)
	}
}
