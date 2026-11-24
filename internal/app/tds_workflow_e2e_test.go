package app

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	ds "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/capability"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/managedpolicy"
	"github.com/ahillspace/tadx/internal/tableau"
	wire "github.com/ahillspace/tadx/internal/tableau/datasource"
	"github.com/ahillspace/tadx/internal/value"
)

const (
	fixtureTDSOriginal  = "11111111-1111-1111-1111-111111111111"
	fixtureTDSCopy      = "22222222-2222-2222-2222-222222222222"
	fixtureTDSPublished = "33333333-3333-3333-3333-333333333333"
	fixtureTDSSite      = "44444444-4444-4444-4444-444444444444"
	fixtureTDSProject   = "55555555-5555-5555-5555-555555555555"
)

type tdsFixtureSession struct{ cacheTestSession }

func (tdsFixtureSession) SiteLUID() string { return fixtureTDSSite }

// This server implements the documented JSON TDS and XML datasource routes.
// Its occupied-name rule is a fixture assumption used only behind the save gate.
type tdsWorkflowServer struct {
	blockCheckout           chan struct{}
	t                       *testing.T
	mu                      sync.Mutex
	source, copy, published map[string]value.TDSElement
	tag, name               string
	reject                  string
	writes                  []string
}

func tdsEntryKey(kind, name string) string { return kind + "/" + name }
func newTDSWorkflowServer(t *testing.T) *tdsWorkflowServer {
	caption, role, fieldType := "Revenue", "DIMENSION", "INTEGER"
	v := value.TDSElement{Name: "Sales", Column: &value.TDSColumn{Caption: &caption, Role: &role, Type: &fieldType}}
	return &tdsWorkflowServer{t: t, source: map[string]value.TDSElement{"Column/Sales": v}}
}
func (f *tdsWorkflowServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("X-Request-Id", "request-fixture")
	const dsPath = "/api/3.30/sites/" + fixtureTDSSite + "/datasources/"
	const wcPath = "/api/3.30/tds/sites/" + fixtureTDSSite + "/workcopy/" + fixtureTDSCopy
	const srcPath = "/api/3.30/tds/sites/" + fixtureTDSSite + "/datasource/"
	switch {
	case r.Method == "GET" && r.URL.Path == "/api/2.4/serverinfo":
		io.WriteString(w, `<tsResponse><serverInfo><restApiVersion>3.30</restApiVersion></serverInfo></tsResponse>`)
	case r.Method == "POST" && r.URL.Path == "/api/3.30/auth/signin":
		fmt.Fprintf(w, `{"credentials":{"token":"secret-token","site":{"id":%q},"user":{"id":"user-luid"}}}`, fixtureTDSSite)
	case r.Method == "POST" && r.URL.Path == "/fixture/overwrite":
		f.writes = append(f.writes, "fixture-overwrite")
		f.source = maps.Clone(f.copy)
		w.WriteHeader(204)
	case r.Header.Get("X-Tableau-Auth") != "secret-token":
		f.t.Errorf("unauthenticated %s %s", r.Method, r.URL)
		w.WriteHeader(401)
	case r.Method == "GET" && r.URL.Path == "/api/3.30/sites/"+fixtureTDSSite+"/projects":
		fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="1"/><projects><project id="%s" name="Test" topLevelProject="true"/></projects></tsResponse>`, r.URL.Query().Get("pageSize"), fixtureTDSProject)
	case r.Method == "GET" && r.URL.Path == strings.TrimSuffix(dsPath, "/"):
		n, entry := 0, ""
		if f.published != nil {
			n = 1
			entry = fmt.Sprintf(`<datasource id="%s" name="%s"><project id="%s" name="Test"/></datasource>`, fixtureTDSPublished, f.name, fixtureTDSProject)
		}
		fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="%d"/><datasources>%s</datasources></tsResponse>`, r.URL.Query().Get("pageSize"), n, entry)
	case r.Method == "GET" && (r.URL.Path == dsPath+fixtureTDSOriginal || r.URL.Path == dsPath+fixtureTDSPublished):
		id := strings.TrimPrefix(r.URL.Path, dsPath)
		name := "Source"
		if id == fixtureTDSPublished {
			name = f.name
		}
		fmt.Fprintf(w, `<tsResponse><datasource id="%s" name="%s" description="sales"><project id="%s" name="Test"/><owner id="user-luid"/><tags>`, id, name, fixtureTDSProject)
		if f.tag != "" {
			fmt.Fprintf(w, `<tag label="%s"/>`, f.tag)
		}
		io.WriteString(w, `</tags></datasource></tsResponse>`)
	case r.Method == "PUT" && r.URL.Path == dsPath+fixtureTDSOriginal+"/tags":
		var req struct {
			Tags []struct {
				Label string `xml:"label,attr"`
			} `xml:"tags>tag"`
		}
		if err := xml.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Tags) != 1 {
			f.t.Error("invalid tag request")
			w.WriteHeader(400)
			return
		}
		f.tag = req.Tags[0].Label
		f.writes = append(f.writes, "tag.add")
		fmt.Fprintf(w, `<tsResponse><tags><tag label="%s"/></tags></tsResponse>`, f.tag)
	case r.Method == "DELETE" && f.tag != "" && r.URL.Path == dsPath+fixtureTDSOriginal+"/tags/"+f.tag:
		f.tag = ""
		f.writes = append(f.writes, "tag.remove")
		w.WriteHeader(204)
	case r.Method == "POST" && r.URL.Path == srcPath+fixtureTDSOriginal+"/workcopy":
		f.copy = maps.Clone(f.source)
		f.writes = append(f.writes, "checkout")
		if f.blockCheckout != nil {
			close(f.blockCheckout)
			f.blockCheckout = nil
			f.mu.Unlock()
			<-r.Context().Done()
			f.mu.Lock()
			return
		}

		w.WriteHeader(201)
		fmt.Fprintf(w, `{"tdsWorkCopy":{"workCopyLuid":%q}}`, fixtureTDSCopy)
	case r.Method == "GET" && (r.URL.Path == wcPath || r.URL.Path == srcPath+fixtureTDSOriginal || r.URL.Path == srcPath+fixtureTDSPublished):
		data := f.copy
		if strings.HasPrefix(r.URL.Path, srcPath) {
			data = f.source
			if r.URL.Path == srcPath+fixtureTDSPublished {
				data = f.published
			}
		}
		kind, name := r.URL.Query().Get("elementType"), r.URL.Query().Get("name")
		items := []value.TDSElement{}
		if name != "" {
			el, ok := data[tdsEntryKey(kind, name)]
			if !ok {
				w.WriteHeader(404)
				io.WriteString(w, `{"error":{"code":"404065","summary":"No named element"}}`)
				return
			}
			items = append(items, el)
		} else {
			for key, el := range data {
				if strings.HasPrefix(key, kind+"/") {
					items = append(items, el)
				}
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"tdsElementList": items})
	case r.Method == "POST" && r.URL.Path == wcPath+"/publish":
		var req struct {
			Datasource struct {
				Name string `json:"name"`
			} `json:"datasource"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Datasource.Name == "" {
			f.t.Error("invalid publish request")
			w.WriteHeader(400)
			return
		}
		if f.published != nil {
			w.WriteHeader(409)
			return
		}
		f.writes = append(f.writes, "publish")
		f.name = req.Datasource.Name
		f.published = maps.Clone(f.copy)
		w.WriteHeader(201)
		fmt.Fprintf(w, `{"tdsWorkCopy":{"datasourceLuid":%q}}`, fixtureTDSPublished)
	case r.URL.Path == wcPath && (r.Method == "POST" || r.Method == "PUT" || r.Method == "DELETE"):
		var req struct {
			Elements []value.TDSElement `json:"tdsElementList"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Elements) != 1 {
			f.t.Error("invalid edit request")
			w.WriteHeader(400)
			return
		}
		el := req.Elements[0]
		key := tdsEntryKey(el.ElementType, el.Name)
		f.writes = append(f.writes, r.Method+" "+key)
		if key == f.reject {
			w.WriteHeader(403)
			fmt.Fprintf(w, `{"tdsElementResult":[{"elementType":%q,"name":%q,"status":{"code":403}}]}`, el.ElementType, el.Name)
			return
		}
		if r.Method == "DELETE" {
			delete(f.copy, key)
		} else {
			if el.Column != nil {
				col := *f.copy[key].Column
				switch {
				case el.Column.Caption != nil:
					col.Caption = el.Column.Caption
				case el.Column.Role != nil:
					col.Role = el.Column.Role
				case el.Column.Type != nil:
					col.Type = el.Column.Type
				default:
					f.t.Error("missing column property")
					w.WriteHeader(400)
					return
				}
				el.Column = &col
			}
			f.copy[key] = el
		}
		if r.Method == "POST" {
			w.WriteHeader(201)
		}
		io.WriteString(w, "{}")
	case strings.Contains(r.URL.Path, "metadata") || strings.Contains(r.URL.Path, "vizql"):
		w.WriteHeader(403)
		io.WriteString(w, `{"error":{"code":"403004","summary":"Optional metadata unavailable"}}`)
	default:
		f.t.Errorf("unexpected fixture request %s %s", r.Method, r.URL)
		w.WriteHeader(404)
	}
}
func (f *tdsWorkflowServer) writeCount() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.writes) }

