package architecture_test

import (
	"testing"

	"github.com/ahillspace/tadx/internal/architecture"
)

func TestPolicyMechanismsBelongOnlyToPolicyOwner(t *testing.T) {
	root := moduleFixture(t)
	for _, file := range []string{
		"actions/policy/service.go",
		"actions/policy/install/service.go",
		"actions/workbook/service.go",
	} {
		writeGo(t, root, file, "package fixture\nimport _ \"example.test/tadx/internal/managedpolicy\"\n")
	}
	violations, err := architecture.Check(root)
	if err != nil {
		t.Fatal(err)
	}
	assertViolationStrings(t, violations, []string{
		"actions/policy/install/service.go imports example.test/tadx/internal/managedpolicy: actions must not import unapproved local packages",
		"actions/workbook/service.go imports example.test/tadx/internal/managedpolicy: actions must not import unapproved local packages",
	})
}
