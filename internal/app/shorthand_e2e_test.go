package app_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestShorthandReadPathsKeepCanonicalOperationsAndFlagValues(t *testing.T) {
	var workbookNameFilter atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/3.29/auth/signin":
			_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case "/api/3.29/sites/site-1/workbooks/wb-1":
			_, _ = io.WriteString(w, `<tsResponse><workbook id="wb-1" name="Finance"><project id="project-1" name="Ops"/><owner id="owner-1"/></workbook></tsResponse>`)
		case "/api/3.29/sites/site-1/workbooks":
			if strings.Contains(r.URL.Query().Get("filter"), "--pv") {
				workbookNameFilter.Store(true)
			}
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1" totalAvailable="1"/><workbooks><workbook id="wb-value" name="--pv"><project id="project-1" name="Ops"/><owner id="owner-1"/></workbook></workbooks></tsResponse>`)
		case "/api/3.29/sites/site-1/projects":
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Ops" topLevelProject="true"/></projects></tsResponse>`)
		case "/api/3.29/sites/site-1/datasources/ds-1":
			_, _ = io.WriteString(w, `<tsResponse><datasource id="ds-1" name="Orders"><project id="project-1" name="Ops"/></datasource></tsResponse>`)
		case "/api/v1/vizql-data-service/read-metadata":
			_, _ = io.WriteString(w, `{"data":[{"fieldName":"Sales, net","fieldCaption":"Sales","dataType":"REAL","fieldRole":"MEASURE"},{"fieldName":"Order Date","fieldCaption":"Order Date","dataType":"DATE","fieldRole":"DIMENSION"},{"fieldName":"Other","dataType":"STRING","fieldRole":"DIMENSION"}]}`)
		case "/api/-/pulse/definitions":
			_, _ = io.WriteString(w, `{"definitions":[{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"ds-1"}}}]}`)
		case "/api/3.29/auth/signout":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected shorthand request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	options := shorthandOptions(t, server)
	run := func(args ...string) string {
		t.Helper()
		var stdout bytes.Buffer
		if code := app.Run(context.Background(), args, &stdout, options); code != 0 {
			t.Fatalf("tadx %s: code=%d output=%s", strings.Join(args, " "), code, stdout.String())
		}
		return stdout.String()
	}

	if output := run("con", "wb", "ins", "-e", "test", "-i", "wb-1", "-f"); !strings.Contains(output, "luid: wb-1") {
		t.Fatalf("workbook shorthand output=%s", output)
	}
	if output := run("lst"); !strings.Contains(output, "operation: workbook.inspect") {
		t.Fatalf("last result did not retain canonical operation: %s", output)
	}
	if output := run("con", "prj", "ins", "--env", "test", "-i", "project-1"); !strings.Contains(output, "luid: project-1") {
		t.Fatalf("project shorthand output=%s", output)
	}
	if output := run("con", "ds", "sch", "--env", "test", "-i", "ds-1", "--fid", "Sales, net", "--fid", "Order Date", "--ful"); !strings.Contains(output, "returned: 2") || strings.Contains(output, "field_id: Other") {
		t.Fatalf("repeated --fid shorthand output=%s", output)
	}
	if output := run("pls", "def", "ls", "--env", "test", "-l", "1"); !strings.Contains(output, "definition-1") {
		t.Fatalf("Pulse shorthand output=%s", output)
	}
	if output := run("con", "wb", "ls", "--env", "test", "--nm=--pv", "-l", "1"); !strings.Contains(output, "--pv") || !workbookNameFilter.Load() {
		t.Fatalf("flag-looking --nm value was rewritten: filter=%t output=%s", workbookNameFilter.Load(), output)
	}
}