type fixtureTDSSave struct {
	*wire.Client
	loopback string
	http     *http.Client
}

// Only the synthetic provider can attest to an entire source revision or overwrite it.
func (f fixtureTDSSave) ReadTDSRevision(context.Context, string) (string, error) {
	return "fixture-revision", nil
}

func (f fixtureTDSSave) TDSSaveDestination(_ context.Context, mode, copy, source, name string) (ds.TDSDestination, error) {
	if mode == "overwrite" && source == fixtureTDSOriginal {
		return ds.TDSDestination{Name: "Source", ProjectLUID: fixtureTDSProject, SourceRevision: "fixture-revision"}, nil
	}
	if mode != "as-new" {
		return ds.TDSDestination{}, errors.New("fixture covers save as new")
	}
	return ds.TDSDestination{Name: name, ProjectLUID: fixtureTDSProject}, nil
}
func (f fixtureTDSSave) SubmitTDSSave(ctx context.Context, mode, copy string, d ds.TDSDestination, desc *string) (value.TDSReceipt, error) {
	if mode == "overwrite" {
		req, err := http.NewRequestWithContext(ctx, "POST", f.loopback+"/fixture/overwrite", nil)
		if err != nil {
			return value.TDSReceipt{}, err
		}
		r, err := f.http.Do(req)
		if err != nil {
			return value.TDSReceipt{Status: "unknown"}, err
		}
		defer r.Body.Close()
		if r.StatusCode != 204 {
			return value.TDSReceipt{Status: "unknown"}, errors.New("fixture overwrite failed")
		}
		return value.TDSReceipt{Status: "acknowledged", DatasourceLUID: fixtureTDSOriginal}, nil
	}
	return f.Client.PublishWorkCopy(ctx, copy, d.Name, desc)
}
func (f fixtureTDSSave) ResolveDatasource(ctx context.Context, s identity.Selector) (ds.Record, error) {
	row, err := f.Client.Get(ctx, string(s.LUID))
	return ds.Record{LUID: row.LUID, Name: row.Name, ProjectLUID: row.ProjectLUID, Description: row.Description}, err
}
func (f fixtureTDSSave) FindDatasources(ctx context.Context, name, project string) ([]ds.Record, error) {
	page, err := f.Client.List(ctx, wire.ListRequest{PageNumber: 1, PageSize: 100, Name: name, ProjectLUID: project})
	if err != nil {
		return nil, err
	}
	var matches []ds.Record
	for _, d := range page.Items {
		if d.Name == name && d.ProjectLUID == project {
			matches = append(matches, ds.Record{LUID: d.LUID})
		}
	}
	return matches, nil
}

