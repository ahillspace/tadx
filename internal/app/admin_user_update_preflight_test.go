package app

import (
	"strings"
	"testing"

	userops "github.com/ahillspace/tadx/actions/admin/user"
)

func TestAdminUserUpdateCloudFullNamePreflightUsesTrustedHost(t *testing.T) {
	for _, tt := range []struct {
		host     string
		input    userops.UpdateRequest
		rejected bool
	}{
		{"https://us-east-1.online.tableau.com", userops.UpdateRequest{FullName: new("Alex")}, true},
		{"https://online.tableau.com.evil.test", userops.UpdateRequest{FullName: new("Alex")}, false},
		{"https://us-east-1.online.tableau.com", userops.UpdateRequest{Email: new("alex@example.com")}, false},
	} {
		t.Run(tt.host+map[bool]string{true: "/full_name", false: "/other"}[tt.input.FullName != nil], func(t *testing.T) {
			err := (adminUserAdapter{serverURL: tt.host}).ValidateUpdate(t.Context(), tt.input)
			if (err != nil) != tt.rejected || (tt.rejected && !strings.Contains(err.Error(), "Tableau Cloud")) {
				t.Fatalf("host=%s rejected=%t error=%v", tt.host, tt.rejected, err)
			}
		})
	}
}
