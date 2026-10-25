package live

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/tests/live/harness"
)

type inspectSetupServer struct {
	t                     *testing.T
	journalPath           string
	projects, workbooks   map[string]map[string]any
	payloads              map[string][]byte
	writes, metadataReads int
	delayedMetadata       bool
	cancelMetadata        context.CancelFunc
	primaryID             string
	manifestPath          string
	checkpointVerified    bool
}

func newInspectSetup(t *testing.T) (*harness.Case, *inspectSetupServer) {
	t.Helper()
	root := t.TempDir()
	journalPath := filepath.Join(root, "journal.json")
	journal, err := harness.NewJournal(journalPath, "setup-run", "target", "tadx-live-setup")
	if err != nil {
		t.Fatal(err)
	}
	native := []byte(`<workbook><datasources><datasource name="native"><column name="[Sales]"/></datasource></datasources><worksheets><worksheet name="Inspect"/></worksheets></workbook>`)
	filename := filepath.Join(root, "fixture.twb")
	if err := os.WriteFile(filename, native, 0600); err != nil {
		t.Fatal(err)
	}
	state := &inspectSetupServer{t: t, journalPath: journalPath, projects: map[string]map[string]any{}, workbooks: map[string]map[string]any{}, payloads: map[string][]byte{}}
	server := httptest.NewServer(http.HandlerFunc(state.serve))
	t.Cleanup(server.Close)
	c := &harness.Case{Root: "shared-root", RunDir: root, NamePrefix: "tadx-live-setup", Journal: journal, API: &harness.Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}, Profile: harness.Profile{Budgets: harness.Budgets{ObservationSeconds: 2}, Bindings: map[string]harness.Binding{"workbook-native": {Path: filename, Expected: map[string]any{"columns": []string{"[Sales]"}, "worksheets": []string{"Inspect"}, "extract_count": 0}}}}}
	return c, state
}