func qualifiedTDSDefinitions() []capability.Definition {
	definitions := capability.All()
	for i, d := range definitions {
		if !strings.HasPrefix(d.ID, "datasource.") {
			continue
		}
		verb := strings.TrimPrefix(d.ID, "datasource.")
		switch verb {
		case "checkout", "elements", "work-copy", "set-field-description", "remove-field-description", "set-table-description", "remove-table-description", "apply-changes", "save.as-new", "save.overwrite", "add-tag", "remove-tag":
			d.CommandPath = append([]string{"content", "datasource"}, strings.Split(verb, ".")...)
			d.Implementation = capability.ImplementationImplemented
		case "set-column-caption", "set-column-role", "set-column-type", "delete-column":
			d.Implementation = capability.ImplementationPlanned
		default:
			continue
		}
		d.Verification = capability.VerificationReady
		d.Blocker = ""
		d.EvidenceLevel = capability.EvidenceContractVerified
		definitions[i] = d
	}
	return definitions
}

type tdsWorkflowCLI struct {
	t       *testing.T
	options Options
	server  *httptest.Server
	ws      string
	fixture *tdsWorkflowServer
}

func newTDSWorkflowCLI(t *testing.T) *tdsWorkflowCLI {
	f := newTDSWorkflowServer(t)
	server := httptest.NewTLSServer(f)
	t.Cleanup(server.Close)
	root := t.TempDir()
	for _, key := range []string{"HOME", "APPDATA", "XDG_CONFIG_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(key, root)
	}
	options := overviewOptions(t, root)
	allowed := map[string]bool{}
	for _, d := range capability.All() {
		allowed[d.ID] = true
	}
	options.managedPolicy = fixtureManagedPolicy{state: managedpolicy.StateActive, remote: true, allowed: allowed}

	t.Setenv("TADX_FIXTURE_NAME", "test-name")
	t.Setenv("TADX_FIXTURE_SECRET", "test-secret")
	cfg := config.Config{Version: config.CurrentVersion, DefaultEnvironment: "dev", Environments: map[string]config.Environment{"dev": {URL: server.URL, SiteContentURL: "test-site", Auth: config.Auth{Type: config.AuthTypePAT, PATNameEnv: "TADX_FIXTURE_NAME", PATSecretEnv: "TADX_FIXTURE_SECRET"}}}}
	if err := config.Save(options.ConfigPath, cfg); err != nil {
		t.Fatal(err)
	}
	h := &tdsWorkflowCLI{t: t, options: options, server: server, ws: filepath.Join(root, "work"), fixture: f}
	h.command(true, "workspace", "create", "work", "--path", h.ws)
	return h
}

