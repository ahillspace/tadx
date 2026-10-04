package permission_test

import (
	"context"
	"testing"

	permission "github.com/ahillspace/tadx/actions/admin/permission"
)

type permissionProviderSpy struct{ opens int }

func (p *permissionProviderSpy) Open(context.Context, string, string, string, bool) (permission.LiveSession, error) {
	p.opens++
	return permission.LiveSession{}, nil
}

func TestServiceRejectsInvalidPermissionInputBeforeOpeningProvider(t *testing.T) {
	for name, invoke := range map[string]func(*permission.Service) error{
		"inspect": func(s *permission.Service) error {
			_, err := s.InspectAdminPermission(t.Context(), permission.InspectInput{})
			return err
		},
		"create": func(s *permission.Service) error {
			_, err := s.CreateAdminPermission(t.Context(), permission.Input{ResourceKind: "workbook", ResourceLUID: "w1"}, true)
			return err
		},
		"delete": func(s *permission.Service) error {
			_, err := s.DeleteAdminPermission(t.Context(), permission.Input{ResourceKind: "workbook", ResourceLUID: "w1"}, true)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			provider := &permissionProviderSpy{}
			if err := invoke(permission.New(provider)); err == nil || provider.opens != 0 {
				t.Fatalf("error=%v opens=%d", err, provider.opens)
			}
		})
	}
}
