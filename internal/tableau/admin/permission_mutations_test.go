package admin_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/tableau"
	admin "github.com/ahillspace/tadx/internal/tableau/admin"
)

func TestPermissionMutationMethodsPathsAndPayloads(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow", "project"} {
		for _, principal := range []string{"user", "group"} {
			for _, defaultFor := range []string{"", "workbooks", "datasources", "flows"} {
				if defaultFor != "" && kind != "project" {
					continue
				}
				t.Run(kind+"/"+principal+"/"+defaultFor, func(t *testing.T) {
					request := admin.PermissionMutationRequest{PermissionRequest: admin.PermissionRequest{ResourceKind: kind, ResourceLUID: "resource /1", DefaultFor: defaultFor}, Rule: admin.PermissionRule{PrincipalType: principal, PrincipalLUID: "principal /1", Capability: "Read", Mode: "Allow"}}
					base := "/api/3.29/sites/site-1/" + kind + "s/resource%20%2F1/permissions"
					if defaultFor != "" {
						base = "/api/3.29/sites/site-1/projects/resource%20%2F1/default-permissions/" + defaultFor
					}
					calls := 0
					server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls++
						w.Header().Set("X-Tableau-Request-Id", "permission-request")
						switch r.Method {
						case http.MethodPut:
							if r.URL.EscapedPath() != base {
								t.Errorf("path=%s want=%s", r.URL.EscapedPath(), base)
							}
							body, _ := io.ReadAll(r.Body)
							want := fmt.Sprintf("<tsRequest><permissions><granteeCapabilities><%s id=\"principal /1\"></%s><capabilities><capability name=\"Read\" mode=\"Allow\"></capability></capabilities></granteeCapabilities></permissions></tsRequest>", principal, principal)
							if kind == "flow" && defaultFor == "" {
								want = strings.Replace(want, "<permissions>", "<permissions><flow id=\"resource /1\"></flow>", 1)
							}
							if string(body) != want {
								t.Errorf("body=%s want=%s", body, want)
							}
							fmt.Fprintf(w, "<tsResponse><permissions><granteeCapabilities><%s id=\"principal /1\"/><capabilities><capability name=\"Read\" mode=\"Allow\"/></capabilities></granteeCapabilities></permissions></tsResponse>", principal)
						case http.MethodDelete:
							want := base + "/" + principal + "s/principal%20%2F1/Read/Allow"
							if r.URL.EscapedPath() != want {
								t.Errorf("path=%s want=%s", r.URL.EscapedPath(), want)
							}
							w.WriteHeader(http.StatusNoContent)
						default:
							t.Errorf("method=%s", r.Method)
						}
					}))
					defer server.Close()
					client := admin.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
					created, err := client.CreatePermission(context.Background(), request)
					if err != nil || created.Status != "created" || created.RequestID != "permission-request" {
						t.Fatalf("created=%+v err=%v", created, err)
					}
					deleted, err := client.DeletePermission(context.Background(), request)
					if err != nil || deleted.Status != "deleted" || calls != 2 {
						t.Fatalf("deleted=%+v err=%v calls=%d", deleted, err, calls)
					}
				})
			}
		}
	}
}

func TestPermissionMutationRejectsMalformedSuccessfulResponse(t *testing.T) {
	for _, body := range []string{"", "<tsResponse/>", "<unexpected><permissions/></unexpected>", "<tsResponse><permissions><granteeCapabilities><user id=\"u1\"/><capabilities><capability name=\"Read\" mode=\"Deny\"/></capabilities></granteeCapabilities></permissions></tsResponse>", "<tsResponse><permissions><workbook id=\"other\"/><granteeCapabilities><user id=\"u1\"/><capabilities><capability name=\"Read\" mode=\"Allow\"/></capabilities></granteeCapabilities></permissions></tsResponse>"} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Tableau-Request-Id", "unknown-request")
			fmt.Fprint(w, body)
		}))
		client := admin.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
		result, err := client.CreatePermission(context.Background(), admin.PermissionMutationRequest{PermissionRequest: admin.PermissionRequest{ResourceKind: "workbook", ResourceLUID: "w1"}, Rule: admin.PermissionRule{PrincipalType: "user", PrincipalLUID: "u1", Capability: "Read", Mode: "Allow"}})
		server.Close()
		if err == nil || result.Status != "unknown" || tableau.RequestID(err) != "unknown-request" {
			t.Fatalf("body=%s result=%+v err=%v", body, result, err)
		}
	}
}

func TestPermissionMutationRejectsInvalidInputBeforeHTTP(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer server.Close()
	client := admin.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	for _, rule := range []admin.PermissionRule{{PrincipalType: "role", PrincipalLUID: "u1", Capability: "Read", Mode: "Allow"}, {PrincipalType: "user", PrincipalLUID: "", Capability: "Read", Mode: "Allow"}, {PrincipalType: "user", PrincipalLUID: "u1", Capability: "Read", Mode: "allow"}, {PrincipalType: "user", PrincipalLUID: "u1", Capability: "MadeUp", Mode: "Allow"}} {
		in := admin.PermissionMutationRequest{PermissionRequest: admin.PermissionRequest{ResourceKind: "project", ResourceLUID: "p1"}, Rule: rule}
		if _, err := client.CreatePermission(context.Background(), in); err == nil {
			t.Fatal("invalid create accepted")
		}
		if _, err := client.DeletePermission(context.Background(), in); err == nil {
			t.Fatal("invalid delete accepted")
		}
	}
	if calls != 0 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestPermissionMutationPreservesUpstreamFailureWithoutRetry(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("X-Tableau-Request-Id", "forbidden-request")
		w.WriteHeader(403)
		io.WriteString(w, "<tsResponse><error code=\"403004\"><summary>Forbidden</summary><detail>Permissions denied</detail></error></tsResponse>")
	}))
	defer server.Close()
	client := admin.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	_, err := client.CreatePermission(context.Background(), admin.PermissionMutationRequest{PermissionRequest: admin.PermissionRequest{ResourceKind: "project", ResourceLUID: "p1"}, Rule: admin.PermissionRule{PrincipalType: "user", PrincipalLUID: "u1", Capability: "Read", Mode: "Allow"}})
	if err == nil || !strings.Contains(err.Error(), "403004") || tableau.RequestID(err) != "forbidden-request" || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestPermissionReadRejectsInvalidSnapshotBeforePlanning(t *testing.T) {
	for _, body := range []string{
		"<tsResponse/>",
		"<tsResponse><permissions/><permissions/></tsResponse>",
		"<tsResponse><permissions><workbook id=\"other\"/></permissions></tsResponse>",
		"<tsResponse><permissions><granteeCapabilities><group id=\"g1\"/><capabilities><capability name=\"Read\" mode=\"Allow\"/><capability name=\"Read\" mode=\"Deny\"/></capabilities></granteeCapabilities></permissions></tsResponse>",
	} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, body)
		}))
		client := admin.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
		_, err := client.GetPermissions(context.Background(), admin.PermissionRequest{ResourceKind: "workbook", ResourceLUID: "w1"})
		server.Close()
		if err == nil {
			t.Fatalf("invalid snapshot accepted: %s", body)
		}
	}
}
