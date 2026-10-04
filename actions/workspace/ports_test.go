package workspace

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ahillspace/tadx/internal/artifact"
	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

func TestWorkspaceArtifactAdaptersPreserveCleanupWarnings(t *testing.T) {
	item := artifact.Item{TreeFingerprint: "private-tree-fingerprint", Warnings: []string{"cleanup remains", "second warning"}}
	moved, deleted := moveArtifact(item), deleteArtifact(item)
	if got := moved.Warnings; !reflect.DeepEqual(got, item.Warnings) {
		t.Fatalf("move warnings = %#v", got)
	}
	if got := deleted.Warnings; !reflect.DeepEqual(got, item.Warnings) {
		t.Fatalf("delete warnings = %#v", got)
	}
	moved.Warnings[0], deleted.Warnings[1] = "changed move", "changed delete"
	if !reflect.DeepEqual(item.Warnings, []string{"cleanup remains", "second warning"}) {
		t.Fatalf("projections modified the source warnings: %v", item.Warnings)
	}
	if got := statusArtifact(item).Diagnostic; got != "cleanup remains" {
		t.Fatalf("status diagnostic = %q", got)
	}
	encoded, err := json.Marshal(deleted)
	if err != nil || bytes.Contains(encoded, []byte("private-tree-fingerprint")) || deleted.TreeFingerprint != item.TreeFingerprint {
		t.Fatalf("delete fingerprint projection: JSON=%s target=%#v error=%v", encoded, deleted, err)
	}
}

func TestWorkspaceRegistrationProjectionRequiresAvailableValidManifest(t *testing.T) {
	for _, available := range []bool{false, true} {
		for _, valid := range []bool{false, true} {
			registration := workspaceRegistration(workspacecore.Record{Name: "example", ID: "ws_1", Root: "root", Available: available, ManifestValid: valid})
			if registration.Registered != (available && valid) {
				t.Fatalf("available=%t valid=%t registration=%#v", available, valid, registration)
			}
			encoded, err := json.Marshal(registration)
			if err != nil || bytes.Contains(encoded, []byte("created_entries")) {
				t.Fatalf("registration JSON=%s error=%v", encoded, err)
			}
		}
	}
}
