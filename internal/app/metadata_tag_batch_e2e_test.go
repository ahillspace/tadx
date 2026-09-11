package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/errs"
)

func TestMetadataTagBatchRetainsPartialSuccessAndContinues(t *testing.T) {
	signins := 0
	writes := []string{}
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if catalogMetadataSignIn(w, r) {
			signins++
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/3.29/sites/site-1/databases/")
		id, tagRequest := strings.CutSuffix(path, "/tags")
		if id != "first" && id != "second" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if r.Method == http.MethodPut {
			writes = append(writes, path)
			if tagRequest {
				if id == "first" {
					w.Header().Set("X-Tableau-Request-Id", "first-tag-request")
					io.WriteString(w, `<tsResponse><tags/></tsResponse>`)
				} else {
					io.WriteString(w, `<tsResponse><tags><tag label="sales"/></tags></tsResponse>`)
				}
				return
			}
		} else if r.Method != http.MethodGet || tagRequest {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		description := "Before"
		if r.Method == http.MethodPut {
			description = "After"
		}
		fmt.Fprintf(w, `<tsResponse><database id="%s" name="Warehouse" description="%s"><tags/></database></tsResponse>`, id, description)
	}))
	defer s.Close()
	var out bytes.Buffer
	code := app.Run(context.Background(), []string{"catalog", "database", "update", "--env", "production", "--id", "first", "--id", "second", "--description", "After", "--add-tag", "sales", "--json"}, &out, catalogMetadataOptions(t, s, true))
	var result struct {
		Status            string
		Succeeded, Failed int
		Items             []struct {
			Status string
			Result struct {
				Result struct {
					Status, Failed string
					Identity       struct{ LUID string }
					Completed      []string
				}
			}
			Error *errs.Payload
		}
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, &out)
	}
	if code == 0 || signins != 1 || result.Status != "partial_failure" || result.Succeeded != 1 || result.Failed != 1 || len(result.Items) != 2 || !slices.Equal(writes, []string{"first", "first/tags", "second", "second/tags"}) {
		t.Fatalf("code=%d signins=%d writes=%v output=%s", code, signins, writes, &out)
	}
	first, second := result.Items[0], result.Items[1]
	if first.Status != "failed" || first.Result.Result.Status != "partial" || first.Result.Result.Identity.LUID != "first" || first.Result.Result.Failed != "add_tags" || !slices.Equal(first.Result.Result.Completed, []string{"description"}) {
		t.Fatalf("first item's confirmed work lost: %s", &out)
	}
	if first.Error == nil || first.Error.Resource != "first" || first.Error.Phase != errs.PhaseVerification || first.Error.Outcome != errs.OutcomeUnknown || first.Error.TableauRequestID != "first-tag-request" || first.Error.Retryable == nil || *first.Error.Retryable || !slices.Equal(first.Error.Completed, []string{"description"}) {
		t.Fatalf("verification failure context lost: %s", &out)
	}
	if second.Status != "succeeded" || second.Result.Result.Status != "updated" || second.Result.Result.Identity.LUID != "second" || !slices.Equal(second.Result.Result.Completed, []string{"description", "add_tags"}) || second.Error != nil {
		t.Fatalf("second item did not complete: %s", &out)
	}
}
