package admin_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/tableau"
	admin "github.com/ahillspace/tadx/internal/tableau/admin"
)

func TestClientRejectsWrongSuccessfulReadStatusWithRequestID(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Tableau-Request-Id", "read-uncertain")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := admin.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	checks := []func() error{
		func() error {
			_, err := client.ListUsers(context.Background(), admin.ListUsersRequest{PageNumber: 1, PageSize: 1})
			return err
		},
		func() error { _, err := client.GetUser(context.Background(), "user-1"); return err },
		func() error {
			_, err := client.ListGroups(context.Background(), admin.ListGroupsRequest{PageNumber: 1, PageSize: 1})
			return err
		},
		func() error {
			_, err := client.ListGroupUsers(context.Background(), "group-1", admin.PageRequest{PageNumber: 1, PageSize: 1})
			return err
		},
		func() error {
			_, err := client.GetPermissions(context.Background(), admin.PermissionRequest{ResourceKind: "workbook", ResourceLUID: "workbook-1"})
			return err
		},
	}
	for index, check := range checks {
		err := check()
		if err == nil || tableau.RequestID(err) != "read-uncertain" || !strings.Contains(err.Error(), "expected 200") {
			t.Errorf("check %d error = %v, request ID = %q", index, err, tableau.RequestID(err))
		}
	}
}

func TestClientRejectsWrongMutationStatusesAsUnknown(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Tableau-Request-Id", "mutation-uncertain")
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/groups/"):
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut:
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		}
		_, _ = io.WriteString(w, `<tsResponse><user id="user-1" name="alex"/><group id="group-1" name="Authors"/></tsResponse>`)
	}))
	defer server.Close()
	client := admin.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	user, err := client.CreateUser(context.Background(), admin.CreateUserRequest{Name: "alex", SiteRole: "Viewer", AuthSetting: "SAML"})
	if err == nil || user.MutationStatus != "unknown" || user.RequestID != "mutation-uncertain" || tableau.RequestID(err) != "mutation-uncertain" {
		t.Fatalf("CreateUser() = %#v, %v", user, err)
	}
	user, err = client.UpdateUser(context.Background(), "user-1", admin.UpdateUserRequest{SiteRole: admin.String("Explorer")})
	if err == nil || user.MutationStatus != "unknown" || user.RequestID != "mutation-uncertain" {
		t.Fatalf("UpdateUser() = %#v, %v", user, err)
	}
	deleted, err := client.DeleteUser(context.Background(), "user-1")
	if err == nil || deleted.Status != "unknown" || deleted.RequestID != "mutation-uncertain" {
		t.Fatalf("DeleteUser() = %#v, %v", deleted, err)
	}
	group, err := client.CreateGroup(context.Background(), admin.CreateGroupRequest{Name: "Authors"})
	if err == nil || group.MutationStatus != "unknown" || group.RequestID != "mutation-uncertain" {
		t.Fatalf("CreateGroup() = %#v, %v", group, err)
	}
	group, err = client.UpdateGroup(context.Background(), "group-1", admin.UpdateGroupRequest{Name: admin.String("Writers")})
	if err == nil || group.MutationStatus != "unknown" || group.RequestID != "mutation-uncertain" {
		t.Fatalf("UpdateGroup() = %#v, %v", group, err)
	}
	deleted, err = client.DeleteGroup(context.Background(), "group-1")
	if err == nil || deleted.Status != "unknown" || deleted.RequestID != "mutation-uncertain" {
		t.Fatalf("DeleteGroup() = %#v, %v", deleted, err)
	}
	member, err := client.AddGroupUser(context.Background(), "group-1", "user-1")
	if err == nil || member.Status != "unknown" || member.RequestID != "mutation-uncertain" {
		t.Fatalf("AddGroupUser() = %#v, %v", member, err)
	}
	member, err = client.RemoveGroupUser(context.Background(), "group-1", "user-1")
	if err == nil || member.Status != "unknown" || member.RequestID != "mutation-uncertain" {
		t.Fatalf("RemoveGroupUser() = %#v, %v", member, err)
	}
}

