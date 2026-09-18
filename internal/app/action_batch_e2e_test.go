package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestUserDeleteRepeatedIDsShareSessionAndKeepPartialResults(t *testing.T) {
	var signins int
	var deleted []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			signins++
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/3.29/sites/site-1/users/")
		if r.Method == http.MethodGet {
			if id == "missing" {
				http.Error(w, "missing", 404)
				return
			}
			fmt.Fprintf(w, `<tsResponse><user id="%s" name="%s@tadx.net" siteRole="Viewer"/></tsResponse>`, id, id)
			return
		}
		if r.Method == http.MethodDelete {
			deleted = append(deleted, id)
			w.WriteHeader(204)
			return
		}
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		w.WriteHeader(500)
	}))
	defer server.Close()
	opts := diagnosticOptions(t, server)
	opts.UserHomeDir = func() (string, error) { return t.TempDir(), nil }
	var output bytes.Buffer
	code := app.Run(context.Background(), []string{"admin", "user", "delete", "--id", "first", "--id", "missing", "--id", "last", "--env", "test", "--json"}, &output, opts)
	if code == 0 || !json.Valid(output.Bytes()) || signins != 1 || !reflect.DeepEqual(deleted, []string{"first", "last"}) {
		t.Fatalf("code=%d signins=%d deleted=%v output=%s", code, signins, deleted, output.String())
	}
	var result struct {
		Succeeded, Failed int
		Items             []json.RawMessage
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Succeeded != 2 || result.Failed != 1 || len(result.Items) != 3 {
		t.Fatalf("result=%+v", result)
	}
}

func TestBatchFileDifferentUserSettingsPreviewAndExecution(t *testing.T) {
	for _, preview := range []bool{true, false} {
		t.Run(fmt.Sprint(preview), func(t *testing.T) {
			var signins int
			roles := map[string]string{"one": "Viewer", "two": "Viewer"}
			var updated []string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					signins++
					return
				}
				id := strings.TrimPrefix(r.URL.Path, "/api/3.29/sites/site-1/users/")
				if _, ok := roles[id]; !ok {
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(500)
					return
				}
				if r.Method == http.MethodPut {
					updated = append(updated, id)
					if id == "one" {
						roles[id] = "Explorer"
					} else {
						roles[id] = "Unlicensed"
					}
				} else if r.Method != http.MethodGet {
					t.Errorf("unexpected method %s", r.Method)
				}
				fmt.Fprintf(w, `<tsResponse><user id="%s" name="%s@tadx.net" siteRole="%s"/></tsResponse>`, id, id, roles[id])
			}))
			defer server.Close()
			opts := diagnosticOptions(t, server)
			root := t.TempDir()
			opts.UserHomeDir = func() (string, error) { return root, nil }
			path := filepath.Join(t.TempDir(), "updates.json")
			if err := os.WriteFile(path, []byte(`{"items":[{"id":"one","site-role":"Explorer"},{"id":"two","site-role":"Unlicensed"}]}`), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"admin", "user", "update", "--batch-file", path, "--env", "test", "--json"}
			if preview {
				args = append(args, "--preview")
				opts = withSiteMutationConsent(t, opts, false)
			}
			var output bytes.Buffer
			code := app.Run(context.Background(), args, &output, opts)
			if code != 0 || !json.Valid(output.Bytes()) || signins != 1 {
				t.Fatalf("code=%d signins=%d output=%s", code, signins, output.String())
			}
			if preview && len(updated) != 0 {
				t.Fatal("preview wrote users")
			}
			if !preview && !reflect.DeepEqual(updated, []string{"one", "two"}) {
				t.Fatalf("updates=%v", updated)
			}
		})
	}
}
