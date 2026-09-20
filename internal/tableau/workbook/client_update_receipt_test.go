package workbook_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ahillspace/tadx/internal/tableau"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

func TestClientReportsOnlyDescriptionConfirmedByCompleteUpdateResponse(t *testing.T) {
	for _, test := range []struct {
		name                string
		description         *string
		newName             *string
		responseDescription string
		responseName        string
		responseLUID        string
		wantDescription     *string
		wantEvidence        string
		wantError           bool
	}{
		{name: "matching", description: new("Confirmed"), responseDescription: ` description="Confirmed"`, responseName: "Finance", responseLUID: "wb-1", wantDescription: new("Confirmed"), wantEvidence: "tableau_update_response"},
		{name: "cleared", description: new(""), responseDescription: ` description=""`, responseName: "Finance", responseLUID: "wb-1", wantDescription: new(""), wantEvidence: "tableau_update_response"},
		{name: "missing", description: new("Confirmed"), responseName: "Finance", responseLUID: "wb-1", wantError: true},
		{name: "mismatched", description: new("Confirmed"), responseDescription: ` description="Other"`, responseName: "Finance", responseLUID: "wb-1", wantError: true},
		{name: "identity mismatch after matching description", description: new("Confirmed"), responseDescription: ` description="Confirmed"`, responseName: "Finance", responseLUID: "wb-2", wantError: true},
		{name: "requested name mismatch after matching description", description: new("Confirmed"), newName: new("Renamed"), responseDescription: ` description="Confirmed"`, responseName: "Finance", responseLUID: "wb-1", wantError: true},
		{name: "description not requested", newName: new("Renamed"), responseDescription: ` description="Server value"`, responseName: "Renamed", responseLUID: "wb-1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				requests++
				if request.Method != http.MethodPut || request.URL.Path != "/api/3.29/sites/site-1/workbooks/wb-1" {
					t.Errorf("unexpected request %s %s", request.Method, request.URL.Path)
				}
				writer.Header().Set("Content-Type", "application/xml")
				writer.Header().Set("X-Tableau-Request-Id", "request-update")
				_, _ = io.WriteString(writer, `<tsResponse><workbook id="`+test.responseLUID+`" name="`+test.responseName+`"`+test.responseDescription+`><project id="project-1"/><owner id="owner-1"/></workbook></tsResponse>`)
			}))
			defer server.Close()

			client := tableauworkbook.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
			result, err := client.Update(t.Context(), tableauworkbook.UpdateRequest{LUID: "wb-1", Description: test.description, Name: test.newName})
			if requests != 1 {
				t.Fatalf("requests=%d", requests)
			}
			if (err != nil) != test.wantError {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if result.WorkbookLUID != "wb-1" || result.TableauRequestID != "request-update" {
				t.Fatalf("lost retained IDs: %#v", result)
			}
			if test.wantDescription == nil {
				if result.Description != nil || result.EvidenceSource != "" {
					t.Fatalf("unverified description exposed: %#v", result)
				}
				return
			}
			if result.Description == nil || *result.Description != *test.wantDescription || result.EvidenceSource != test.wantEvidence {
				t.Fatalf("confirmed description missing: %#v", result)
			}
		})
	}
}
