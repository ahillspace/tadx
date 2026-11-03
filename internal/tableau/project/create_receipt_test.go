package project_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ahillspace/tadx/internal/tableau"
	tableauproject "github.com/ahillspace/tadx/internal/tableau/project"
)

func TestCreateKeepsDecodedProjectOnVerificationFailure(t *testing.T) {
	for _, attrs := range []string{
		`name="Observed" parentProjectId="parent" description="Requested" contentPermissions="LockedToProject"`,
		`name="Requested" parentProjectId="different-parent" description="Requested" contentPermissions="LockedToProject"`,
		`name="Requested" parentProjectId="parent" description="Observed" contentPermissions="LockedToProject"`,
		`name="Requested" parentProjectId="parent" description="Requested" contentPermissions="ManagedByOwner"`,
	} {
		t.Run(attrs, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Tableau-Request-Id", "acknowledged-create")
				w.WriteHeader(http.StatusCreated)
				io.WriteString(w, `<tsResponse><project id="created" `+attrs+`/></tsResponse>`)
			}))
			defer server.Close()
			client := tableauproject.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
			result, err := client.Create(t.Context(), tableauproject.CreateRequest{Name: "Requested", ParentLUID: "parent", Description: "Requested", ContentPermissions: "LockedToProject"})
			if err == nil || result.Status != "partial" || result.Project.LUID != "created" || result.TableauRequestID != "acknowledged-create" {
				t.Fatalf("result=%+v error=%v", result, err)
			}
		})
	}
}
