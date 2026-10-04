package job

import (
	"context"
	"errors"
	"testing"

	tableauadmin "github.com/ahillspace/tadx/internal/tableau/admin"
)

type monitorRoleReader struct {
	role string
	err  error
	seen string
}

func (r *monitorRoleReader) GetUser(_ context.Context, id string) (tableauadmin.User, error) {
	r.seen = id
	return tableauadmin.User{SiteRole: r.role}, r.err
}

func TestMonitoringRolePortUsesExactAuthenticatedUserAndRoles(t *testing.T) {
	for _, tc := range []struct {
		role string
		want bool
	}{
		{"ServerAdministrator", true},
		{"SiteAdministratorExplorer", true},
		{"SiteAdministratorCreator", true},
		{"Explorer", false},
		{"", false},
	} {
		r := &monitorRoleReader{role: tc.role}
		got, err := (MonitoringRolePort{Reader: r, UserLUID: "user-1"}).Eligible(t.Context())
		if err != nil || got != tc.want || r.seen != "user-1" {
			t.Fatalf("role=%q eligible=%t user=%q err=%v", tc.role, got, r.seen, err)
		}
	}
	want := errors.New("native role read failed")
	got, err := (MonitoringRolePort{Reader: &monitorRoleReader{err: want}, UserLUID: "user-1"}).Eligible(t.Context())
	if got || !errors.Is(err, want) {
		t.Fatalf("eligible=%t err=%v", got, err)
	}
}