func TestClientRejectsBodiesForEmptyDeleteResponses(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Tableau-Request-Id", "body-uncertain")
		w.WriteHeader(http.StatusNoContent)
		// net/http suppresses a body for HTTP 204, so advertise an invalid body length.
		w.Header().Set("Content-Length", "1")
	}))
	defer server.Close()
	client := admin.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	// A custom transport is required because net/http normalizes 204 response bodies.
	client = admin.NewClient(tableau.NewTransport(&http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNoContent, Header: http.Header{"X-Tableau-Request-Id": []string{"body-uncertain"}}, Body: io.NopCloser(strings.NewReader("x"))}, nil
	})}, "3.29", nil), session{}, "https://tableau.example")
	for _, call := range []func() (admin.MutationResult, error){func() (admin.MutationResult, error) { return client.DeleteUser(context.Background(), "user-1") }, func() (admin.MutationResult, error) { return client.DeleteGroup(context.Background(), "group-1") }, func() (admin.MutationResult, error) {
		return client.RemoveGroupUser(context.Background(), "group-1", "user-1")
	}} {
		result, err := call()
		if err == nil || result.Status != "unknown" || result.RequestID != "body-uncertain" {
			t.Errorf("result = %#v, error = %v", result, err)
		}
	}
}

func TestClientRejectsEmptyOrMismatchedMemberAddResponse(t *testing.T) {
	responses := []string{"", `<tsResponse><user id="other" name="alex"/></tsResponse>`}
	for _, body := range responses {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-Tableau-Request-Id", "member-uncertain")
			_, _ = io.WriteString(w, body)
		}))
		client := admin.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
		result, err := client.AddGroupUser(context.Background(), "group-1", "user-1")
		server.Close()
		if err == nil || result.Status != "unknown" || result.RequestID != "member-uncertain" {
			t.Errorf("body %q result = %#v, error = %v", body, result, err)
		}
	}
}

