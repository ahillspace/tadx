//go:build windows

package app_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/workspace"
	"golang.org/x/sys/windows"
)

func windowsFixturePathName(t *testing.T, path string, expand func(*uint16, *uint16, uint32) (uint32, error)) string {
	t.Helper()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	buffer := make([]uint16, 512)
	for {
		size, err := expand(name, &buffer[0], uint32(len(buffer)))
		if err != nil || size == 0 {
			t.Fatalf("resolve generated filesystem spelling: %v", err)
		}
		if size >= uint32(len(buffer)) {
			buffer = make([]uint16, size+1)
			continue
		}
		return windows.UTF16ToString(buffer[:size])
	}
}

func shortPathWorkspaceFixture(t *testing.T) (string, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "long workspace directory")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	long := windowsFixturePathName(t, root, windows.GetLongPathName)
	short := windowsFixturePathName(t, long, windows.GetShortPathName)
	if strings.EqualFold(filepath.Clean(long), filepath.Clean(short)) {
		t.Skip("filesystem exposes no distinct Windows short-name alias for the generated workspace")
	}
	left, leftErr := os.Stat(long)
	right, rightErr := os.Stat(short)
	if leftErr != nil || rightErr != nil || !os.SameFile(left, right) {
		t.Fatalf("generated short and long spellings identify different directories: %v, %v", leftErr, rightErr)
	}
	return long, short
}

func shortPathRunCLI(t *testing.T, options app.Options, args ...string) string {
	t.Helper()
	var output strings.Builder
	if code := app.Run(t.Context(), append(args, "--json"), &output, options); code != 0 {
		t.Fatalf("CLI %s exit=%d: %s", strings.Join(args[:min(len(args), 3)], " "), code, output.String())
	}
	return output.String()
}

func shortPathRequireRelative(t *testing.T, path string) {
	t.Helper()
	if path == "" || !filepath.IsLocal(filepath.FromSlash(path)) || strings.Contains(path, `\`) || !strings.HasPrefix(path, "artifacts/") {
		t.Fatalf("artifact output is not a relative slash-delimited path: %q", path)
	}
}

func shortPathRejectRootDisclosure(t *testing.T, text string, roots ...string) {
	t.Helper()
	for _, root := range roots {
		for _, spelling := range []string{root, filepath.ToSlash(root), strings.Trim(strconv.Quote(root), `"`)} {
			if strings.Contains(strings.ToLower(text), strings.ToLower(spelling)) {
				t.Fatal("rendered result disclosed an absolute workspace spelling")
			}
		}
	}
}

func shortPathLinkDirectory(t *testing.T, link, target string) {
	t.Helper()
	if output, err := exec.CommandContext(t.Context(), "cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Fatalf("create isolated fixture junction: %v: %s", err, output)
	}
	t.Cleanup(func() {
		if err := os.Remove(link); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Errorf("remove owned junction fixture: %v", err)
		}
	})
}

