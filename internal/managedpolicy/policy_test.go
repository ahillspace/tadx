package managedpolicy

import (
	"errors"
	"reflect"
	"testing"

	"github.com/ahillspace/tadx/internal/capability"
)

func testCatalog() []capability.Definition {
	return []capability.Definition{{ID: "read"}, {ID: "write", RemoteMutation: true}, {ID: "admin", Administrative: true}}
}

func TestParseStrictContract(t *testing.T) {
	valid := `{"version":1,"allowed_capabilities":["read","write"],"remote_mutations":false}`
	if _, err := Parse([]byte(valid), testCatalog()); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		`{}`, `null`, `[]`, `{"version":1,"allowed_capabilities":[],"remote_mutations":null}`,
		`{"version":1,"allowed_capabilities":null,"remote_mutations":false}`,
		`{"version":2,"allowed_capabilities":[],"remote_mutations":false}`,
		`{"version":1,"allowed_capabilities":["unknown"],"remote_mutations":false}`,
		`{"version":1,"allowed_capabilities":["read","read"],"remote_mutations":false}`,
		`{"version":1,"version":1,"allowed_capabilities":[],"remote_mutations":false}`,
		`{"version":1,"allowed_capabilities":[],"remote_mutations":false,"extra":true}`,
		`{"Version":1,"allowed_capabilities":[],"remote_mutations":false}`,
		valid + `{}`, valid + ` junk`,
	} {
		t.Run(input, func(t *testing.T) {
			if _, err := Parse([]byte(input), testCatalog()); err == nil {
				t.Fatal("accepted invalid policy")
			}
		})
	}
}

func TestTemplateBoundaries(t *testing.T) {
	for _, name := range []string{"read-only", "read-write-no-admin", "admin"} {
		doc, err := Template(name, testCatalog())
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"read", "write"}
		if name == "admin" {
			want = []string{"admin", "read", "write"}
		}
		if !reflect.DeepEqual(doc.AllowedCapabilities, want) || doc.RemoteMutations != (name != "read-only") {
			t.Fatalf("%s: %#v", name, doc)
		}
	}
}

func TestEnforcement(t *testing.T) {
	p := activePolicy("fixture", Document{Version: 1, AllowedCapabilities: []string{"read"}})
	if err := p.CheckCapability("read"); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(p.CheckCapability("new-capability"), ErrCapabilityDenied) {
		t.Fatal("new capability allowed")
	}
	if !errors.Is(p.CheckRemoteMutation(), ErrRemoteMutationDenied) {
		t.Fatal("remote mutation allowed")
	}
	p.status.State = StateBlocked
	if !errors.Is(p.CheckCapability("read"), ErrBlocked) || !errors.Is(p.CheckRemoteMutation(), ErrBlocked) {
		t.Fatal("blocked policy failed open")
	}
	var zero Policy
	if !errors.Is(zero.CheckCapability("read"), ErrBlocked) {
		t.Fatal("uninitialized policy failed open")
	}
}
