//go:build windows

package artifact_test

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/artifact"
)

func managedJunctionStoragePull(t *testing.T, consumer, root, revision string) (string, error) {
	t.Helper()
	switch consumer {
	case "flow":
		payload := filepath.Join(t.TempDir(), "Daily.tfl")
		if err := os.WriteFile(payload, fmt.Appendf(nil, `{"nodes":{%q:{}}}`, revision), 0o600); err != nil {
			t.Fatal(err)
		}
		result, err := artifact.NewFlowManager(nil).Pull(t.Context(), artifact.FlowPull{Workspace: root, Filename: "Daily.tfl", PayloadPath: payload, Metadata: artifact.FlowMetadata{Name: "Daily", TableauID: "flow-1", SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1", SourceEnvironment: "test", SourceSite: "sales", SourceProjectName: "Analytics", SourceProjectID: "project-1", FileType: "tfl"}, Lineage: artifact.LineageDocument{Direction: "both", Depth: 1}})
		return result.CanonicalPath, err
	case "pulse-definition":
		configuration := fmt.Appendf(nil, `{"metadata":{"id":"definition-1","name":"Revenue","description":%q},"specification":{"datasource":{"id":"datasource-1"}}}`, revision)
		result, err := artifact.NewPulseDefinitionManager(nil).Pull(t.Context(), artifact.PulseDefinitionPull{Workspace: root, Configuration: configuration, Metadata: artifact.PulseDefinitionMetadata{Name: "Revenue", TableauID: "definition-1", DatasourceLUID: "datasource-1", SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1", SourceEnvironment: "test", SourceSite: "sales"}})
		return filepath.Join(root, filepath.FromSlash(result.CanonicalPath)), err
	case "lineage":
		result, err := artifact.NewLineageManager(nil).Pull(t.Context(), artifact.LineagePull{Workspace: root, Metadata: artifact.LineageMetadata{ResourceKind: "flow", Name: "Daily", TableauID: "flow-1", MetadataID: "meta-flow", SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1", SourceEnvironment: "test", SourceSite: "sales"}, Lineage: artifact.LineageDocument{Complete: true, Direction: "both", Depth: 1, Nodes: []artifact.LineageNode{{MetadataID: "meta-flow", Kind: "flow", RESTLUID: "flow-1", Name: revision}}}, CountsKnown: true})
		return filepath.Join(root, filepath.FromSlash(result.LineagePath)), err
	default:
		t.Fatalf("unknown fixture consumer %q", consumer)
		return "", nil
	}
}

func managedJunctionStorageSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	state := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			state[filepath.ToSlash(relative)] = "directory"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		state[filepath.ToSlash(relative)] = "file:" + string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot outside artifact tree: %v", err)
	}
	return state
}

func managedJunctionStorageLink(t *testing.T, link, target string) {
	t.Helper()
	if output, err := exec.CommandContext(t.Context(), "cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Fatalf("create managed-directory junction fixture: %v: %s", err, output)
	}
	t.Cleanup(func() {
		if err := os.Remove(link); err != nil {
			t.Errorf("remove owned junction fixture: %v", err)
		}
	})
}

func managedJunctionStorageControl(t *testing.T, consumer string) (string, string) {
	t.Helper()
	outside := createWorkspace(t)
	canonical, err := managedJunctionStoragePull(t, consumer, outside, "original")
	if err != nil {
		t.Fatal(err)
	}
	luid := "flow-1"
	if consumer == "pulse-definition" {
		luid = "definition-1"
	}
	if _, err := artifact.Resolve(t.Context(), outside, artifact.Selector{Kind: consumer, LUID: luid}); err != nil {
		t.Fatalf("outside matched artifact is invalid: %v", err)
	}
	return outside, canonical
}

func TestWindowsManagedPullAdmitsArtifactsBeforeLegacyRecovery(t *testing.T) {
	for _, consumer := range []string{"flow", "pulse-definition", "lineage"} {
		for _, linked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/junction=%t", consumer, linked), func(t *testing.T) {
				outside, canonical := managedJunctionStorageControl(t, consumer)
				original, err := os.ReadFile(canonical)
				if err != nil {
					t.Fatal(err)
				}
				artifacts := filepath.Join(outside, "artifacts")
				marker := filepath.Join(artifacts, ".tadx-bundle-transaction.committed.json")
				if err := os.WriteFile(marker, []byte("committed\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				before := managedJunctionStorageSnapshot(t, artifacts)
				root := outside
				if linked {
					root = createWorkspace(t)
					link := filepath.Join(root, "artifacts")
					if err := os.Remove(link); err != nil && !errors.Is(err, os.ErrNotExist) {
						t.Fatal(err)
					}
					managedJunctionStorageLink(t, link, artifacts)
				}
				_, pullErr := managedJunctionStoragePull(t, consumer, root, "incoming")
				if linked {
					if after := managedJunctionStorageSnapshot(t, artifacts); pullErr == nil || !maps.Equal(before, after) {
						t.Fatalf("managed junction allowed a pull or altered outside content/metadata/legacy marker: error=%v unchanged=%t", pullErr, maps.Equal(before, after))
					}
					return
				}
				if pullErr != nil {
					t.Fatalf("normal legacy recovery control failed: %v", pullErr)
				}
				if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("normal pull did not consume the valid legacy marker: %v", err)
				}
				if after, err := os.ReadFile(canonical); err != nil || bytes.Equal(after, original) {
					t.Fatalf("normal pull did not install its new payload: %v", err)
				}
			})
		}
	}
}

func TestWindowsManagedPullRejectsResourceRootJunctions(t *testing.T) {
	for _, consumer := range []string{"flow", "pulse-definition", "lineage"} {
		parts := []string{consumer}
		if consumer == "lineage" {
			parts = []string{"lineage", "flow"}
		}
		for depth := range len(parts) {
			t.Run(consumer+"/"+strings.Join(parts[:depth+1], "/"), func(t *testing.T) {
				outside, _ := managedJunctionStorageControl(t, consumer)
				before := managedJunctionStorageSnapshot(t, filepath.Join(outside, "artifacts"))
				root := createWorkspace(t)
				link := filepath.Join(append([]string{root, "artifacts"}, parts[:depth+1]...)...)
				target := filepath.Join(append([]string{outside, "artifacts"}, parts[:depth+1]...)...)
				if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
					t.Fatal(err)
				}
				managedJunctionStorageLink(t, link, target)
				_, err := managedJunctionStoragePull(t, consumer, root, "incoming")
				after := managedJunctionStorageSnapshot(t, filepath.Join(outside, "artifacts"))
				if err == nil || !maps.Equal(before, after) {
					t.Fatalf("resource-root junction admitted a pull or changed outside state: error=%v unchanged=%t", err, maps.Equal(before, after))
				}
			})
		}
	}
}