func TestWindowsShortPathWorkspacePullResolveCloneAndDeleteThroughCLI(t *testing.T) {
	for _, scenario := range []struct{ kind, presented string }{{"datasource", "long"}, {"datasource", "short"}, {"workbook", "long"}, {"workbook", "short"}, {"flow", "long"}, {"flow", "short"}} {
		t.Run(scenario.kind+"/"+scenario.presented, func(t *testing.T) {
			long, short := shortPathWorkspaceFixture(t)
			selected := long
			if scenario.presented == "short" {
				selected = short
			}
			id, extension, payload := "ds-alias", ".tds", "<datasource/>"
			if scenario.kind == "workbook" {
				id, extension, payload = "wb-alias", ".twb", "<workbook/>"
			} else if scenario.kind == "flow" {
				id, extension, payload = "flow-alias", ".tfl", `{"nodes":{}}`
			}
			resource := "/api/3.29/sites/site-1/" + scenario.kind + "s/" + id
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				switch r.URL.Path {
				case resource:
					_, _ = fmt.Fprintf(w, `<tsResponse><%s id="%s" name="Alias Sales" fileType="%s"><project id="project-1" name="Analytics"/></%s></tsResponse>`, scenario.kind, id, strings.TrimPrefix(extension, "."), scenario.kind)
				case resource + "/content":
					w.Header().Set("Content-Disposition", `attachment; filename="Alias Sales`+extension+`"`)
					_, _ = io.WriteString(w, payload)
				case "/api/3.29/sites/site-1/projects":
					_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
				case "/api/metadata/graphql":
					if !diagnosticEmptyWorkbookMetadata(w, r) {
						http.Error(w, "fixture has no lineage access", http.StatusForbidden)
					}
				default:
					t.Errorf("unexpected fixture request: %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected request", http.StatusBadRequest)
				}
			}))
			t.Cleanup(server.Close)
			options := diagnosticOptions(t, server)
			shortPathRunCLI(t, options, "workspace", "create", "default", "--path", filepath.Join(filepath.Dir(options.ConfigPath), "default"))
			shortPathRunCLI(t, options, "workspace", "create", "alias", "--path", selected)
			if scenario.kind == "flow" {
				shortPathRunCLI(t, options, "content", "flow", "pull", "--environment", "test", "--workspace", "alias", "--id", id, "--preview")
			}
			output := shortPathRunCLI(t, options, "content", scenario.kind, "pull", "--environment", "test", "--workspace", "alias", "--id", id)
			var pulled struct {
				Status   string `json:"status"`
				Artifact struct {
					Path          string `json:"path"`
					CanonicalPath string `json:"canonical_path"`
				} `json:"artifact"`
			}
			if err := json.Unmarshal([]byte(output), &pulled); err != nil {
				t.Fatal(err)
			}
			if pulled.Status != "pulled" || !strings.HasPrefix(pulled.Artifact.Path, "artifacts/"+scenario.kind+"/") {
				t.Fatalf("unexpected pull receipt: %s", output)
			}
			shortPathRequireRelative(t, pulled.Artifact.Path)
			shortPathRequireRelative(t, pulled.Artifact.CanonicalPath)
			shortPathRejectRootDisclosure(t, output, long, short)
			for _, root := range []string{long, short} {
				item, err := artifact.Resolve(t.Context(), root, artifact.Selector{Kind: scenario.kind, Path: pulled.Artifact.Path})
				if err != nil || item.LUID != id || item.State != artifact.StateClean || item.Path != pulled.Artifact.Path || item.CanonicalPath != pulled.Artifact.CanonicalPath {
					t.Fatalf("resolve the same artifact through a workspace alias: %+v, %v", item, err)
				}
				if content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(item.CanonicalPath))); err != nil || string(content) != payload {
					t.Fatalf("alias payload changed: %q, %v", content, err)
				}
			}
			clone := filepath.Join(filepath.Dir(short), "cloned workspace")
			shortPathRunCLI(t, options, "workspace", "clone", "alias", "--name", "copy", "--path", clone)
			if _, err := artifact.Resolve(t.Context(), clone, artifact.Selector{Kind: scenario.kind, Path: pulled.Artifact.Path}); err != nil {
				t.Fatalf("sibling clone is not a valid artifact control: %v", err)
			}
			for _, root := range []string{long, short} {
				if _, err := artifact.Resolve(t.Context(), root, artifact.Selector{Kind: scenario.kind, Path: "../cloned workspace/" + pulled.Artifact.Path}); err == nil {
					t.Fatal("workspace alias admitted an escape to a valid sibling artifact")
				}
			}
			if content, err := os.ReadFile(filepath.Join(clone, filepath.FromSlash(pulled.Artifact.CanonicalPath))); err != nil || string(content) != payload {
				t.Fatalf("cloning through an aliased parent lost the payload: %q, %v", content, err)
			}
			for _, name := range []string{"copy", "alias"} {
				shortPathRunCLI(t, options, "workspace", "delete", name)
			}
			for _, root := range []string{long, short, clone} {
				if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("deleted workspace remains available through a spelling: %v", err)
				}
			}
		})
	}
}

