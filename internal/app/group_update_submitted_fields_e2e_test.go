package app_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	groupops "github.com/ahillspace/tadx/actions/admin/group"
	"github.com/ahillspace/tadx/internal/app"
)

func TestGroupUpdateVerifiesEverySubmittedMetadataFieldThroughCLI(t *testing.T) {
	for _, tc := range []struct {
		name, response, field, observed string
		flags                           []string
		unverified                      bool
		initialExternal                 bool
	}{
		{"unchanged-name-mismatch", `name="Different" minimumSiteRole="Creator"`, "name", "Different", []string{"--new-name", "Before", "--minimum-site-role", "Creator"}, false, false},
		{"unchanged-role-mismatch", `name="After" minimumSiteRole="Creator"`, "minimum_site_role", "Creator", []string{"--new-name", "After", "--minimum-site-role", "Viewer"}, false, false},
		{"unchanged-role-omitted", `name="After"`, "minimum_site_role", "", []string{"--new-name", "After", "--minimum-site-role", "Viewer"}, true, false},
		{"unchanged-false-mismatch", `name="After" externalUserEnabled="true"`, "external_user_enabled", "true", []string{"--new-name", "After", "--external-user-enabled=false"}, false, false},
		{"unchanged-false-omitted", `name="After"`, "external_user_enabled", "", []string{"--new-name", "After", "--external-user-enabled=false"}, true, false},
		{"unchanged-true-mismatch", `name="After" externalUserEnabled="false"`, "external_user_enabled", "false", []string{"--new-name", "After", "--external-user-enabled=true"}, false, true},
		{"unchanged-true-omitted", `name="After"`, "external_user_enabled", "", []string{"--new-name", "After", "--external-user-enabled=true"}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metadataWrites, membershipWrites := 0, 0
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				switch {
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/groups"):
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="1"/><groups><group id="group-1" name="Before" minimumSiteRole="Viewer" externalUserEnabled="%t"/></groups></tsResponse>`, r.URL.Query().Get("pageSize"), tc.initialExternal)
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/groups/group-1/users"):
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="0"/><users/></tsResponse>`, r.URL.Query().Get("pageSize"))
				case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/groups/group-1"):
					metadataWrites++
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					attribute := map[string]string{"name": `name="Before"`, "minimum_site_role": `minimumSiteRole="Viewer"`, "external_user_enabled": fmt.Sprintf(`externalUserEnabled="%t"`, tc.initialExternal)}[tc.field]
					if !strings.Contains(string(body), attribute) {
						t.Errorf("explicit unchanged field was not submitted: %s", body)
					}
					fmt.Fprintf(w, `<tsResponse><group id="group-1" %s/></tsResponse>`, tc.response)
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/groups/group-1/users"):
					membershipWrites++
					io.WriteString(w, `<tsResponse><user id="new-user" name="New"/></tsResponse>`)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer server.Close()
			args := []string{"admin", "group", "update", "--environment", "test", "--id", "group-1", "--set-members", "--member-id", "new-user", "--json", "--full"}
			args = append(args, tc.flags...)
			var output strings.Builder
			code := app.Run(t.Context(), args, &output, diagnosticOptions(t, server))
			if code == 0 || metadataWrites != 1 || membershipWrites != 0 {
				t.Fatalf("code=%d metadata writes=%d membership writes=%d output=%s", code, metadataWrites, membershipWrites, output.String())
			}
			var document struct {
				Error struct {
					ID, Phase, Outcome string
					Retryable          *bool
				}
				Output groupops.UpdateOutput
			}
			if err := json.Unmarshal([]byte(output.String()), &document); err != nil {
				t.Fatal(err)
			}
			result := document.Output.Result
			if document.Error.ID != "admin.group.update.verification" || document.Error.Phase != "verification" || document.Error.Outcome != "unknown" || document.Error.Retryable == nil || *document.Error.Retryable || result == nil || result.Group == nil || result.Status != "partial" || result.Added != 0 || result.Removed != 0 {
				t.Fatalf("verification receipt=%+v error=%+v", result, document.Error)
			}
			if tc.unverified {
				if !slices.Equal(result.UnverifiedSettings, []string{tc.field}) || len(result.MismatchedSettings) != 0 {
					t.Fatalf("unverified fields lost: %+v", result)
				}
			} else if !slices.Equal(result.MismatchedSettings, []string{tc.field}) || len(result.UnverifiedSettings) != 0 {
				t.Fatalf("mismatched fields lost: %+v", result)
			}
			observed := ""
			switch tc.field {
			case "name":
				observed = result.Group.Name
			case "minimum_site_role":
				observed = result.Group.MinimumSiteRole
			case "external_user_enabled":
				if result.Group.ExternalUserEnabled != nil {
					observed = fmt.Sprint(*result.Group.ExternalUserEnabled)
				}
			}
			if observed != tc.observed {
				t.Fatalf("observed %s=%q want %q", tc.field, observed, tc.observed)
			}
		})
	}
}
