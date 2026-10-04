package user_test

import (
	"context"
	"strings"
	"testing"

	user "github.com/ahillspace/tadx/actions/admin/user"
)

func TestAdminUserUpdateCloudFullNamePreflightUsesTrustedHost(t *testing.T) {
	for _, tt := range []struct {
		host     string
		input    user.UpdateRequest
		rejected bool
	}{
		{"https://us-east-1.online.tableau.com", user.UpdateRequest{FullName: new("Alex")}, true},
		{"https://online.tableau.com.evil.test", user.UpdateRequest{FullName: new("Alex")}, false},
		{"https://us-east-1.online.tableau.com", user.UpdateRequest{Email: new("alex@example.com")}, false},
	} {
		t.Run(tt.host+map[bool]string{true: "/full_name", false: "/other"}[tt.input.FullName != nil], func(t *testing.T) {
			err := user.ValidateUpdatePolicy(t.Context(), tt.host, func(context.Context) (string, error) { return "", nil }, tt.input)
			if (err != nil) != tt.rejected || (tt.rejected && !strings.Contains(err.Error(), "Tableau Cloud")) {
				t.Fatalf("host=%s rejected=%t error=%v", tt.host, tt.rejected, err)
			}
		})
	}
}
