package architecture_test

import (
	"github.com/ahillspace/tadx/internal/architecture"
	"testing"
)

func TestLiveToolingKeepsIndependentRemoteOracle(t *testing.T) {
	root := moduleFixture(t)
	writeGo(t, root, "tests/live/harness/probe.go", `package harness
import (
 "example.test/tadx/internal/auth"
 "example.test/tadx/internal/config"
 "example.test/tadx/internal/tableau/workbook"
 "example.test/tadx/internal/resources/workbook"
 "example.test/tadx/actions/workbook"
)
`)
	writeGo(t, root, "tests/live/suite/probe.go", `package suite
import "example.test/tadx/tests/live/harness"
`)
	writeGo(t, root, "actions/probe/action.go", `package probe
import "example.test/tadx/tests/live/harness"
`)
	v, err := architecture.Check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 4 {
		t.Fatalf("want only remote-oracle and production-to-test violations, got %v", v)
	}
	for _, violation := range v {
		if violation.Import == "example.test/tadx/internal/auth" || violation.Import == "example.test/tadx/internal/config" {
			t.Errorf("credential/config boundary incorrectly blocked: %v", violation)
		}
	}
}
