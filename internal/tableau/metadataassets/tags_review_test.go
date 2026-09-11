package metadataassets

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/tableau"
)

func TestAddTagsRequiresEveryRequestedAcknowledgment(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		wantError      bool
	}{
		{"empty", `<tsResponse><tags/></tsResponse>`, true},
		{"partial", `<tsResponse><tags><tag label="sales"/></tags></tsResponse>`, true},
		{"unrelated", `<tsResponse><tags><tag label="other"/></tags></tsResponse>`, true},
		{"missing", `<tsResponse/>`, true},
		{"complete", `<tsResponse><tags><tag label="sales"/><tag label="retail"/></tags></tsResponse>`, false},
		{"superset", `<tsResponse><tags><tag label="other"/><tag label="retail"/><tag label="sales"/></tags></tsResponse>`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodPut || r.URL.Path != "/api/3.29/sites/site-1/tables/table-1/tags" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
				}
				w.Header().Set("X-Tableau-Request-Id", "tag-ack-request")
				io.WriteString(w, tc.response)
			})
			got, err := c.AddTags(context.Background(), LabelTarget{Type: "table", LUID: "table-1"}, []string{"sales", "retail"})
			if (err != nil) != tc.wantError || calls != 1 {
				t.Fatalf("tags=%v error=%v calls=%d", got, err, calls)
			}
			if tc.wantError {
				var protocol *tableau.ProtocolError
				var verification interface{ VerificationFailed() bool }
				if !errors.As(err, &protocol) || protocol.Retryable() || protocol.RequestID() != "tag-ack-request" || !errors.As(err, &verification) || !verification.VerificationFailed() {
					t.Fatalf("verification evidence lost: %v", err)
				}
				if tc.name == "partial" && !slices.Contains(got, "sales") {
					t.Fatalf("acknowledged tags discarded: %v", got)
				}
			}
		})
	}
}

func TestAddTagsCountsUnicodeCharacters(t *testing.T) {
	for _, tc := range []struct {
		name, tag string
		valid     bool
	}{
		{"ascii128", strings.Repeat("a", 128), true},
		{"ascii129", strings.Repeat("a", 129), false},
		{"unicode128", strings.Repeat("界", 128), true},
		{"unicode129", strings.Repeat("界", 129), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				io.WriteString(w, `<tsResponse><tags><tag label="`+tc.tag+`"/></tags></tsResponse>`)
			})
			_, err := c.AddTags(context.Background(), LabelTarget{Type: "table", LUID: "table-1"}, []string{tc.tag})
			if (err == nil) != tc.valid || (!tc.valid && calls != 0) {
				t.Fatalf("error=%v requests=%d", err, calls)
			}
		})
	}
}
