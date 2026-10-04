package user_test

import (
	"context"
	"testing"
	"time"

	user "github.com/ahillspace/tadx/actions/admin/user"
	"github.com/ahillspace/tadx/internal/readsource"
)

type userProviderSpy struct {
	opens      int
	cacheReads int
}

func (*userProviderSpy) CacheTarget(string) (user.Target, error) {
	return user.Target{Environment: "canonical", Site: "site"}, nil
}
func (p *userProviderSpy) CachedList(user.Target) user.CachedListReader {
	return cachedUserPage{provider: p}
}
func (*userProviderSpy) CachedInspect(user.Target) user.CachedInspectResolver { return nil }
func (*userProviderSpy) ListFilter(user.ListInput) (string, error)            { return "", nil }
func (p *userProviderSpy) Open(context.Context, string, string, string, bool) (user.LiveSession, error) {
	p.opens++
	return user.LiveSession{}, nil
}
func (*userProviderSpy) Now() time.Time { return time.Time{} }

type cachedUserPage struct{ provider *userProviderSpy }

func (r cachedUserPage) ListUsers(_ context.Context, in user.ListPageRequest) (user.ListPage, error) {
	r.provider.cacheReads++
	items := []user.Record{{LUID: "u1", Name: "A"}, {LUID: "u2", Name: "B"}}
	return user.ListPage{Number: in.PageNumber, Size: in.PageSize, Total: len(items), Users: items[in.PageNumber-1 : in.PageNumber]}, nil
}
func (cachedUserPage) Source() *readsource.Metadata { return nil }

func TestServiceRejectsInvalidUserInputBeforeOpeningProvider(t *testing.T) {
	for name, invoke := range map[string]func(*user.Service) error{
		"list": func(s *user.Service) error {
			_, err := s.ListAdminUsers(t.Context(), user.ListInput{All: true, Limit: 1})
			return err
		},
		"inspect": func(s *user.Service) error {
			_, err := s.InspectAdminUser(t.Context(), user.InspectInput{})
			return err
		},
		"create": func(s *user.Service) error {
			_, err := s.CreateAdminUser(t.Context(), user.CreateInput{}, true)
			return err
		},
		"update": func(s *user.Service) error {
			_, err := s.UpdateAdminUser(t.Context(), user.UpdateInput{}, true)
			return err
		},
		"delete": func(s *user.Service) error {
			_, err := s.DeleteAdminUser(t.Context(), user.DeleteInput{}, true)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			provider := &userProviderSpy{}
			if err := invoke(user.New(provider)); err == nil || provider.opens != 0 || provider.cacheReads != 0 {
				t.Fatalf("error=%v opens=%d cacheReads=%d", err, provider.opens, provider.cacheReads)
			}
		})
	}
}

func TestServiceBindsCachedUserContinuationToCanonicalTarget(t *testing.T) {
	provider := &userProviderSpy{}
	service := user.New(provider)
	first, err := service.ListAdminUsers(t.Context(), user.ListInput{Environment: "alias", Cache: true, Limit: 1})
	if err != nil || first.Environment != "canonical" || first.Site != "site" || first.Page.NextCursor == "" {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := service.ListAdminUsers(t.Context(), user.ListInput{Environment: "alias", Cache: true, Cursor: first.Page.NextCursor})
	if err != nil || len(second.Users) != 1 || second.Users[0].LUID != "u2" || provider.opens != 0 || provider.cacheReads != 2 {
		t.Fatalf("second=%+v err=%v opens=%d cacheReads=%d", second, err, provider.opens, provider.cacheReads)
	}
}