func TestShorthandDatasourcePublishPreviewHonorsBooleanAndMutationGate(t *testing.T) {
	var reads, writes atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/3.29/auth/signin":
			_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/projects"):
			reads.Add(1)
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/datasources"):
			reads.Add(1)
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="0"/><datasources/></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/datasources"):
			writes.Add(1)
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	file := filepath.Join(t.TempDir(), "Orders.tds")
	if err := os.WriteFile(file, []byte("<datasource/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := shorthandOptions(t, server)
	options.MutationsEnabled = false
	base := []string{"con", "ds", "pub", "--fil", file, "--env", "test", "--pid", "project-1", "--new"}

	var preview bytes.Buffer
	if code := app.Run(context.Background(), append(append([]string(nil), base...), "--pv=true", "-f"), &preview, options); code != 0 || writes.Load() != 0 || !strings.Contains(preview.String(), "operation: datasource.publish") || !strings.Contains(preview.String(), "mode: preview") {
		t.Fatalf("preview code=%d reads=%d writes=%d output=%s", code, reads.Load(), writes.Load(), preview.String())
	}
	readCount := reads.Load()
	var rejected bytes.Buffer
	if code := app.Run(context.Background(), append(append([]string(nil), base...), "--pv=false"), &rejected, options); code == 0 || !strings.Contains(rejected.String(), "mutation.disabled") || reads.Load() != readCount || writes.Load() != 0 {
		t.Fatalf("false preview bypassed gate: code=%d reads=%d/%d writes=%d output=%s", code, readCount, reads.Load(), writes.Load(), rejected.String())
	}
}

func TestDocumentedDatasourceShorthandExamplesUseCanonicalPaths(t *testing.T) {
	var requests, deletes atomic.Int32
	var documentedListRequest atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/auth/signin"):
			_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/datasources"):
			if r.URL.Query().Get("filter") == "projectName:eq:Analytics" && r.URL.Query().Get("pageSize") == "50" {
				documentedListRequest.Store(true)
			}
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><datasources><datasource id="ds-revenue" name="Revenue"><project id="project-1" name="Analytics"/></datasource></datasources></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/datasources/ds-revenue"):
			deletes.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected shorthand request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	options := shorthandOptions(t, server)
	options.MutationsEnabled = false

	var invalidDelete bytes.Buffer
	if code := app.Run(context.Background(), []string{"con", "ds", "del", "--env", "test", "--nm", "Revenue", "--pv", "-f"}, &invalidDelete, options); code == 0 || !strings.Contains(invalidDelete.String(), "use --id or both --name and --project") || requests.Load() != 0 {
		t.Fatalf("incomplete delete code=%d requests=%d output=%s", code, requests.Load(), invalidDelete.String())
	}
	var invalidList bytes.Buffer
	if code := app.Run(context.Background(), []string{"con", "ds", "ls", "--env", "test", "--pid", "project-1", "-l", "50"}, &invalidList, options); code == 0 || !strings.Contains(invalidList.String(), "unknown flag: --pid") || requests.Load() != 0 {
		t.Fatalf("unsupported list filter code=%d requests=%d output=%s", code, requests.Load(), invalidList.String())
	}

	var preview bytes.Buffer
	if code := app.Run(context.Background(), []string{"con", "ds", "del", "--env", "test", "--nm", "Revenue", "--prj", "Analytics", "-p", "-f"}, &preview, options); code != 0 || !strings.Contains(preview.String(), "operation: datasource.delete") || !strings.Contains(preview.String(), "mode: preview") || !strings.Contains(preview.String(), "luid: ds-revenue") || deletes.Load() != 0 {
		t.Fatalf("corrected delete code=%d deletes=%d output=%s", code, deletes.Load(), preview.String())
	}
	var list bytes.Buffer
	if code := app.Run(context.Background(), []string{"con", "ds", "ls", "--env", "test", "--pnm", "Analytics", "-l", "50"}, &list, options); code != 0 || !strings.Contains(list.String(), "ds-revenue") || !documentedListRequest.Load() || deletes.Load() != 0 {
		t.Fatalf("corrected list code=%d request=%t deletes=%d output=%s", code, documentedListRequest.Load(), deletes.Load(), list.String())
	}
}

func TestShorthandHelpAndCompletionExposeAliasesWithoutChangingCanonicalUse(t *testing.T) {
	options := app.Options{ConfigPath: filepath.Join(t.TempDir(), "config.yaml")}
	var help bytes.Buffer
	if code := app.Run(context.Background(), []string{"con", "wb", "ins", "--help"}, &help, options); code != 0 {
		t.Fatalf("alias help code=%d output=%s", code, help.String())
	}
	for _, want := range []string{"Usage: tadx content workbook <verb> [flags]", "inspect: target", "--id <luid>", "--environment (--env,-e)", "--full (details, not rows)"} {
		if !strings.Contains(help.String(), want) {
			t.Errorf("alias help missing %q:\n%s", want, help.String())
		}
	}
	var deleteHelp bytes.Buffer
	if code := app.Run(context.Background(), []string{"con", "ds", "del", "--help"}, &deleteHelp, options); code != 0 {
		t.Fatalf("delete alias help code=%d output=%s", code, deleteHelp.String())
	}
	for _, want := range []string{"Target (remote, exact):", "--id <luid>", "| (--name <name> --project (--prj) <path>)"} {
		if !strings.Contains(deleteHelp.String(), want) {
			t.Errorf("delete alias help missing %q:\n%s", want, deleteHelp.String())
		}
	}
	var forceHelp bytes.Buffer
	if code := app.Run(context.Background(), []string{"agt", "ist", "--help"}, &forceHelp, options); code != 0 {
		t.Fatalf("force alias help code=%d output=%s", code, forceHelp.String())
	}
	for _, want := range []string{"--full (details, not rows)", "[--force]"} {
		if !strings.Contains(forceHelp.String(), want) {
			t.Errorf("force help missing independent full/force spelling %q:\n%s", want, forceHelp.String())
		}
	}

	var completion bytes.Buffer
	if code := app.Run(context.Background(), []string{"__complete", "con", "wb", "i"}, &completion, options); code != 0 {
		t.Fatalf("dynamic completion through aliases code=%d output=%s", code, completion.String())
	}
	for _, want := range []string{"inspect", "alias: ins", ":4"} {
		if !strings.Contains(completion.String(), want) {
			t.Errorf("completion missing %q", want)
		}
	}

	completion.Reset()
	if code := app.Run(context.Background(), []string{"__complete", "con", "ds", "del", "-"}, &completion, options); code != 0 {
		t.Fatalf("flag completion through aliases code=%d output=%s", code, completion.String())
	}
	candidates := completionCandidates(completion.String())
	for _, canonical := range []string{"--environment", "--full", "--id", "--name", "--preview", "--project"} {
		if !candidates[canonical] {
			t.Errorf("completion missing canonical flag %q", canonical)
		}
	}
	for _, shorthand := range []string{"-e", "-f", "-i", "-n", "-p"} {
		if !candidates[shorthand] {
			t.Errorf("completion missing single-letter shorthand %q", shorthand)
		}
	}
	for _, longAlias := range []string{"--env", "--ful", "--nm", "--pv", "--prj"} {
		if candidates[longAlias] {
			t.Errorf("completion unexpectedly advertised normalized long alias %q", longAlias)
		}
	}
}

func TestShorthandConfigAliasBeforeCommandSelectsCanonicalOperation(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("version: 1\nenvironments:\n  selected:\n    url: https://tableau.example.test\n    site_content_url: test\n    auth:\n      type: pat\n      pat_name_env: SELECTED_PAT_NAME\n      pat_secret_env: SELECTED_PAT_SECRET\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	options := app.Options{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml")}
	if code := app.Run(context.Background(), []string{"--cfg", configPath, "env", "ls"}, &stdout, options); code != 0 || !strings.Contains(stdout.String(), "selected") {
		t.Fatalf("leading --cfg did not reach canonical env.profile.list: code=%d output=%s", code, stdout.String())
	}
}

func shorthandOptions(t *testing.T, server *httptest.Server) app.Options {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	contents := fmt.Sprintf("version: 1\ndefault_environment: test\nenvironments:\n  test:\n    url: %s\n    site_content_url: test\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: SHORTHAND_PAT_NAME\n      pat_secret_env: SHORTHAND_PAT_SECRET\n", server.URL)
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHORTHAND_PAT_NAME", "fixture-pat")
	t.Setenv("SHORTHAND_PAT_SECRET", "fixture-secret")
	return app.Options{ConfigPath: configPath, HTTPClient: server.Client()}
}

func completionCandidates(output string) map[string]bool {
	candidates := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		candidate, _, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if ok {
			candidates[candidate] = true
		}
	}
	return candidates
}
