package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
	workspacecore "github.com/ahillspace/tadx/internal/workspace"
	"gopkg.in/yaml.v3"
)

func TestInvalidWorkspaceUnregisterHelpRegistersPreservedRootThroughCLI(t *testing.T) {
	for _, name := range []string{"broken", "-broken alias'fixture"} {
		t.Run(name, func(t *testing.T) {
			path := resilienceConfig(t, "")
			options := Options{ConfigPath: path, PATStore: &fakePATStore{}}
			resilienceRequire(t, options, "workspace", "create", "good-work", "--path", filepath.Join(t.TempDir(), "good"))
			root := filepath.Join(t.TempDir(), "preserved root")
			resilienceRequire(t, options, "workspace", "create", "--path", root, "--", name)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var raw map[string]any
			if err := yaml.Unmarshal(data, &raw); err != nil {
				t.Fatal(err)
			}
			raw["workspaces"].(map[string]any)[name].(map[string]any)["id"] = "invalid-identity"
			data, err = yaml.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			var recovery []string
			for _, preview := range []bool{true, false} {
				args := []string{"workspace", "unregister", "--json"}
				if preview {
					args = append(args, "--preview")
				}
				args = append(args, "--", name)
				out := resilienceRequire(t, options, args...)
				var result struct {
					Help []string `json:"help"`
				}
				if err := json.Unmarshal([]byte(out), &result); err != nil || len(result.Help) != 1 {
					t.Fatalf("unregister omitted repair help: output=%s err=%v", out, err)
				}
				recovery = parseCacheRecoveryCommand(t, result.Help[0])
				want := []string{"--config", path, "workspace", "register", "--path", "<path>"}
				if strings.HasPrefix(name, "-") {
					want = append(want, "--")
				}
				want = append(want, name)
				if !slices.Equal(recovery, want) {
					t.Fatalf("unregister emitted unsupported repair arguments: got=%q want=%q", recovery, want)
				}
			}
			recovery[slices.Index(recovery, "<path>")] = root
			resilienceRequire(t, options, recovery...)
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			registered, _, err := cfg.ResolveWorkspace(name)
			if err != nil || registered != name {
				t.Fatalf("repair command did not restore the exact registration: name=%q err=%v", registered, err)
			}
		})
	}
}

func TestBlankWorkspaceNamesCanBeUnregisteredThroughCLI(t *testing.T) {
	for _, name := range []string{"", " "} {
		for _, unsetDefault := range []bool{false, true} {
			t.Run(name, func(t *testing.T) {
				path := resilienceConfig(t, "")
				options := Options{ConfigPath: path, PATStore: &fakePATStore{}}
				resilienceRequire(t, options, "workspace", "create", "good-work", "--path", filepath.Join(t.TempDir(), "good"))
				root := filepath.Join(t.TempDir(), "preserved")
				resilienceRequire(t, options, "workspace", "create", "broken", "--path", root)
				marker := filepath.Join(root, "keep.txt")
				if err := os.WriteFile(marker, []byte("preserved"), 0o600); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var raw map[string]any
				if err := yaml.Unmarshal(data, &raw); err != nil {
					t.Fatal(err)
				}
				registrations := raw["workspaces"].(map[string]any)
				registrations[name] = registrations["broken"]
				delete(registrations, "broken")
				if unsetDefault {
					delete(raw, "default_workspace")
				}
				data, err = yaml.Marshal(raw)
				if err != nil || os.WriteFile(path, data, 0o600) != nil {
					t.Fatal("write invalid workspace fixture", err)
				}
				item, err := workspacecore.NewManager(path, nil).PreviewUnregister(t.Context(), name)
				if err != nil || item.Name != name || item.Default {
					t.Fatalf("unset defaults blocked or selected blank registration: item=%+v err=%v", item, err)
				}
				for _, preview := range []bool{true, false} {
					args := []string{"workspace", "unregister", "--json"}
					if preview {
						args = append(args, "--preview")
					}
					out := resilienceRequire(t, options, append(args, "--", name)...)
					var result struct {
						Workspace struct {
							Name    string `json:"name"`
							Status  string `json:"status"`
							Default bool   `json:"default"`
						} `json:"workspace"`
						Help []string `json:"help"`
					}
					if err := json.Unmarshal([]byte(out), &result); err != nil || result.Workspace.Name != name || result.Workspace.Status != "invalid" || result.Workspace.Default || len(result.Help) != 1 {
						t.Fatalf("unregister lost exact invalid identity: output=%s err=%v", out, err)
					}
					if want := []string{"--config", path, "workspace", "register", "--path", "<path>", "<name>"}; !slices.Equal(parseCacheRecoveryCommand(t, result.Help[0]), want) {
						t.Fatalf("blank name recovery did not request a valid replacement name: %s", out)
					}
					if current, err := os.ReadFile(path); preview && (err != nil || !bytes.Equal(current, data)) {
						t.Fatal("unregister preview changed configuration", err)
					}
					if contents, err := os.ReadFile(marker); err != nil || string(contents) != "preserved" {
						t.Fatal("unregister changed workspace files", err)
					}
				}
				cfg, err := config.Load(path)
				if err != nil || !slices.Equal(cfg.WorkspaceNames(), []string{"good-work"}) {
					t.Fatal("unregister removed another registration", err)
				}
				if code, out := resilienceRun(t, options, "workspace", "unregister", "--json"); code != 2 {
					t.Fatalf("omitted workspace unregister selector was accepted: code=%d output=%s", code, out)
				}
			})
		}
	}
}