func (s *inspectSetupServer) confirmedBeforeRead(id string) {
	s.t.Helper()
	journal, err := harness.LoadJournal(s.journalPath, "target")
	if err != nil {
		s.t.Error(err)
		return
	}
	for _, entry := range journal.Entries {
		if entry.Object != nil && entry.Object.ID == id {
			return
		}
	}
	s.t.Errorf("setup read began before returned identity %s was durably confirmed", id)
}
func (s *inspectSetupServer) serve(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "POST" && r.URL.Path == "/api/3.29/sites/site/projects":
		s.writes++
		var request struct {
			Project struct {
				Name   string `json:"name"`
				Parent string `json:"parentProjectId"`
			} `json:"project"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			s.t.Error(err)
			w.WriteHeader(400)
			return
		}
		for _, project := range s.projects {
			if project["name"] == request.Project.Name && project["parentProjectId"] == request.Project.Parent {
				w.WriteHeader(409)
				return
			}
		}
		id := fmt.Sprintf("project-%d", len(s.projects)+1)
		s.projects[id] = map[string]any{"id": id, "name": request.Project.Name, "parentProjectId": request.Project.Parent}
		_ = json.NewEncoder(w).Encode(map[string]any{"project": map[string]string{"id": id}})
	case r.Method == "GET" && r.URL.Path == "/api/3.29/sites/site/projects":
		rows := []any{}
		for id, project := range s.projects {
			s.confirmedBeforeRead(id)
			rows = append(rows, project)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"projects": map[string]any{"project": rows}, "pagination": map[string]any{"totalAvailable": len(rows)}})
	case r.Method == "POST" && r.URL.Path == "/api/3.29/sites/site/workbooks":
		s.writes++
		parts, err := r.MultipartReader()
		if err != nil {
			s.t.Error(err)
			w.WriteHeader(400)
			return
		}
		requestPart, err := parts.NextPart()
		if err != nil {
			s.t.Error(err)
			return
		}
		document, err := io.ReadAll(requestPart)
		if err != nil {
			s.t.Error(err)
			return
		}
		var request struct {
			Workbook struct {
				Name    string `xml:"name,attr"`
				Project struct {
					ID string `xml:"id,attr"`
				} `xml:"project"`
			} `xml:"workbook"`
		}
		if err := xml.Unmarshal(document, &request); err != nil {
			s.t.Error(err)
			return
		}
		nativePart, err := parts.NextPart()
		if err != nil {
			s.t.Error(err)
			return
		}
		native, err := io.ReadAll(nativePart)
		if err != nil {
			s.t.Error(err)
			return
		}
		id := fmt.Sprintf("workbook-%d", len(s.workbooks)+1)
		s.workbooks[id] = map[string]any{"id": id, "name": request.Workbook.Name, "project": map[string]any{"id": request.Workbook.Project.ID}}
		s.payloads[id] = native
		if s.primaryID == "" {
			s.primaryID = id
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"workbook": map[string]string{"id": id}})
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/3.29/sites/site/workbooks/"):
		suffix := strings.TrimPrefix(r.URL.Path, "/api/3.29/sites/site/workbooks/")
		id, content := strings.CutSuffix(suffix, "/content")
		workbook, ok := s.workbooks[id]
		if !ok {
			w.WriteHeader(404)
			return
		}
		s.confirmedBeforeRead(id)
		if content {
			w.Header().Set("Content-Disposition", `attachment; filename="fixture.twb"`)
			_, _ = w.Write(s.payloads[id])
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"workbook": workbook})
	case r.Method == "POST" && r.URL.Path == "/api/metadata/graphql":
		s.metadataReads++
		if s.manifestPath != "" && s.metadataReads == 1 {
			data, err := os.ReadFile(s.manifestPath)
			if err != nil {
				s.t.Errorf("read setup checkpoint before Metadata request: %v", err)
			} else {
				var manifest Manifest
				if err := json.Unmarshal(data, &manifest); err != nil {
					s.t.Errorf("decode setup checkpoint before Metadata request: %v", err)
				} else {
					primary := manifest.Profile.Bindings["workbook-inspect-primary"]
					decoy := manifest.Profile.Bindings["workbook-inspect-decoy"]
					if primary.ID != s.primaryID || decoy.ID == "" || decoy.ID == primary.ID || primary.Params["project_id"] == decoy.Params["project_id"] || !manifest.ReadyAt.IsZero() {
						s.t.Errorf("setup checkpoint incomplete before Metadata readiness: primary=%q expected=%q decoy=%q projects=%q/%q ready=%t", primary.ID, s.primaryID, decoy.ID, primary.Params["project_id"], decoy.Params["project_id"], !manifest.ReadyAt.IsZero())
					} else {
						s.checkpointVerified = true
					}
				}
			}
		}
		var request struct {
			Variables map[string]any `json:"variables"`
			Query     string         `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			s.t.Error(err)
			return
		}
		if request.Variables["luid"] != s.primaryID || !strings.Contains(request.Query, "first: 2") || !strings.Contains(request.Query, "filter: {luid: $luid}") {
			s.t.Error("Metadata readiness did not use bounded exact primary LUID")
		}
		if s.cancelMetadata != nil {
			s.cancelMetadata()
		}
		nodes := []any{}
		if !s.delayedMetadata || s.metadataReads > 1 {
			nodes = append(nodes, map[string]any{"luid": s.primaryID, "name": s.workbooks[s.primaryID]["name"]})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"workbooksConnection": map[string]any{"totalCount": len(nodes), "nodes": nodes, "pageInfo": map[string]bool{"hasNextPage": false}}}})
	default:
		s.t.Errorf("unexpected setup request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(500)
	}
}

