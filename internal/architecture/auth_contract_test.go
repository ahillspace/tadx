package architecture_test

import (
	"testing"

	"github.com/ahillspace/tadx/internal/architecture"
)

func TestAuthCredentialMechanismDependencyIsExact(t *testing.T) {
	for _, tc := range []struct {
		name, file string
		allowed    bool
	}{
		{"auth workflow", "actions/auth/persistence.go", true},
		{"session readiness", "actions/session/action.go", true},
		{"other action", "actions/env/profile.go", false},
		{"obsolete verb package", "actions/auth/login/action.go", false},
		{"nested lookalike", "actions/auth/nested/persistence.go", false},
		{"nested session package", "actions/session/nested/action.go", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := moduleFixture(t)
			writeGo(t, root, tc.file, "package fixture\nimport _ \"example.test/tadx/internal/auth\"\n")
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			if tc.allowed {
				assertViolationStrings(t, violations, nil)
				return
			}
			assertViolationStrings(t, violations, []string{tc.file + " imports example.test/tadx/internal/auth: actions must not import authentication logic"})
		})
	}
}