func TestWindowsShortPathDownloadsRetainLifecycleAndCleanupOwnership(t *testing.T) {
	long, short := shortPathWorkspaceFixture(t)
	manager := workspace.NewManager(filepath.Join(t.TempDir(), "config.yaml"), nil)
	created, err := manager.Create(t.Context(), "alias", short)
	if err != nil {
		t.Fatal(err)
	}
	downloads := workspace.NewDownloads(short)
	t.Cleanup(downloads.Close)
	staged, err := downloads.File(t.Context(), func(w io.Writer) error { _, err := io.WriteString(w, "active download"); return err })
	if err != nil {
		t.Fatal(err)
	}
	actual, actualErr := os.Stat(filepath.Dir(filepath.Dir(staged)))
	want, wantErr := os.Stat(filepath.Join(long, ".tadx", "tmp"))
	if actualErr != nil || wantErr != nil || !os.SameFile(actual, want) {
		t.Fatalf("download was staged outside the physical workspace: %v, %v", actualErr, wantErr)
	}
	for _, root := range []string{long, short} {
		if _, err := workspace.Clean(t.Context(), root, "temporary"); err == nil || !strings.Contains(err.Error(), "in use") {
			t.Fatalf("cleanup entered through an alias while download was active: %v", err)
		}
		selected := created
		selected.Root = root
		if _, err := manager.Delete(t.Context(), workspace.DeleteRequest{Expected: selected}); !errors.Is(err, workspace.ErrWorkspaceInUse) {
			t.Fatalf("deletion entered through an alias while download was active: %v", err)
		}
	}
	if content, err := os.ReadFile(staged); err != nil || string(content) != "active download" {
		t.Fatalf("a refused operation changed staged content: %q, %v", content, err)
	}
	downloads.Close()
	if _, err := os.Stat(staged); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("closing staged downloads retained the payload: %v", err)
	}
	cleaned, err := workspace.Clean(t.Context(), short, "temporary")
	if err != nil || len(cleaned.Removed) != 1 || cleaned.Removed[0] != ".tadx/tmp" {
		t.Fatalf("cleanup after closing the alias owner: %+v, %v", cleaned, err)
	}
}

