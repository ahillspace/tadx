package app_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

type flagValidationTransport struct{ calls *int }

func (r flagValidationTransport) RoundTrip(*http.Request) (*http.Response, error) {
	*r.calls++
	return nil, fmt.Errorf("unexpected request")
}

func TestCobraFlagConflictsAreUsageErrorsBeforeSetup(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow", "project"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			var text bytes.Buffer
			exit := app.Run(context.Background(), []string{"content", kind, "list", "--all", "--limit", "1"}, &text, app.Options{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml"), HTTPClient: &http.Client{Transport: flagValidationTransport{&calls}}})
			if exit != 2 || calls != 0 || !strings.Contains(text.String(), "kind: usage") || !strings.Contains(text.String(), "all limit") {
				t.Fatalf("exit=%d calls=%d output=%s", exit, calls, text.String())
			}
		})
	}
}