// A fresh process per invocation proves the workflow across output and journal
// boundaries. Qualification and destination assumptions exist only in this test binary.
type tdsFixtureInvocation struct {
	Args                 []string
	Config, Home, Server string
	Certificate          []byte
	Allowed              map[string]bool
	Remote               bool
}

func TestTDSQualifiedCLIProcess(t *testing.T) {
	if os.Getenv("TADX_TEST_TDS_CLI") != "1" {
		t.Skip("fixture subprocess")
	}
	var in tdsFixtureInvocation
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(in.Server, "https://127.0.0.1:") {
		t.Fatal("a loopback fixture is required")
	}
	cert, err := x509.ParseCertificate(in.Certificate)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}}}
	options := Options{ConfigPath: in.Config, HTTPClient: client, Stderr: os.Stderr, UserHomeDir: func() (string, error) { return in.Home, nil }, managedPolicy: fixtureManagedPolicy{state: managedpolicy.StateActive, remote: in.Remote, allowed: in.Allowed}}
	options.tdsSave = fixtureTDSSave{loopback: in.Server, http: client, Client: wire.NewClient(tableau.NewTransport(client, "3.30", nil), tdsFixtureSession{}, in.Server)}
	os.Exit(run(context.Background(), in.Args, os.Stdout, options, qualifiedTDSDefinitions()))
}

