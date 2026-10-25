package harness

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestControlledUserLoginUsesExactTemplateAndCaseRoleToken(t *testing.T) {
	c := controlledUserCase(t, "controlled+{prefix}@example.test")
	first, err := c.ControlledUserLogin("subject")
	if err != nil {
		t.Fatal(err)
	}
	retry, err := c.ControlledUserLogin("subject")
	if err != nil {
		t.Fatal(err)
	}
	otherRole, err := c.ControlledUserLogin("search")
	if err != nil {
		t.Fatal(err)
	}
	if first != retry || first == otherRole || !strings.HasPrefix(first, "controlled+") || !strings.HasSuffix(first, "@example.test") {
		t.Fatalf("controlled logins are not deterministic exact-template case/role identities")
	}
	if strings.HasPrefix(first, c.NamePrefix) {
		t.Fatal("approved email alias unexpectedly relies on the internal owner-key prefix")
	}
	if err := ValidateControlledUserTemplate("{prefix}@example.test"); err != nil {
		t.Fatalf("generic owned email template became incompatible: %v", err)
	}
}

func TestControlledUserLoginRejectsUnsafeTemplatesAndWrongRenderedAddresses(t *testing.T) {
	for name, template := range map[string]string{
		"bare address":         "base@example.test",
		"missing placeholder":  "user{suffix}@example.test",
		"repeated placeholder": "{prefix}+{prefix}@example.test",
		"overlong local part":  strings.Repeat("a", 50) + "+{prefix}@example.test",
		"overlong address":     "{prefix}@" + strings.Repeat("a", 230) + ".test",
	} {
		t.Run(name, func(t *testing.T) {
			c := controlledUserCase(t, template)
			if _, err := c.ControlledUserLogin("subject"); ErrorStatus(err) != BlockedStatus {
				t.Fatalf("unsafe template status: %v", err)
			}
		})
	}
	c := controlledUserCase(t, "controlled+{prefix}@example.test")
	login, err := c.ControlledUserLogin("subject")
	if err != nil {
		t.Fatal(err)
	}
	token := strings.TrimSuffix(strings.TrimPrefix(login, "controlled+"), "@example.test")
	if err := c.Journal.AddControlledUserIntent(c.NamePrefix+"-user-subject", "unrelated@example.test", c.Profile.Bindings["controlled_user"].Name, token); err == nil {
		t.Fatal("unrelated address was accepted for a generated controlled user intent")
	}
}

func TestControlledUserSeedUsesExactLoginIntentAndReceipt(t *testing.T) {
	c := controlledUserCase(t, "controlled+{prefix}@example.test")
	createdLogin := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site/users":
			_ = json.NewEncoder(w).Encode(map[string]any{"users": map[string]any{"user": []any{}}, "pagination": map[string]any{"totalAvailable": "0"}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/3.29/sites/site/users":
			var request struct {
				User map[string]any `json:"user"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			createdLogin, _ = request.User["name"].(string)
			_ = json.NewEncoder(w).Encode(map[string]any{"user": map[string]any{"id": "created-user"}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site/users/created-user":
			_ = json.NewEncoder(w).Encode(map[string]any{"user": map[string]any{"id": "created-user", "name": createdLogin, "siteRole": "Unlicensed", "authSetting": "ServerDefault"}})
		default:
			t.Errorf("unexpected controlled user request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	c.API = &Client{Server: server.URL, Version: "3.29", SiteID: "site", UserID: "actor-user", HTTP: server.Client()}
	expected, err := c.ControlledUserLogin("subject")
	if err != nil {
		t.Fatal(err)
	}
	user, err := c.Seed(t.Context(), "user", "subject")
	if err != nil {
		t.Fatal(err)
	}
	if createdLogin != expected || user.Name != expected || user.ID != "created-user" {
		t.Fatalf("Seed user identity differs from the approved template render")
	}
	if len(c.Journal.Entries) != 1 || c.Journal.Entries[0].Name != c.NamePrefix+"-user-subject" || c.Journal.Entries[0].UserLogin != expected || c.Journal.Entries[0].Object.ID != user.ID {
		t.Fatal("Seed user did not retain exact alias ownership and authoritative ID")
	}
	reloaded, err := LoadJournal(filepath.Join(c.RunDir, "journal.json"), c.TargetFingerprint)
	if err != nil || len(reloaded.Entries) != 1 || reloaded.Entries[0].UserLogin != expected {
		t.Fatalf("controlled user journal did not reload safely: %v", err)
	}
}

func TestSeedRefusesExistingControlledUserBeforeIntent(t *testing.T) {
	c := controlledUserCase(t, "controlled+{prefix}@example.test")
	existing := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/3.29/sites/site/users" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"users": map[string]any{"user": []any{map[string]any{"id": "existing", "name": existing}}}, "pagination": map[string]any{"totalAvailable": "1"}})
	}))
	t.Cleanup(server.Close)
	c.API = &Client{Server: server.URL, Version: "3.29", SiteID: "site", UserID: "actor-user", HTTP: server.Client()}
	existing, _ = c.ControlledUserLogin("subject")
	if _, err := c.Seed(t.Context(), "user", "subject"); ErrorStatus(err) != BlockedStatus {
		t.Fatalf("existing controlled login was adopted: %v", err)
	}
	if len(c.Journal.Entries) != 0 {
		t.Fatal("existing login created an ownership intent")
	}
}

func TestCleanupResolvesOnlyExactJournaledControlledLogin(t *testing.T) {
	c := controlledUserCase(t, "controlled+{prefix}@example.test")
	login, err := c.IntentControlledUser("discovery")
	if err != nil {
		t.Fatal(err)
	}
	deleted := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site/users":
			_ = json.NewEncoder(w).Encode(map[string]any{"users": map[string]any{"user": []any{
				map[string]any{"id": "unrelated-user", "name": "base@example.test"},
				map[string]any{"id": "owned-user", "name": login},
			}}, "pagination": map[string]any{"totalAvailable": "2"}})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/3.29/sites/site/users/owned-user":
			deleted = append(deleted, "owned-user")
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/users/owned-user"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/users/unrelated-user"):
			t.Error("cleanup queried an unrelated base account")
		default:
			t.Errorf("unexpected cleanup request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	c.API = &Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}
	if err := c.Journal.Cleanup(t.Context(), c.API); err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0] != "owned-user" || !c.Journal.Entries[0].Done {
		t.Fatal("cleanup did not delete only the exact journaled controlled login")
	}
}

func controlledUserCase(t *testing.T, template string) *Case {
	t.Helper()
	root := t.TempDir()
	prefix := "tadx-live-controlled-user"
	journal, err := NewJournal(filepath.Join(root, "journal.json"), "0123456789abcdefabcd", "target", prefix)
	if err != nil {
		t.Fatal(err)
	}
	return &Case{RunDir: root, NamePrefix: prefix, TargetFingerprint: "target", Journal: journal, Profile: Profile{Bindings: map[string]Binding{"controlled_user": {Name: template, Params: map[string]string{"auth_setting": "ServerDefault"}}}}}
}
