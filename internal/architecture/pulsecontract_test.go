package architecture_test

import (
	"testing"

	"github.com/ahillspace/tadx/internal/architecture"
)

func TestPulseContractIsAvailableOnlyToPulseActions(t *testing.T) {
	for _, tc := range []struct {
		file    string
		allowed bool
	}{
		{"actions/pulse/definition/create/action.go", true},
		{"actions/pulse/definition/publish/action.go", true},
		{"actions/pulse/metric/fork/action.go", true},
		{"actions/workbook/publish.go", false},
		{"actions/pulsex/create/action.go", false},
		{"internal/app/pulse.go", false},
		{"internal/tableau/pulse/client.go", false},
		{"internal/resources/pulse/adapter.go", false},
	} {
		t.Run(tc.file, func(t *testing.T) {
			root := moduleFixture(t)
			writeGo(t, root, tc.file, "package fixture\nimport _ \"example.test/tadx/internal/pulsecontract\"\n")
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			if (len(violations) == 0) != tc.allowed {
				t.Fatalf("allowed=%t violations=%v", tc.allowed, violations)
			}
		})
	}
}

func TestPulseContractPermitsOnlyStandardLibraryImports(t *testing.T) {
	root := moduleFixture(t)
	writeGo(t, root, "internal/pulsecontract/validation.go", `package pulsecontract
import (
 _ "encoding/json"
 _ "math/big"
 _ "example.test/tadx/actions/pulse/definition/create"
 _ "example.test/tadx/internal/app"
 _ "example.test/tadx/internal/tableau/pulse"
 _ "example.test/tadx/internal/value"
 _ "example.test/tadx/internal/pulsecontract/helper"
 _ "github.com/example/schema"
)
`)
	violations, err := architecture.Check(root)
	if err != nil {
		t.Fatal(err)
	}
	assertViolationStrings(t, violations, []string{
		"internal/pulsecontract/validation.go imports example.test/tadx/actions/pulse/definition/create: Pulse contracts must depend only on the standard library",
		"internal/pulsecontract/validation.go imports example.test/tadx/internal/app: Pulse contracts must depend only on the standard library",
		"internal/pulsecontract/validation.go imports example.test/tadx/internal/pulsecontract/helper: Pulse contracts must depend only on the standard library",
		"internal/pulsecontract/validation.go imports example.test/tadx/internal/tableau/pulse: Pulse contracts must depend only on the standard library",
		"internal/pulsecontract/validation.go imports example.test/tadx/internal/value: Pulse contracts must depend only on the standard library",
		"internal/pulsecontract/validation.go imports github.com/example/schema: Pulse contracts must depend only on the standard library",
	})
}