func TestClientAddsGroupMemberWithDocumentedSingleEnvelope(t *testing.T) {
	var body string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/3.29/sites/site-1/groups/group-1/users" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		_, _ = io.WriteString(w, `<tsResponse><user id="user-1" name="alex" siteRole="Viewer"/></tsResponse>`)
	}))
	defer server.Close()
	client := admin.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	if _, err := client.AddGroupUser(context.Background(), "group-1", "user-1"); err != nil {
		t.Fatal(err)
	}
	want := `<tsRequest><user id="user-1"></user></tsRequest>`
	if body != want {
		t.Fatalf("request body = %q, want %q", body, want)
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type session struct{}

func (session) String() string          { return "admin test session" }
func (session) Authorize(*http.Request) {}
func (session) SiteLUID() string        { return "site-1" }
func (session) UserLUID() string        { return "caller-1" }

var _ coreauth.Session = session{}

func TestClientListsAndGetsUsers(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Tableau-Request-Id", "request-1")
		switch r.URL.Path {
		case "/api/3.29/sites/site-1/users":
			if r.URL.Query().Get("pageNumber") != "2" || r.URL.Query().Get("pageSize") != "25" || !strings.Contains(r.URL.Query().Get("filter"), "siteRole:eq:Viewer") {
				t.Errorf("query = %q", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="2" pageSize="25" totalAvailable="26"/><users><user id="user-1" name="alex@example.com" fullName="Alex" email="notify@example.com" siteRole="Viewer" authSetting="SAML"><domain name="example"/></user></users></tsResponse>`)
		case "/api/3.29/sites/site-1/users/user-1":
			_, _ = io.WriteString(w, `<tsResponse><user id="user-1" name="alex@example.com" siteRole="Viewer"><domain name="example"/></user></tsResponse>`)
		default:
			t.Errorf("path = %q", r.URL.Path)
		}
	}))
	defer server.Close()
	client := admin.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	page, err := client.ListUsers(context.Background(), admin.ListUsersRequest{PageNumber: 2, PageSize: 25, SiteRole: "Viewer"})
	if err != nil || len(page.Items) != 1 || page.Items[0].LUID != "user-1" || page.Items[0].Domain != "example" || page.RequestID != "request-1" {
		t.Fatalf("ListUsers() = %#v, %v", page, err)
	}
	user, err := client.GetUser(context.Background(), "user-1")
	if err != nil || user.LUID != "user-1" {
		t.Fatalf("GetUser() = %#v, %v", user, err)
	}
}

func TestClientListsGroupsAndMembership(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/3.29/sites/site-1/groups":
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="10" totalAvailable="1"/><groups><group id="group-1" name="Authors" minimumSiteRole="Explorer"><domain name="local"/><import grantLicenseMode="onLogin" siteRole="Explorer"/></group></groups></tsResponse>`)
		case "/api/3.29/sites/site-1/groups/group-1/users":
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="10" totalAvailable="1"/><users><user id="user-1" name="alex" siteRole="Viewer"/></users></tsResponse>`)
		default:
			t.Errorf("path = %q", r.URL.Path)
		}
	}))
	defer server.Close()
	client := admin.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	groups, err := client.ListGroups(context.Background(), admin.ListGroupsRequest{PageNumber: 1, PageSize: 10})
	if err != nil || len(groups.Items) != 1 || groups.Items[0].GrantLicenseMode != "onLogin" {
		t.Fatalf("ListGroups() = %#v, %v", groups, err)
	}
	members, err := client.ListGroupUsers(context.Background(), "group-1", admin.PageRequest{PageNumber: 1, PageSize: 10})
	if err != nil || len(members.Items) != 1 || members.Items[0].LUID != "user-1" {
		t.Fatalf("ListGroupUsers() = %#v, %v", members, err)
	}
}

func TestClientWritesExplicitUsersAndGroups(t *testing.T) {
	var requests []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests = append(requests, r.Method+" "+r.URL.Path+" "+string(body))
		w.Header().Set("X-Tableau-Request-Id", "mutation-1")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/3.29/sites/site-1/users":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `<tsResponse><user id="user-1" name="alex@example.com" siteRole="Viewer" authSetting="SAML"/></tsResponse>`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/3.29/sites/site-1/users/user-1":
			_, _ = io.WriteString(w, `<tsResponse><user name="alex@example.com" fullName="Alex" siteRole="Explorer"/></tsResponse>`)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/api/3.29/sites/site-1/groups":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `<tsResponse><group id="group-1" name="Authors"/></tsResponse>`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/3.29/sites/site-1/groups/group-1":
			_, _ = io.WriteString(w, `<tsResponse><group id="group-1" name="Writers"/></tsResponse>`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/3.29/sites/site-1/groups/group-1/users":
			_, _ = io.WriteString(w, `<tsResponse><user id="user-1" name="alex"/></tsResponse>`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client := admin.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	if _, err := client.CreateUser(context.Background(), admin.CreateUserRequest{Name: "alex@example.com", SiteRole: "Viewer", AuthSetting: "SAML"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.UpdateUser(context.Background(), "user-1", admin.UpdateUserRequest{FullName: admin.String("Alex"), SiteRole: admin.String("Explorer")}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DeleteUser(context.Background(), "user-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateGroup(context.Background(), admin.CreateGroupRequest{Name: "Authors", MinimumSiteRole: "Viewer"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.UpdateGroup(context.Background(), "group-1", admin.UpdateGroupRequest{Name: admin.String("Writers")}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.AddGroupUser(context.Background(), "group-1", "user-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.RemoveGroupUser(context.Background(), "group-1", "user-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DeleteGroup(context.Background(), "group-1"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(requests, "\n")
	for _, want := range []string{`siteRole="Viewer"`, `authSetting="SAML"`, `fullName="Alex"`, `<user id="user-1"></user>`} {
		if !strings.Contains(joined, want) {
			t.Errorf("requests missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "mapAssetsTo") {
		t.Fatalf("user delete inferred ownership transfer: %s", joined)
	}
}

func TestClientNormalizesPermissionFacts(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/3.29/sites/site-1/workbooks/workbook-1/permissions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = io.WriteString(w, `<tsResponse><permissions><parent type="Project" id="project-1"/><workbook id="workbook-1"/><granteeCapabilities><group id="group-1"/><capabilities><capability name="Read" mode="Allow"/></capabilities></granteeCapabilities></permissions></tsResponse>`)
	}))
	defer server.Close()
	client := admin.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	result, err := client.GetPermissions(context.Background(), admin.PermissionRequest{ResourceKind: "workbook", ResourceLUID: "workbook-1"})
	if err != nil || result.Source != "inherited" || len(result.Rules) != 1 || result.Rules[0].PrincipalType != "group" {
		t.Fatalf("GetPermissions() = %#v, %v", result, err)
	}
}