func TestInspectSetupRetainsOwnedPairAcrossRetryWithoutMetadataWait(t *testing.T) {
	c, state := newInspectSetup(t)
	if err := PrepareInspectFixtures(t.Context(), c); err != nil {
		t.Fatal(err)
	}
	primary, decoy := c.Profile.Bindings["workbook-inspect-primary"], c.Profile.Bindings["workbook-inspect-decoy"]
	if primary.ID == "" || decoy.ID == "" || primary.ID == decoy.ID || primary.Name != decoy.Name || primary.Params["project_id"] == decoy.Params["project_id"] {
		t.Fatal("setup did not produce distinct same-name workbooks in two owned projects")
	}
	if state.writes != 4 || state.metadataReads != 0 {
		t.Fatalf("setup performed unexpected creation or Metadata wait: writes=%d metadata=%d", state.writes, state.metadataReads)
	}
	loaded, err := harness.LoadJournal(state.journalPath, "target")
	if err != nil {
		t.Fatal(err)
	}
	c.Journal = loaded
	c.Fixtures = nil
	delete(c.Profile.Bindings, "workbook-inspect-primary")
	delete(c.Profile.Bindings, "workbook-inspect-decoy")
	if err := PrepareInspectFixtures(t.Context(), c); err != nil {
		t.Fatal(err)
	}
	if state.writes != 4 || state.metadataReads != 0 || c.Profile.Bindings["workbook-inspect-primary"].ID != primary.ID || c.Profile.Bindings["workbook-inspect-decoy"].ID != decoy.ID {
		t.Fatal("setup retry duplicated or adopted resources instead of reusing confirmed owned IDs")
	}
	// The complete pair is checkpointable before a single Metadata request.
	if _, err := json.Marshal(c.Profile); err != nil {
		t.Fatal(err)
	}
	state.delayedMetadata = true
	if err := AwaitInspectFixture(t.Context(), c); err != nil {
		t.Fatal(err)
	}
	if state.metadataReads != 2 || state.writes != 4 {
		t.Fatal("post-checkpoint Metadata wait did not remain bounded and read-only")
	}
}

func TestOutputRealResultSetupCheckpointsAndAwaitsRetainedWorkbookPack(t *testing.T) {
	plan, err := Select(Selectors{IDs: []string{"live.contract.output-real-result"}})
	if err != nil {
		t.Fatal(err)
	}
	specs, err := specsFor(plan)
	if err != nil {
		t.Fatal(err)
	}
	c, state := newInspectSetup(t)
	state.manifestPath = filepath.Join(c.RunDir, "manifest.json")
	state.delayedMetadata = true
	m := Manifest{Version: 1, FixtureSpec: fixtureSpecVersion, SetupRun: "setup-run", Profile: c.Profile, RootProject: harness.Object{ID: c.Root, Name: "shared-root"}}
	if err := completeSetup(t.Context(), c, &m, c.RunDir, specs); err != nil {
		t.Fatal(err)
	}
	if state.writes != 4 || state.metadataReads != 2 || !state.checkpointVerified {
		t.Fatalf("output-result setup did not create, checkpoint, then await the retained pack: writes=%d metadata=%d checkpoint=%t", state.writes, state.metadataReads, state.checkpointVerified)
	}
	if m.ReadyAt.IsZero() || m.Profile.Bindings["workbook-inspect-primary"].ID == "" || m.Profile.Bindings["workbook-inspect-decoy"].ID == "" {
		t.Fatal("output-result setup completed without ready retained workbook bindings")
	}
	for _, key := range []string{"workbook-native", "workbook-inspect-primary", "workbook-inspect-decoy"} {
		if m.ArtifactHashes["binding:"+key] == "" {
			t.Errorf("output-result setup did not pin retained fixture binding %s", key)
		}
	}
	if err := verifyRetainedWorkbookPack(t.Context(), c, specs); err != nil {
		t.Fatal(err)
	}
	if state.writes != 4 {
		t.Fatal("read-only output-result setup verification repaired the retained pack")
	}
}

func TestUnrelatedSetupDoesNotPrepareOrVerifyRetainedWorkbookPack(t *testing.T) {
	plan, err := Select(Selectors{IDs: []string{"live.admin-label-category.list"}})
	if err != nil {
		t.Fatal(err)
	}
	specs, err := specsFor(plan)
	if err != nil {
		t.Fatal(err)
	}
	c, state := newInspectSetup(t)
	m := Manifest{Version: 1, FixtureSpec: fixtureSpecVersion, SetupRun: "setup-run", Profile: c.Profile, RootProject: harness.Object{ID: c.Root, Name: "shared-root"}}
	if err := completeSetup(t.Context(), c, &m, c.RunDir, specs); err != nil {
		t.Fatal(err)
	}
	if err := verifyRetainedWorkbookPack(t.Context(), c, specs); err != nil {
		t.Fatal(err)
	}
	if state.writes != 0 || state.metadataReads != 0 || c.Profile.Bindings["workbook-inspect-primary"].ID != "" {
		t.Fatal("unrelated setup provisioned or verified the retained workbook pack")
	}
}

