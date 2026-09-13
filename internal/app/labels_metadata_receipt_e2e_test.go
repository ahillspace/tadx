package app_test

import (
	"bytes"
	"context"
	"github.com/ahillspace/tadx/internal/app"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLabelsMetadataAcknowledgedMalformedWriteDoesNotInventAttachmentIdentity(t *testing.T) {
	writes := 0
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if catalogMetadataSignIn(w, r) {
			return
		}
		switch r.URL.Path {
		case "/api/3.29/sites/site-1/labels":
			if r.Method == http.MethodPost {
				io.WriteString(w, `<tsResponse><labelList/></tsResponse>`)
				return
			}
			if r.Method != http.MethodPut {
				t.Errorf("unexpected %s", r.Method)
				w.WriteHeader(500)
				return
			}
			writes++
			io.WriteString(w, `<tsResponse><labelList><label id="unrelated-attachment" contentId="wrong-asset" contentType="table" value="Warning" category="Custom" message="After" active="true" elevated="true"/></labelList></tsResponse>`)
		case "/api/3.29/sites/site-1/labelValues/Warning":
			io.WriteString(w, "<tsResponse>"+valueFixture+"</tsResponse>")
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(500)
		}
	}))
	defer s.Close()
	var out bytes.Buffer
	code := app.Run(context.Background(), []string{"catalog", "label", "update", "--env", "production", "--type", "table", "--target-id", "table-1", "--value", "Warning", "--message", "After", "--json"}, &out, catalogMetadataOptions(t, s, true))
	if code == 0 || writes != 1 || !strings.Contains(out.String(), "confirmed") || !strings.Contains(out.String(), "verification_pending") || strings.Contains(out.String(), "unrelated-attachment") {
		t.Fatalf("code=%d writes=%d %s", code, writes, &out)
	}
}