func TestWindowsShortPathCleanupErrorsRemainRelativeWithFilesystemCause(t *testing.T) {
	long, short := shortPathWorkspaceFixture(t)
	blocked := filepath.Join(long, ".tadx", "tmp.lock")
	if err := os.MkdirAll(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{long, short} {
		_, err := workspace.Clean(t.Context(), root, "temporary")
		cause, ok := errors.AsType[*fs.PathError](err)
		if !ok || !errors.Is(err, cause.Err) || !strings.Contains(err.Error(), ".tadx/tmp.lock") {
			t.Fatalf("cleanup lost the relative path or inspectable filesystem cause: %v", err)
		}
		shortPathRejectRootDisclosure(t, err.Error(), long, short)
		actual, actualErr := os.Stat(cause.Path)
		want, wantErr := os.Stat(blocked)
		if actualErr != nil || wantErr != nil || !os.SameFile(actual, want) {
			t.Fatalf("filesystem cause identifies another object: %v, %v", actualErr, wantErr)
		}
	}
}

func TestWindowsShortPathArtifactAliasDoesNotAdmitJunctionEscape(t *testing.T) {
	long, short := shortPathWorkspaceFixture(t)
	manager := workspace.NewManager(filepath.Join(t.TempDir(), "config.yaml"), nil)
	if _, err := manager.Create(t.Context(), "alias", short); err != nil {
		t.Fatal(err)
	}
	outside := windowsFixturePathName(t, t.TempDir(), windows.GetLongPathName)
	if _, err := manager.Create(t.Context(), "outside", outside); err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(t.TempDir(), "Outside.tds")
	if err := os.WriteFile(payload, []byte("<datasource/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	created, err := artifact.NewDatasourceManager(time.Now).Pull(t.Context(), artifact.DatasourcePull{Workspace: outside, Filename: "Outside.tds", PayloadPath: payload, Metadata: artifact.DatasourceMetadata{Name: "Outside", TableauID: "outside-id", SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1", SourceEnvironment: "test", SourceSite: "", SourceProjectName: "Analytics", SourceProjectID: "project-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := artifact.Resolve(t.Context(), outside, artifact.Selector{Kind: "datasource", Path: created.WorkspaceRelativePath}); err != nil {
		t.Fatalf("outside control artifact is invalid: %v", err)
	}
	link := filepath.Join(long, "artifacts")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	shortPathLinkDirectory(t, link, filepath.Join(outside, "artifacts"))
	for _, root := range []string{long, short} {
		if _, err := artifact.Resolve(t.Context(), root, artifact.Selector{Kind: "datasource", Path: created.WorkspaceRelativePath}); err == nil {
			t.Fatal("artifact resolution followed a junction outside the workspace")
		}
	}
	if content, err := os.ReadFile(created.CanonicalPath); err != nil || string(content) != "<datasource/>" {
		t.Fatalf("junction rejection changed the outside control payload: %q, %v", content, err)
	}
}

func TestWindowsPullRejectsManagedJunctionBeforeOutsideMutation(t *testing.T) {
	for _, kind := range []string{"datasource", "flow"} {
		for _, boundary := range []string{"artifacts", "resource"} {
			t.Run(kind+"/"+boundary, func(t *testing.T) {
				long, short := shortPathWorkspaceFixture(t)
				outside, _ := shortPathWorkspaceFixture(t)
				extension, incoming := ".tds", `<datasource incoming="true"/>`
				old := []byte(`<datasource original="true"/>`)
				if kind == "flow" {
					extension, incoming, old = ".tfl", `{"nodes":{"incoming":{}}}`, []byte(`{"nodes":{"original":{}}}`)
				}
				resource := "/api/3.29/sites/site-1/" + kind + "s/item-1"
				server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if diagnosticSignIn(w, r) {
						return
					}
					switch r.URL.Path {
					case resource:
						_, _ = fmt.Fprintf(w, `<tsResponse><%s id="item-1" name="Junction" fileType="tfl"><project id="project-1" name="Analytics"/></%s></tsResponse>`, kind, kind)
					case resource + "/content":
						w.Header().Set("Content-Disposition", `attachment; filename="Junction`+extension+`"`)
						_, _ = io.WriteString(w, incoming)
					case "/api/3.29/sites/site-1/projects":
						_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Analytics"/></projects></tsResponse>`, r.URL.Query().Get("pageSize"))
					case "/api/metadata/graphql":
						http.Error(w, "fixture lineage unavailable", http.StatusForbidden)
					default:
						t.Errorf("unexpected fixture request: %s %s", r.Method, r.URL.Path)
						http.Error(w, "unexpected request", http.StatusBadRequest)
					}
				}))
				t.Cleanup(server.Close)
				options := diagnosticOptions(t, server)
				shortPathRunCLI(t, options, "workspace", "create", "source", "--path", short)
				shortPathRunCLI(t, options, "workspace", "create", "outside", "--path", outside)
				var target, canonical string
				if kind == "flow" {
					created, err := artifact.NewFlowManager(nil).Pull(t.Context(), artifact.FlowPull{Workspace: outside, Filename: "Junction.tfl", PayloadPath: app.PayloadFile(t, old), Metadata: artifact.FlowMetadata{Name: "Junction", TableauID: "item-1", SourceServerOrigin: server.URL, SourceSiteLUID: "site-1", SourceEnvironment: "test", SourceProjectName: "Analytics", SourceProjectID: "project-1", FileType: "tfl"}, Lineage: artifact.LineageDocument{Direction: "both", Depth: 1}})
					if err != nil {
						t.Fatal(err)
					}
					target, canonical = created.ArtifactPath, created.CanonicalPath
				} else {
					created, err := artifact.NewDatasourceManager(nil).Pull(t.Context(), artifact.DatasourcePull{Workspace: outside, Filename: "Junction.tds", PayloadPath: app.PayloadFile(t, old), Metadata: artifact.DatasourceMetadata{Name: "Junction", TableauID: "item-1", SourceServerOrigin: server.URL, SourceSiteLUID: "site-1", SourceEnvironment: "test", SourceProjectName: "Analytics", SourceProjectID: "project-1"}, Lineage: artifact.LineageDocument{Direction: "both", Depth: 1}})
					if err != nil {
						t.Fatal(err)
					}
					target, canonical = created.ArtifactPath, created.CanonicalPath
				}
				if _, err := artifact.Resolve(t.Context(), outside, artifact.Selector{Kind: kind, LUID: "item-1"}); err != nil {
					t.Fatalf("outside matched artifact is invalid: %v", err)
				}
				metadataPath := filepath.Join(target, "metadata.json")
				metadata, err := os.ReadFile(metadataPath)
				if err != nil {
					t.Fatal(err)
				}
				link, destination := filepath.Join(long, "artifacts"), filepath.Join(outside, "artifacts")
				if boundary == "resource" {
					link, destination = filepath.Join(link, kind), filepath.Join(destination, kind)
				} else if err := os.Remove(link); err != nil {
					t.Fatal(err)
				}
				shortPathLinkDirectory(t, link, destination)
				for _, preview := range []bool{true, false} {
					args := []string{"content", kind, "pull", "--environment", "test", "--workspace", "source", "--id", "item-1", "--json"}
					if preview {
						args = append(args, "--preview")
					}
					var output strings.Builder
					code := app.Run(t.Context(), args, &output, options)
					after, payloadErr := os.ReadFile(canonical)
					afterMetadata, metadataErr := os.ReadFile(metadataPath)
					if code == 0 || payloadErr != nil || metadataErr != nil || !bytes.Equal(after, old) || !bytes.Equal(afterMetadata, metadata) {
						t.Errorf("preview=%t admitted a managed junction or changed outside state: code=%d payload-error=%v metadata-error=%v output=%s", preview, code, payloadErr, metadataErr, output.String())
					}
				}
			})
		}
	}
}