func TestInspectSetupNeverRepeatsUncertainCreation(t *testing.T) {
	c, state := newInspectSetup(t)
	if err := c.Intent("project", c.NamePrefix+"-inspect-primary-project", c.Root); err != nil {
		t.Fatal(err)
	}
	if err := PrepareInspectFixtures(t.Context(), c); harness.ErrorStatus(err) != harness.InconclusiveStatus {
		t.Fatalf("uncertain setup creation was not retained: %v", err)
	}
	if state.writes != 0 || len(state.projects) != 0 {
		t.Fatal("setup duplicated an uncertain creation")
	}
}

func TestInspectSetupBlocksChangedImmutablePayloadWithoutOverwrite(t *testing.T) {
	c, state := newInspectSetup(t)
	if err := PrepareInspectFixtures(t.Context(), c); err != nil {
		t.Fatal(err)
	}
	primary := c.Profile.Bindings["workbook-inspect-primary"]
	state.payloads[primary.ID] = []byte(strings.ReplaceAll(string(state.payloads[primary.ID]), "[Sales]", "[Changed]"))
	if err := PrepareInspectFixtures(t.Context(), c); harness.ErrorStatus(err) != harness.BlockedStatus {
		t.Fatalf("changed immutable payload was silently accepted: %v", err)
	}
	if state.writes != 4 {
		t.Fatal("changed retained payload was overwritten or duplicated")
	}
}

func TestInspectMetadataDeadlinePreservesCheckpointedOwnership(t *testing.T) {
	c, state := newInspectSetup(t)
	if err := PrepareInspectFixtures(t.Context(), c); err != nil {
		t.Fatal(err)
	}
	primary := c.Profile.Bindings["workbook-inspect-primary"]
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	state.cancelMetadata = cancel
	if err := AwaitInspectFixture(ctx, c); err == nil {
		t.Fatal("cancelled Metadata readiness passed")
	}
	if c.Profile.Bindings["workbook-inspect-primary"].ID != primary.ID || state.writes != 4 {
		t.Fatal("Metadata timeout discarded checkpoint bindings or repeated setup writes")
	}
	loaded, err := harness.LoadJournal(state.journalPath, "target")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Entries) != 4 {
		t.Fatal("Metadata timeout lost setup ownership records")
	}
	for _, entry := range loaded.Entries {
		if entry.Object == nil || entry.Object.ID == "" || entry.Done {
			t.Fatal("Metadata timeout lost a confirmed resource identity")
		}
	}
}

func TestInspectVerificationNeverCreatesOrRepairsMissingOrChangedPack(t *testing.T) {
	for _, mode := range []string{"correct", "missing-binding", "missing-workbook", "changed-payload", "unowned-binding"} {
		t.Run(mode, func(t *testing.T) {
			c, state := newInspectSetup(t)
			if err := PrepareInspectFixtures(t.Context(), c); err != nil {
				t.Fatal(err)
			}
			primary := c.Profile.Bindings["workbook-inspect-primary"]
			switch mode {
			case "missing-binding":
				delete(c.Profile.Bindings, "workbook-inspect-decoy")
			case "missing-workbook":
				delete(state.workbooks, primary.ID)
			case "changed-payload":
				state.payloads[primary.ID] = []byte(strings.ReplaceAll(string(state.payloads[primary.ID]), "[Sales]", "[Changed]"))
			case "unowned-binding":
				primary.ID = "foreign-workbook"
				c.Profile.Bindings["workbook-inspect-primary"] = primary
			}
			err := VerifyInspectFixtures(t.Context(), c)
			if mode == "correct" {
				if err != nil {
					t.Fatal(err)
				}
				if state.metadataReads != 1 {
					t.Fatal("verification skipped exact Metadata readiness")
				}
			} else if err == nil {
				t.Fatal("missing or changed retained pack passed verification")
			}
			if state.writes != 4 {
				t.Fatal("verification created or repaired remote resources")
			}
		})
	}
}
