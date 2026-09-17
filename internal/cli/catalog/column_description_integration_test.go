package catalog

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	columnupdate "github.com/ahillspace/tadx/actions/catalog/column/update"
	"github.com/ahillspace/tadx/internal/errs"
	catalogresource "github.com/ahillspace/tadx/internal/resources/catalog"
	"github.com/ahillspace/tadx/internal/tableau"
	"github.com/ahillspace/tadx/internal/tableau/metadataassets"
)

type columnDescriptionService struct {
	action *columnupdate.Action
	last   columnupdate.Output
}

func (s *columnDescriptionService) UpdateCatalogColumn(ctx context.Context, in columnupdate.Input, preview bool) (columnupdate.Output, error) {
	in.Environment, in.Site, in.TargetResolved = "fixture", "site", true
	out, err := s.action.Execute(ctx, in, preview)
	s.last = out
	return out, err
}

func TestCLIColumnDescriptionClearPreviewAndExecution(t *testing.T) {
	description := "Original description"
	reads, writes := 0, 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Tableau-Auth") != "fixture-token" {
			t.Error("request did not use the configured session")
		}
		if r.URL.Path != "/api/3.29/sites/site/tables/table/columns/column" {
			t.Errorf("unexpected target: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodGet:
			reads++
		case http.MethodPut:
			writes++
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			var request struct {
				Column struct {
					Description *string `xml:"description,attr"`
				} `xml:"column"`
			}
			if err := xml.Unmarshal(body, &request); err != nil {
				t.Fatal(err)
			}
			if request.Column.Description == nil || *request.Column.Description != "" {
				t.Errorf("clear must send an explicit empty description: %s", body)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if strings.Contains(string(body), "tags") || strings.Contains(string(body), "contact") || strings.Contains(string(body), "name=") {
				t.Errorf("clear modified an unrequested property: %s", body)
			}
			description = ""
		default:
			t.Errorf("unexpected method: %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		fmt.Fprintf(w, `<tsResponse><column id="column" name="Category" parentTableId="table" description="%s" remoteType="WSTR"><tags><tag label="keep-tag"/></tags></column></tsResponse>`, description)
	}))
	defer server.Close()
	adapter := catalogresource.New(metadataassets.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL))
	service := &columnDescriptionService{action: columnupdate.New(adapter, adapter)}
	run := func(preview bool) {
		t.Helper()
		command := New(Dependencies{ColumnUpdater: service, Renderer: &recorder{}})
		args := []string{"column", "update", "--table-id", "table", "--id", "column", "--description="}
		if preview {
			args = append(args, "--preview")
		}
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Fatal(err)
		}
	}
	run(true)
	if reads != 1 || writes != 0 || description != "Original description" {
		t.Fatalf("preview changed state: reads=%d writes=%d description=%q", reads, writes, description)
	}
	if len(service.last.Plan.Changes) != 1 || service.last.Plan.Changes[0].Property != "description" || service.last.Plan.Changes[0].After != "" {
		t.Fatalf("preview did not describe clearing: %+v", service.last)
	}
	run(false)
	if writes != 1 || description != "" || service.last.Result == nil || !slices.Contains(service.last.Result.Completed, "description") {
		t.Fatalf("clear was not confirmed: writes=%d description=%q output=%+v", writes, description, service.last)
	}
	run(false)
	if writes != 1 || !service.last.Plan.NoOp || service.last.Result.Status != "unchanged" {
		t.Fatalf("repeated clear should be a no-op: writes=%d output=%+v", writes, service.last)
	}
}

func TestCLIColumnDescriptionClearReadbackFailureIsConfirmedVerification(t *testing.T) {
	reads, writes := 0, 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			reads++
			io.WriteString(w, `<tsResponse><column id="column" name="Category" parentTableId="table" description="Original description"/></tsResponse>`)
			return
		}
		if r.Method != http.MethodPut {
			t.Errorf("unexpected method: %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writes++
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), `description=""`) {
			t.Errorf("clear did not send an explicit empty description: %s", body)
		}
		w.Header().Set("X-Tableau-Request-Id", "clear-readback-request")
		io.WriteString(w, `<tsResponse><column id="column" name="Category" parentTableId="table"/></tsResponse>`)
	}))
	defer server.Close()

	adapter := catalogresource.New(metadataassets.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL))
	service := &columnDescriptionService{action: columnupdate.New(adapter, adapter)}
	command := New(Dependencies{ColumnUpdater: service, Renderer: &recorder{}})
	command.SetContext(t.Context())
	command.SetArgs([]string{"column", "update", "--table-id", "table", "--id", "column", "--description="})
	err := command.Execute()
	structured, ok := errors.AsType[*errs.Error](err)
	if err == nil || !ok || structured.Outcome != errs.OutcomeConfirmed || structured.Phase != errs.PhaseVerification || structured.TableauRequestID != "clear-readback-request" {
		t.Fatalf("clear readback failure = %v, structured=%+v, reads=%d, writes=%d", err, structured, reads, writes)
	}
	if service.last.Result == nil || service.last.Result.Status != "partial" || service.last.Result.Identity.LUID != "column" || reads != 2 || writes != 1 {
		t.Fatalf("clear readback receipt = %+v, reads=%d, writes=%d", service.last, reads, writes)
	}
}