func (h *tdsWorkflowCLI) process(args ...string) *exec.Cmd {
	h.t.Helper()
	p := h.options.managedPolicy.(fixtureManagedPolicy)
	in := tdsFixtureInvocation{Args: args, Config: h.options.ConfigPath, Home: filepath.Dir(h.ws), Server: h.server.URL, Certificate: h.server.Certificate().Raw, Allowed: p.allowed, Remote: p.remote}
	data, err := json.Marshal(in)
	if err != nil {
		h.t.Fatal(err)
	}
	cmd := exec.CommandContext(h.t.Context(), os.Args[0], "-test.run=^TestTDSQualifiedCLIProcess$")
	cmd.Env = append(os.Environ(), "TADX_TEST_TDS_CLI=1", "GORACE=atexit_sleep_ms=0")
	cmd.Stdin = bytes.NewReader(data)
	return cmd
}

func (h *tdsWorkflowCLI) command(success bool, args ...string) string {
	h.t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := h.process(append(args, "--json", "--full")...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if (err == nil) != success || !json.Valid(stdout.Bytes()) {
		h.t.Fatalf("%s: %v stdout=%s stderr=%s", strings.Join(args, " "), err, &stdout, &stderr)
	}
	return stdout.String()
}
func (h *tdsWorkflowCLI) ds(success bool, args ...string) string {
	h.t.Helper()
	return h.command(success, append([]string{"content", "datasource"}, args...)...)
}
func requireTDSJSON(t *testing.T, text string, fields ...string) {
	t.Helper()
	for _, s := range fields {
		if !strings.Contains(text, s) {
			t.Fatalf("missing %q in %s", s, text)
		}
	}
}

func TestTDSWorkflowCLI(t *testing.T) {
	h := newTDSWorkflowCLI(t)
	checkout := []string{"checkout", "--id", fixtureTDSOriginal}
	h.ds(false, checkout...)
	h.ds(true, append(checkout, "--preview")...)
	if h.fixture.writeCount() != 0 {
		t.Fatal("denial or preview created a work copy")
	}
	h.options = withSiteMutationConsent(t, h.options, true)
	created := h.ds(true, append(checkout, "--workspace", "work", "--audit-file", "audit.md", "--changes-file", "starter.yaml")...)
	requireTDSJSON(t, created, fixtureTDSCopy, fixtureTDSOriginal)
	if h.fixture.writeCount() != 1 {
		t.Fatal("checkout allocation count")
	}
	starter, err := os.ReadFile(filepath.Join(h.ws, "starter.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(starter, []byte("changes: []")) || !bytes.Contains(starter, []byte("#")) || !bytes.Contains(starter, []byte("Sales")) {
		t.Fatalf("unexpected starter: %s", starter)
	}
	audit, err := os.ReadFile(filepath.Join(h.ws, "audit.md"))
	if err != nil || !bytes.Contains(audit, []byte("Sales")) {
		t.Fatalf("audit missing observed field: %s, %v", audit, err)
	}
	direct := []string{"set-field-description", "--work-copy", fixtureTDSCopy, "--field", "Sales", "--description", "Gross sales"}
	preview := h.ds(true, append(direct, "--preview")...)
	var p struct {
		PreviewToken string `json:"preview_token"`
	}
	if err := json.Unmarshal([]byte(preview), &p); err != nil || p.PreviewToken == "" {
		t.Fatalf("missing comparison token: %s", preview)
	}
	prior := h.fixture.writeCount()
	h.fixture.mu.Lock()
	external := "parallel description"
	h.fixture.copy["FieldDescription/Sales"] = value.TDSElement{Name: "Sales", Description: &external}
	h.fixture.mu.Unlock()
	h.ds(false, append(direct, "--expect-preview", p.PreviewToken)...)
	current := h.ds(true, append(direct, "--preview")...)
	var fresh struct {
		PreviewToken string `json:"preview_token"`
	}
	if err := json.Unmarshal([]byte(current), &fresh); err != nil || fresh.PreviewToken == "" || fresh.PreviewToken == p.PreviewToken {
		t.Fatal("stale token did not change")
	}
	p = fresh
	if h.fixture.writeCount() != prior {
		t.Fatal("stale preview submitted a request")
	}
	requireTDSJSON(t, h.ds(true, append(direct, "--expect-preview", p.PreviewToken)...), "verified", "Gross sales")
	requireTDSJSON(t, h.ds(true, direct...), "unchanged")
	requireTDSJSON(t, h.ds(true, "work-copy", "--work-copy", fixtureTDSCopy, "--field", "Sales"), "Gross sales")
	edits := strings.Replace(string(starter), "changes: []", `changes:
  - op: set-column-caption
    field: Sales
    caption: Bookings
  - op: set-table-description
    table: Orders
    description: Line items
  - op: set-column-role
    field: Sales
    role: MEASURE
  - op: set-column-type
    field: Sales
    type: REAL`, 1)
	if err := os.WriteFile(filepath.Join(h.ws, "edits.yaml"), []byte(edits), 0600); err != nil {
		t.Fatal(err)
	}
	file := []string{"--workspace", "work", "--from-changes", "edits.yaml"}
	runFile := append([]string{"apply-changes"}, file...)
	policy := fixtureManagedPolicy{state: managedpolicy.StateActive, remote: true, allowed: map[string]bool{"datasource.apply-changes": true, "datasource.set-table-description": true}}
	originalPolicy := h.options.managedPolicy
	h.options.managedPolicy = policy
	prior = h.fixture.writeCount()
	requireTDSJSON(t, h.ds(false, append(runFile, "--preview")...), "datasource.set-column-caption")
	h.ds(false, runFile...)
	if h.fixture.writeCount() != prior {
		t.Fatal("denied file member submitted a request")
	}
	h.options.managedPolicy = originalPolicy
	h.fixture.mu.Lock()
	h.fixture.reject = "TableDescription/Orders"
	h.fixture.mu.Unlock()
	requireTDSJSON(t, h.ds(false, runFile...), "verified", "rejected", "unattempted", "Bookings")
	h.fixture.mu.Lock()
	h.fixture.reject = ""
	h.fixture.mu.Unlock()
	requireTDSJSON(t, h.ds(true, runFile...), "unchanged", "verified")
	requireTDSJSON(t, h.ds(true, append([]string{"work-copy"}, file...)...), "Line items", "Bookings")
	save := append([]string{"save", "as-new", "--name", "Reviewed"}, file...)
	prior = h.fixture.writeCount()
	requireTDSJSON(t, h.ds(true, append(save, "--preview")...), "Reviewed")
	if h.fixture.writeCount() != prior {
		t.Fatal("save preview published")
	}
	saved := h.ds(true, save...)
	requireTDSJSON(t, saved, fixtureTDSPublished, "matched")
	var result struct {
		AttemptID string `json:"attempt_id"`
	}
	if err := json.Unmarshal([]byte(saved), &result); err != nil || result.AttemptID == "" {
		t.Fatalf("publish lost attempt identity: %s", saved)
	}
	requireTDSJSON(t, h.ds(true, "work-copy", "--attempt", result.AttemptID, "--verify"), fixtureTDSPublished, "matched")
	prior = h.fixture.writeCount()
	h.ds(false, save...)
	if h.fixture.writeCount() != prior {
		t.Fatal("occupied destination published")
	}
	overwrite := append([]string{"save", "overwrite"}, file...)
	requireTDSJSON(t, h.ds(true, append(overwrite, "--preview")...), fixtureTDSOriginal)
	requireTDSJSON(t, h.ds(true, overwrite...), fixtureTDSOriginal, "matched")
	requireTDSJSON(t, h.ds(true, "add-tag", "--id", fixtureTDSOriginal, "--tag", "Quarterly"), "verified", "Quarterly")
	requireTDSJSON(t, h.ds(true, "remove-tag", "--id", fixtureTDSOriginal, "--tag", "Quarterly"), "verified", "Quarterly")
	requireTDSJSON(t, h.ds(true, "elements", "--id", fixtureTDSPublished, "--field", "Sales"), "Gross sales", "Bookings", "MEASURE", "REAL")
	requireTDSJSON(t, h.ds(true, "remove-field-description", "--work-copy", fixtureTDSCopy, "--field", "Sales"), "verified", "absent")
	requireTDSJSON(t, h.ds(true, "remove-table-description", "--work-copy", fixtureTDSCopy, "--table", "Orders"), "verified", "absent")
	if err := os.WriteFile(filepath.Join(h.ws, "edits.yaml"), []byte(strings.Replace(string(starter), "changes: []", "changes:\n  - op: delete-column\n    field: Sales", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := h.process(append([]string{"content", "datasource"}, runFile...)...)
	compact, err := cmd.Output()
	if err != nil || !bytes.Contains(compact, []byte("verified")) || !bytes.Contains(compact, []byte("last --full")) {
		t.Fatalf("compact delete: %s (%v)", compact, err)
	}
	beforeLast := h.fixture.writeCount()
	requireTDSJSON(t, h.command(true, "last"), "verified", "delete-column")
	if h.fixture.writeCount() != beforeLast {
		t.Error("last repeated a remote operation")
	}

}

func TestTDSWorkflowCLIUnknownCheckoutRecovery(t *testing.T) {
	h := newTDSWorkflowCLI(t)
	h.options = withSiteMutationConsent(t, h.options, true)
	arrived := make(chan struct{})
	h.fixture.blockCheckout = arrived
	cmd := h.process("content", "datasource", "checkout", "--id", fixtureTDSOriginal, "--json")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	select {
	case <-arrived:
	case <-time.After(15 * time.Second):
		cmd.Process.Kill()
		cmd.Wait()
		t.Fatalf("checkout did not reach fixture: %s", &stderr)
	}
	// Two independent fixture PATs sign in as the same Tableau caller.
	// Separate credentials prevent the auth lock from masking journal admission.
	other := *h
	cfg, err := config.Load(h.options.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	env := cfg.Environments["dev"]
	t.Setenv("TADX_FIXTURE_OTHER_NAME", "second-name")
	env.Auth.PATNameEnv = "TADX_FIXTURE_OTHER_NAME"
	cfg.Environments["dev"] = env
	other.options.ConfigPath = filepath.Join(t.TempDir(), "other.yaml")
	if err := config.Save(other.options.ConfigPath, cfg); err != nil {
		t.Fatal(err)
	}
	beforeKill := other.ds(false, "checkout", "--id", fixtureTDSOriginal)
	requireTDSJSON(t, beforeKill, "admission")
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("killed checkout exited successfully")
	}
	requireTDSJSON(t, h.ds(true, "work-copy", "--attempts", "--id", fixtureTDSOriginal), fixtureTDSOriginal)
	h.ds(false, "checkout", "--id", fixtureTDSOriginal)
	if h.fixture.writeCount() != 1 {
		t.Fatal("unresolved checkout was submitted again")
	}
}
