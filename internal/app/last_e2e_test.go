package app_test

import (
	"bytes"
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/app"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

type noLastNetwork struct{ calls int }

func (n *noLastNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	n.calls++
	return nil, errors.New("last must not contact Tableau")
}

func TestLastDisplaysSavedFullResultWithoutReplacingIt(t *testing.T) {
	network := &noLastNetwork{}
	opts := app.Options{ConfigPath: filepath.Join(t.TempDir(), "config.yaml"), HTTPClient: &http.Client{Transport: network}}
	run := func(args ...string) (int, string) {
		var out bytes.Buffer
		code := app.Run(context.Background(), args, &out, opts)
		return code, out.String()
	}
	if code, _ := run("last"); code == 0 {
		t.Fatal("missing last result succeeded")
	}
	if code, out := run("capability", "get", "workbook.publish"); code != 0 {
		t.Fatal(out)
	}
	code, first := run("last")
	if code != 0 || !strings.Contains(first, "recorded_at") || !strings.Contains(first, "workbook.publish") {
		t.Fatal(code, first)
	}
	_, second := run("last")
	if first != second {
		t.Fatal("last replaced its own receipt")
	}
	if code, full := run("last", "--full"); code != 0 || full != first {
		t.Fatalf("full changed saved result: %d %s", code, full)
	}
	if code, _ := run("capability", "get", "not-a-capability"); code == 0 {
		t.Fatal("unknown capability succeeded")
	}
	if code, failed := run("last"); code != 0 || !strings.Contains(failed, "error:") || strings.Contains(failed, "exit_code: 0") || !strings.Contains(failed, "not-a-capability") {
		t.Fatalf("failed command result lost: %d %s", code, failed)
	}
	if network.calls != 0 {
		t.Fatalf("last contacted Tableau %d times", network.calls)
	}
}
