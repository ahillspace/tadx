package live

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/ahillspace/tadx/tests/live/harness"
	"github.com/ahillspace/tadx/tests/live/suite"
)

const fixtureSpecVersion = "tadx-live-fixtures-v2"

type Manifest struct {
	Version           int               `json:"schema_version"`
	FixtureSpec       string            `json:"fixture_spec"`
	CatalogHash       string            `json:"catalog_hash"`
	TargetFingerprint string            `json:"target_fingerprint"`
	SiteID            string            `json:"site_id"`
	ActorUserID       string            `json:"actor_user_id"`
	SetupRun          string            `json:"setup_run"`
	RootProject       harness.Object    `json:"root_project"`
	Profile           harness.Profile   `json:"resolved_profile"`
	Selection         []string          `json:"selection"`
	ArtifactHashes    map[string]string `json:"artifact_hashes"`
	ReadyAt           time.Time         `json:"ready_at"`
}

type Result struct {
	ID       string         `json:"test_id"`
	RunID    string         `json:"run_id"`
	Status   harness.Status `json:"status"`
	Message  string         `json:"message,omitempty"`
	Cleanup  string         `json:"cleanup_status"`
	Started  time.Time      `json:"started_at"`
	Ended    time.Time      `json:"ended_at"`
	Evidence string         `json:"evidence,omitempty"`
	Notes    []string       `json:"notes,omitempty"`
}
type Report struct {
	Version            int      `json:"schema_version"`
	RunID              string   `json:"run_id"`
	CandidateSHA256    string   `json:"candidate_sha256"`
	CandidateRevision  string   `json:"candidate_revision,omitempty"`
	CandidateModified  string   `json:"candidate_modified,omitempty"`
	CandidateGoVersion string   `json:"candidate_go_version,omitempty"`
	TargetFingerprint  string   `json:"target_fingerprint"`
	CatalogHash        string   `json:"catalog_hash"`
	FixtureSpec        string   `json:"fixture_spec"`
	Results            []Result `json:"results"`
	NotSelected        []string `json:"not_selected"`
}

func Scenarios() (map[string]harness.Scenario, error) {
	all := map[string]harness.Scenario{}
	for _, set := range []map[string]harness.Scenario{suite.ContentCases(), suite.AdminCases(), suite.CatalogCases(), suite.PulseCases(), suite.SharedCases()} {
		for id, fn := range set {
			if all[id] != nil || fn == nil {
				return nil, fmt.Errorf("duplicate or empty scenario %s", id)
			}
			all[id] = fn
		}
	}
	catalog, err := Catalog()
	if err != nil {
		return nil, err
	}
	if len(catalog) != len(all) {
		return nil, fmt.Errorf("catalog/scenario count mismatch: %d/%d", len(catalog), len(all))
	}
	for _, s := range catalog {
		if all[s.ID] == nil {
			return nil, fmt.Errorf("catalog case has no executable scenario: %s", s.ID)
		}
	}
	return all, nil
}

func fixtureCapabilities(specs []Spec) []string {
	set := map[string]bool{"workspace.create": true, "workspace.status": true, "policy.status": true, "auth.check": true, "project.create": true, "project.delete": true}
	for _, s := range specs {
		for _, cap := range s.Capabilities {
			set[cap] = true
		}
		for _, helpers := range [][]string{suite.AssignedFixtureCapabilities(s.ID), suite.ContentFixtureCapabilities(s.ID), suite.SharedFixtureCapabilities(s.ID)} {
			for _, id := range helpers {
				set[id] = true
			}
		}
	}
	for key := range selectedBindingKeys(specs) {
		for _, kind := range []string{"workbook", "datasource", "flow"} {
			if strings.HasPrefix(key, kind+"-native") {
				set[kind+".publish"] = true
				set[kind+".delete"] = true
				set["job.inspect"] = true
			}
		}
	}
	return slices.Sorted(maps.Keys(set))
}

func profileFixtureCapabilities(specs []Spec, p harness.Profile) []string {
	caps := fixtureCapabilities(specs)
	if pulseNativeSelected(specs, p) {
		caps = append(caps, "datasource.publish", "datasource.delete", "datasource.schema", "job.inspect")
	}
	if p.Features["flows"] == "supported" {
		for _, s := range specs {
			if s.ID == "live.search.content-discovery" {
				caps = append(caps, "flow.publish", "flow.delete", "job.inspect")
			}
		}
	}
	slices.Sort(caps)
	return slices.Compact(caps)
}

func specsFor(plan Plan) ([]Spec, error) {
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	catalog, err := Catalog()
	if err != nil {
		return nil, err
	}
	out := []Spec{}
	for _, sel := range plan.Tests {
		for _, s := range catalog {
			if s.ID == sel.ID {
				out = append(out, s)
				break
			}
		}
	}
	return out, nil
}
func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	h := sha256.New()
	if _, err = io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func validateBindings(specs []Spec, p harness.Profile) error {
	validationProfile := p
	if pulseNativeSelected(specs, p) {
		input := p.Bindings[pulseNativeBinding]
		if err := validatePulseNativeInput(input); err != nil {
			return err
		}
		// This ephemeral binding lets existing case validators check account
		// prerequisites and requested fields; it is never persisted or executed.
		validationProfile.Bindings = maps.Clone(p.Bindings)
		validationProfile.Bindings["pulse_datasource"] = harness.Binding{ID: "setup-input-validation-only", Params: map[string]string{"measure_field": input.Params["measure_name"], "date_field": input.Params["date_name"], "dimension_field": input.Params["dimension_name"], "category": input.Params["category"]}}
		if pulseMigrationSelected(specs) {
			validationProfile.Bindings["pulse_destination_datasource"] = harness.Binding{ID: "setup-input-validation-only"}
		}
	}
	var errs []error
	for _, s := range specs {
		if err := suite.ValidateAssignedBindings(s.ID, validationProfile); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.ID, err))
		}
		if err := suite.ValidateSharedBindings(s.ID, validationProfile); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.ID, err))
		}
	}
	ids := []string{}
	for _, s := range specs {
		ids = append(ids, s.ID)
	}
	if err := suite.ValidateContentRequirements(p, ids); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func Setup(ctx context.Context, p harness.Profile, binary, state string, plan Plan) (Manifest, error) {
	var m Manifest
	if err := harness.ConfigurePython(p.Python); err != nil {
		return m, err
	}
	specs, err := specsFor(plan)
	if err != nil {
		return m, err
	}
	if len(specs) == 0 {
		return m, errors.New("setup requires a nonempty explicit selection")
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return m, err
	}
	if _, err = hashFile(binary); err != nil {
		return m, err
	}
	state, err = filepath.Abs(state)
	if err != nil {
		return m, err
	}
	if err = os.MkdirAll(state, 0o700); err != nil {
		return m, err
	}
	required := profileFixtureCapabilities(specs, p)
	if err = harness.Preflight(ctx, p, binary, state, required); err != nil {
		return m, err
	}
	if err = harness.VerifyIndependentCredentials(ctx, p); err != nil {
		return m, err
	}
	if p.Bindings == nil {
		p.Bindings = map[string]harness.Binding{}
	}
	if p.Artifacts == nil {
		p.Artifacts = map[string]string{}
	}
	needsNative := false
	for key := range selectedBindingKeys(specs) {
		if strings.HasPrefix(key, "workbook-native") || strings.HasPrefix(key, "datasource-native") {
			needsNative = true
		}
	}
	if needsNative && (p.Artifacts["workbook-native"] == "" || p.Artifacts["datasource-native"] == "") {
		artifacts, bindings, err := harness.GenerateFixtures(ctx, filepath.Join(state, "native"))
		if err != nil {
			return m, err
		}
		for k, v := range artifacts {
			if p.Artifacts[k] == "" {
				p.Artifacts[k] = v
			}
		}
		for k, v := range bindings {
			if _, exists := p.Bindings[k]; !exists {
				p.Bindings[k] = v
			}
		}
	}
	if err = validateBindings(specs, p); err != nil {
		return m, err
	}
	unlock, err := harness.AcquireTargetLock(harness.Fingerprint(p.ServerURL, p.SiteContentURL, ""))
	if err != nil {
		return m, err
	}
	defer unlock()
	if err = guardLocalWorkers(ctx, p, binary, state); err != nil {
		return m, err
	}
	// Actor identity is established independently before candidate transfers.
	actorProfile := p
	actorProfile.FixturePAT = harness.Credential{}
	actor, err := harness.NewClient(ctx, actorProfile)
	if err != nil {
		return m, err
	}
	actorID, siteID := actor.UserID, actor.SiteID
	if err = actor.Close(ctx); err != nil {
		return m, err
	}
	p.ExpectedSiteLUID = siteID
	api, err := harness.NewClient(ctx, p)
	if err != nil {
		return m, err
	}
	defer api.Close(context.WithoutCancel(ctx))
	target := harness.Fingerprint(p.ServerURL, p.SiteContentURL, api.SiteID)
	if err = VerifyTargetFeatures(ctx, api, p, specs); err != nil {
		return m, err
	}
	if err = verifyInputBindings(ctx, api, p, specs); err != nil {
		return m, err
	}
	manifestPath := filepath.Join(state, "manifest.json")
	if data, readErr := os.ReadFile(manifestPath); readErr == nil {
		var prior Manifest
		if json.Unmarshal(data, &prior) != nil {
			return m, errors.New("existing setup manifest is invalid; preserve it for recovery")
		}
		if prior.TargetFingerprint != target {
			return m, harness.Blocked("state directory belongs to another target; use a separate state directory to preserve exact-target recovery")
		}
		if prior.RootProject.ID == "" {
			return m, harness.Blocked("setup root acceptance is unresolved; cleanup setup run " + prior.SetupRun + " before setup")
		}
		if prior.FixtureSpec != fixtureSpecVersion {
			return m, harness.Blocked("cleanup the existing setup root before changing its fixture specification")
		}
		if prior.TargetFingerprint == target {
			root, readErr := api.Get(ctx, "project", prior.RootProject.ID)
			if readErr == nil && root.Name == prior.RootProject.Name {
				prior.Selection = selectionIDs(plan)
				prior.CatalogHash = CatalogHash()
				prior.Profile = p
				prior.ActorUserID = actorID
				dir := filepath.Join(state, "runs", prior.SetupRun, "setup")
				c, resumeErr := harness.ResumeCase(ctx, p, binary, dir, target, api, fixtureCapabilities(nil))
				if resumeErr != nil {
					return m, resumeErr
				}
				c.Root = root.ID
				c.ActorUserID = actorID
				err = completeSetup(ctx, c, &prior, state, specs)
				return prior, err
			}
			if !harness.IsNotFound(readErr) {
				return m, errors.New("existing fixture root identity or visibility changed; cleanup or restore its journal before setup")
			}
			// An authoritative absent root permits a new owned setup; the old
			// journal remains available for exact-target recovery and auditing.
		}
	}
	run := harness.NewRunID()
	dir := filepath.Join(state, "runs", run, "setup")
	hashes, err := artifactHashesFor(p, specs)
	if err != nil {
		return m, err
	}
	m = Manifest{Version: 1, FixtureSpec: fixtureSpecVersion, CatalogHash: CatalogHash(), TargetFingerprint: target, SiteID: api.SiteID, ActorUserID: actorID, SetupRun: run, Profile: p, Selection: selectionIDs(plan), ArtifactHashes: hashes}
	if err = harness.WriteJSON(manifestPath, m); err != nil {
		return m, err
	}
	c, err := harness.NewCase(ctx, p, binary, dir, "live.setup.root", run, "", target, fixtureCapabilities(nil))
	if err != nil {
		return m, err
	}
	c.ActorUserID = actorID
	c.BindAPI(api)
	root, err := c.Project(ctx, "root")
	if err != nil {
		return m, err
	}
	m.RootProject = root
	c.Root = root.ID
	err = completeSetup(ctx, c, &m, state, specs)
	return m, err
}

func completeSetup(ctx context.Context, c *harness.Case, m *Manifest, state string, specs []Spec) error {
	path := filepath.Join(state, "manifest.json")
	m.ReadyAt = time.Time{}
	if err := harness.WriteJSON(path, m); err != nil {
		return err
	}
	retainedWorkbookPack := retainedWorkbookPackSelected(specs)
	retainedPulsePack := pulseNativeSelected(specs, c.Profile)
	if retainedWorkbookPack {
		if err := c.AddRequiredCapabilities([]string{"workbook.publish", "workbook.delete", "job.inspect"}); err != nil {
			return err
		}
		if err := PrepareInspectFixtures(ctx, c); err != nil {
			return err
		}
	}
	if retainedPulsePack {
		if err := c.AddRequiredCapabilities([]string{"datasource.publish", "datasource.delete", "datasource.schema", "job.inspect"}); err != nil {
			return err
		}
		if err := PreparePulseFixtures(ctx, c, pulseMigrationSelected(specs)); err != nil {
			return err
		}
	}
	m.Profile = c.Profile
	hashes, err := artifactHashesFor(m.Profile, specs)
	if err != nil {
		return err
	}
	m.ArtifactHashes = hashes
	if err = harness.WriteJSON(path, m); err != nil {
		return err
	}
	if retainedWorkbookPack {
		if err = AwaitInspectFixture(ctx, c); err != nil {
			return err
		}
	}
	if retainedPulsePack {
		if err = AwaitPulseFixtures(ctx, c, pulseMigrationSelected(specs)); err != nil {
			return err
		}
		m.Profile = c.Profile
		if err = harness.WriteJSON(path, m); err != nil {
			return err
		}
	}
	m.ReadyAt = time.Now().UTC()
	return harness.WriteJSON(path, m)
}

func retainedWorkbookPackSelected(specs []Spec) bool {
	return slices.ContainsFunc(specs, func(s Spec) bool {
		return s.ID == "live.workbook.inspect" || s.ID == "live.contract.output-real-result"
	})
}

func verifyRetainedWorkbookPack(ctx context.Context, c *harness.Case, specs []Spec) error {
	if !retainedWorkbookPackSelected(specs) {
		return nil
	}
	return VerifyInspectFixtures(ctx, c)
}

func verifyRetainedPulsePack(ctx context.Context, c *harness.Case, specs []Spec) error {
	if !pulseNativeSelected(specs, c.Profile) {
		return nil
	}
	return VerifyPulseFixtures(ctx, c, pulseMigrationSelected(specs))
}

func selectedBindingKeys(specs []Spec) map[string]bool {
	keys := map[string]bool{}
	for _, s := range specs {
		for _, requirements := range []map[string][]string{suite.BindingRequirements(), suite.ContentBindingRequirements(), suite.SharedBindingRequirements()} {
			for _, key := range requirements[s.ID] {
				keys[key] = true
			}
		}
	}
	if pulseCasesSelected(specs) {
		keys[pulseNativeBinding] = true
	}
	if retainedWorkbookPackSelected(specs) {
		keys["workbook-native"] = true
		keys["workbook-inspect-primary"] = true
		keys["workbook-inspect-decoy"] = true
	}
	return keys
}
func artifactHashesFor(p harness.Profile, specs []Spec) (map[string]string, error) {
	out := map[string]string{}
	keys := selectedBindingKeys(specs)
	if pulseNativeSelected(specs, p) {
		delete(keys, "pulse_datasource")
		delete(keys, "pulse_destination_datasource")
	}
	if p.Features["flows"] == "supported" {
		for _, s := range specs {
			if s.ID == "live.search.content-discovery" {
				keys["flow-native"] = true
			}
		}
	}
	for k := range keys {
		if b, ok := p.Bindings[k]; ok && b.Path != "" {
			h, err := hashFile(b.Path)
			if err != nil {
				return nil, fmt.Errorf("fixture binding %s is unavailable", k)
			}
			out["binding:"+k] = h
			continue
		}
		if path := p.Artifacts[k]; path != "" {
			h, err := hashFile(path)
			if err != nil {
				return nil, fmt.Errorf("fixture %s is unavailable", k)
			}
			out[k] = h
		}
	}
	return out, nil
}
func verifyArtifactHashes(m Manifest) error {
	for key, want := range m.ArtifactHashes {
		path := m.Profile.Artifacts[key]
		if strings.HasPrefix(key, "binding:") {
			path = m.Profile.Bindings[strings.TrimPrefix(key, "binding:")].Path
		}
		got, err := hashFile(path)
		if err != nil || got != want {
			return harness.Blocked("fixture bytes changed or disappeared after setup; run setup again")
		}
	}
	return nil
}
func selectionIDs(p Plan) []string {
	out := []string{}
	for _, s := range p.Tests {
		out = append(out, s.ID)
	}
	return out
}
func uniqueIDs(ids []string) []string { slices.Sort(ids); return slices.Compact(ids) }

func verifyInputBindings(ctx context.Context, api *harness.Client, p harness.Profile, specs []Spec) error {
	keys := selectedBindingKeys(specs)
	for key := range keys {
		if pulseNativeSelected(specs, p) && (key == "pulse_datasource" || key == "pulse_destination_datasource") {
			continue
		}
		b := p.Bindings[key]
		if b.Params["oracle_query"] != "" {
			var vars map[string]any
			if raw := b.Params["oracle_variables"]; raw != "" {
				if json.Unmarshal([]byte(raw), &vars) != nil {
					return harness.Blocked("fixture oracle_variables are invalid")
				}
			}
			observed, err := api.Metadata(ctx, b.Params["oracle_query"], vars)
			if err != nil {
				return err
			}
			if expected, ok := b.Expected["oracle"].(map[string]any); ok {
				if err = observedSubset(observed, expected); err != nil {
					return harness.Blocked("fixture " + key + " does not match its authored independent oracle")
				}
			}
		}
		kind := ""
		if key == "user_a" || key == "user_b" || key == "alternate-owner" {
			kind = "user"
		}
		if (key == "pulse_datasource" || key == "pulse_destination_datasource") && !pulseNativeSelected(specs, p) {
			kind = "datasource"
		}
		if kind != "" && b.ID != "" {
			if _, err := api.Get(ctx, kind, b.ID); err != nil {
				return harness.Blocked("required fixture binding " + key + " is not readable on this target")
			}
		}
	}
	return nil
}
func observedSubset(actual, expected any) error {
	switch want := expected.(type) {
	case map[string]any:
		got, ok := actual.(map[string]any)
		if !ok {
			return errors.New("expected object")
		}
		for k, v := range want {
			value, present := got[k]
			if !present {
				return fmt.Errorf("missing expected field %s", k)
			}
			if err := observedSubset(value, v); err != nil {
				return err
			}
		}
	case []any:
		got, ok := actual.([]any)
		if !ok || len(got) != len(want) {
			return errors.New("expected exact array length")
		}
		for i, v := range want {
			if err := observedSubset(got[i], v); err != nil {
				return err
			}
		}
	default:
		a, _ := json.Marshal(actual)
		b, _ := json.Marshal(expected)
		if string(a) != string(b) {
			return errors.New("expected field value differs")
		}
	}
	return nil
}

func loadTargetManifest(state string, p harness.Profile) (Manifest, error) {
	var m Manifest
	data, err := os.ReadFile(filepath.Join(state, "manifest.json"))
	if err != nil {
		return m, harness.Blocked("run setup for this selection before running live tests")
	}
	if err = json.Unmarshal(data, &m); err != nil {
		return m, errors.New("invalid setup manifest")
	}
	if m.Version != 1 {
		return m, harness.Blocked("unknown setup manifest version")
	}
	if harness.Fingerprint(p.ServerURL, p.SiteContentURL, m.SiteID) != m.TargetFingerprint || m.Profile.ServerURL != p.ServerURL || m.Profile.SiteContentURL != p.SiteContentURL {
		return m, errors.New("setup manifest belongs to another target")
	}
	if p.ExpectedSiteLUID != "" && p.ExpectedSiteLUID != m.SiteID {
		return m, errors.New("profile site guard differs from setup")
	}
	return m, nil
}
func LoadManifest(state string, p harness.Profile) (Manifest, error) {
	m, err := loadTargetManifest(state, p)
	if err != nil {
		return m, err
	}
	if m.CatalogHash != CatalogHash() || m.FixtureSpec != fixtureSpecVersion {
		return m, harness.Blocked("setup manifest is stale; run setup again")
	}
	if m.ReadyAt.IsZero() || m.RootProject.ID == "" {
		return m, harness.Blocked("setup has not completed its selected fixture readiness; resume setup before running cases")
	}
	if err := verifyPulseNativeInputProfile(m.Profile, p); err != nil {
		return m, err
	}
	return m, verifyArtifactHashes(m)
}

func verifyPulseNativeInputProfile(saved, current harness.Profile) error {
	savedBinding, savedOK := saved.Bindings[pulseNativeBinding]
	currentBinding, currentOK := current.Bindings[pulseNativeBinding]
	if savedOK != currentOK {
		return harness.Blocked("pulse-native setup input differs from the retained manifest; rerun setup")
	}
	if !savedOK {
		return nil
	}
	savedJSON, err := json.Marshal(savedBinding)
	if err != nil {
		return err
	}
	currentJSON, err := json.Marshal(currentBinding)
	if err != nil {
		return err
	}
	if !bytes.Equal(savedJSON, currentJSON) {
		return harness.Blocked("pulse-native setup input differs from the retained manifest; rerun setup")
	}
	return nil
}

func VerifySetup(ctx context.Context, p harness.Profile, binary, state string, plan Plan) (Manifest, error) {
	var absoluteErr error
	binary, absoluteErr = filepath.Abs(binary)
	if absoluteErr != nil {
		return Manifest{}, absoluteErr
	}
	state, absoluteErr = filepath.Abs(state)
	if absoluteErr != nil {
		return Manifest{}, absoluteErr
	}
	m, err := LoadManifest(state, p)
	if err != nil {
		return m, err
	}
	specs, err := specsFor(plan)
	if err != nil {
		return m, err
	}
	if err = harness.Preflight(ctx, p, binary, state, profileFixtureCapabilities(specs, p)); err != nil {
		return m, err
	}
	for _, s := range specs {
		if !slices.Contains(m.Selection, s.ID) {
			return m, harness.Blocked("setup does not include selected case " + s.ID)
		}
	}
	resolved := m.Profile
	resolved.ActorPAT = p.ActorPAT
	resolved.FixturePAT = p.FixturePAT
	resolved.ConfigPath = p.ConfigPath
	resolved.ExpectedSiteLUID = m.SiteID
	if err = harness.ConfigurePython(resolved.Python); err != nil {
		return m, err
	}
	unlock, err := harness.AcquireTargetLock(harness.Fingerprint(p.ServerURL, p.SiteContentURL, ""))
	if err != nil {
		return m, err
	}
	defer unlock()
	if err = guardLocalWorkers(ctx, p, binary, state); err != nil {
		return m, err
	}
	api, err := harness.NewClient(ctx, resolved)
	if err != nil {
		return m, err
	}
	defer api.Close(context.WithoutCancel(ctx))
	root, err := api.Get(ctx, "project", m.RootProject.ID)
	if err != nil || root.Name != m.RootProject.Name {
		return m, harness.Blocked("setup fixture root is missing or changed")
	}
	if err = VerifyTargetFeatures(ctx, api, resolved, specs); err != nil {
		return m, err
	}
	if retainedWorkbookPackSelected(specs) || pulseNativeSelected(specs, resolved) {
		dir := filepath.Join(state, "runs", m.SetupRun, "setup")
		c, resumeErr := harness.ResumeCase(ctx, resolved, binary, dir, m.TargetFingerprint, api, fixtureCapabilities(nil))
		if resumeErr != nil {
			return m, resumeErr
		}
		c.Root = m.RootProject.ID
		if retainedWorkbookPackSelected(specs) {
			if err = verifyRetainedWorkbookPack(ctx, c, specs); err != nil {
				return m, err
			}
		}
		if err = verifyRetainedPulsePack(ctx, c, specs); err != nil {
			return m, err
		}
	}
	return m, verifyInputBindings(ctx, api, resolved, specs)
}

func Run(ctx context.Context, p harness.Profile, binary, state string, plan Plan) (report Report, runErr error) {
	report = Report{Version: 1, RunID: harness.NewRunID(), CatalogHash: CatalogHash(), FixtureSpec: fixtureSpecVersion, NotSelected: plan.NotSelected}
	specs, err := specsFor(plan)
	if err != nil {
		return report, err
	}
	if len(specs) == 0 {
		return report, errors.New("run requires a nonempty explicit selection")
	}
	defer func() {
		if runErr == nil {
			return
		}
		finished := map[string]bool{}
		for _, result := range report.Results {
			finished[result.ID] = true
		}
		now := time.Now().UTC()
		for _, spec := range specs {
			if !finished[spec.ID] {
				report.Results = append(report.Results, Result{ID: spec.ID, RunID: report.RunID, Status: harness.ErrorStatus(runErr), Message: "run could not start this case: " + runErr.Error(), Cleanup: "not-attempted", Started: now, Ended: now})
			}
		}
		if filepath.IsAbs(state) {
			if err := harness.WriteJSON(filepath.Join(state, "runs", report.RunID, "report.json"), report); err != nil {
				runErr = errors.Join(runErr, err)
			}
		}
	}()
	registry, err := Scenarios()
	if err != nil {
		return report, err
	}
	state, err = filepath.Abs(state)
	if err != nil {
		return report, err
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return report, err
	}
	report.CandidateSHA256, err = hashFile(binary)
	if err != nil {
		return report, err
	}
	if info, readErr := buildinfo.ReadFile(binary); readErr == nil {
		report.CandidateGoVersion = info.GoVersion
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				report.CandidateRevision = setting.Value
			case "vcs.modified":
				report.CandidateModified = setting.Value
			}
		}
	}
	if err = harness.Preflight(ctx, p, binary, state, profileFixtureCapabilities(specs, p)); err != nil {
		return report, err
	}
	m, err := LoadManifest(state, p)
	if err != nil {
		return report, err
	}
	report.TargetFingerprint = m.TargetFingerprint
	for _, s := range specs {
		if !slices.Contains(m.Selection, s.ID) {
			return report, harness.Blocked("setup does not include selected case " + s.ID)
		}
	}
	resolved := m.Profile
	resolved.ActorPAT = p.ActorPAT
	resolved.FixturePAT = p.FixturePAT
	resolved.ConfigPath = p.ConfigPath
	resolved.Budgets = p.Budgets
	resolved.ExpectedSiteLUID = m.SiteID
	if p.Python != "" {
		resolved.Python = p.Python
	}
	if err = harness.ConfigurePython(resolved.Python); err != nil {
		return report, err
	}
	if err = harness.VerifyIndependentCredentials(ctx, resolved); err != nil {
		return report, err
	}
	unlock, err := harness.AcquireTargetLock(harness.Fingerprint(p.ServerURL, p.SiteContentURL, ""))
	if err != nil {
		return report, err
	}
	defer unlock()
	if err = guardLocalWorkers(ctx, p, binary, state); err != nil {
		return report, err
	}
	api, err := harness.NewClient(ctx, resolved)
	if err != nil {
		return report, err
	}
	defer api.Close(context.WithoutCancel(ctx))
	if _, err = api.Get(ctx, "project", m.RootProject.ID); err != nil {
		return report, harness.Blocked("fixture root is unavailable; run setup again")
	}
	var failures []error
	for _, spec := range specs {
		stopAfterCleanup := false
		dir := filepath.Join(state, "runs", report.RunID, strings.TrimPrefix(spec.ID, "live."))
		result := Result{ID: spec.ID, RunID: report.RunID, Started: time.Now().UTC(), Cleanup: "not-attempted"}
		c, caseErr := harness.NewCase(ctx, resolved, binary, dir, spec.ID, report.RunID, m.RootProject.ID, m.TargetFingerprint, profileFixtureCapabilities([]Spec{spec}, resolved))
		if caseErr == nil {
			c.ActorUserID = m.ActorUserID
			c.BindAPI(api)
			caseErr = c.CreateWorkspace(ctx)
		}
		if caseErr == nil {
			caseErr = registry[spec.ID](ctx, c)
		}
		result.Status = harness.ErrorStatus(caseErr)
		if caseErr != nil {
			result.Message = sanitize(api.Redact(caseErr.Error()), p, state, m)
			failures = append(failures, fmt.Errorf("%s: %w", spec.ID, caseErr))
		}
		if c != nil {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
			cleanupErr := c.Cleanup(cleanupCtx)
			cancel()
			result.Cleanup = "passed"
			if cleanupErr != nil {
				stopAfterCleanup = true
				result.Cleanup = "failed"
				result.Status = harness.Failed
				result.Message += sanitize("; cleanup: "+api.Redact(cleanupErr.Error()), p, state, m)
				failures = append(failures, cleanupErr)
			}
			result.Notes = c.Notes
			result.Evidence = filepath.ToSlash(filepath.Join("runs", report.RunID, strings.TrimPrefix(spec.ID, "live."), "commands.json"))
			if err = harness.WriteJSON(filepath.Join(dir, "commands.json"), c.Commands); err != nil {
				failures = append(failures, err)
				result.Status = harness.Failed
				result.Message = "write bounded command evidence failed"
			}
		}
		result.Ended = time.Now().UTC()
		report.Results = append(report.Results, result)
		if err = harness.WriteJSON(filepath.Join(state, "runs", report.RunID, "report.json"), report); err != nil {
			return report, err
		}
		if ctx.Err() != nil || stopAfterCleanup {
			break
		}
	}
	return report, errors.Join(failures...)
}

var identityPattern = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)

func sanitize(s string, p harness.Profile, state string, m Manifest) string {
	for _, v := range []string{p.ServerURL, p.ConfigPath, state, m.SiteID, m.ActorUserID} {
		if v != "" {
			s = strings.ReplaceAll(s, v, "[private]")
		}
	}
	return identityPattern.ReplaceAllStringFunc(s, func(id string) string { h := sha256.Sum256([]byte(id)); return "id:" + hex.EncodeToString(h[:6]) })
}

func Cleanup(ctx context.Context, p harness.Profile, binary, state, run string) error {
	if !regexp.MustCompile(`^[a-f0-9]{20}$`).MatchString(run) {
		return errors.New("cleanup requires one exact generated run ID")
	}
	var err error
	binary, err = filepath.Abs(binary)
	if err != nil {
		return err
	}
	state, err = filepath.Abs(state)
	if err != nil {
		return err
	}
	m, err := loadTargetManifest(state, p)
	if err != nil {
		return err
	}
	resolved := m.Profile
	resolved.ActorPAT = p.ActorPAT
	resolved.FixturePAT = p.FixturePAT
	resolved.ConfigPath = p.ConfigPath
	unlock, err := harness.AcquireTargetLock(harness.Fingerprint(p.ServerURL, p.SiteContentURL, ""))
	if err != nil {
		return err
	}
	defer unlock()
	if err = guardLocalWorkers(ctx, p, binary, state); err != nil {
		return err
	}
	api, err := harness.NewClient(ctx, resolved)
	if err != nil {
		return err
	}
	defer api.Close(context.WithoutCancel(ctx))
	root := filepath.Join(state, "runs", run)
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	var failures []error
	catalog, err := Catalog()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		required := fixtureCapabilities(nil)
		if entry.Name() == "setup" {
			if err = guardRootCleanup(state, m.TargetFingerprint, run); err != nil {
				failures = append(failures, err)
				continue
			}
		} else {
			found := false
			for _, spec := range catalog {
				if strings.TrimPrefix(spec.ID, "live.") == entry.Name() {
					required = profileFixtureCapabilities([]Spec{spec}, resolved)
					found = true
					break
				}
			}
			if !found {
				return errors.New("unknown case journal directory; cleanup requires its original runner version")
			}
		}
		c, resumeErr := harness.ResumeCase(ctx, resolved, binary, dir, m.TargetFingerprint, api, required)
		if resumeErr != nil {
			failures = append(failures, resumeErr)
			continue
		}
		if err = harness.Preflight(ctx, p, binary, state, c.RequiredCapabilities); err != nil {
			failures = append(failures, err)
			continue
		}
		if err = c.Cleanup(ctx); err != nil {
			failures = append(failures, err)
		}
	}
	if len(failures) == 0 && run == m.SetupRun {
		return os.Remove(filepath.Join(state, "manifest.json"))
	}
	return errors.Join(failures...)
}

func guardRootCleanup(state, target, setupRun string) error {
	return filepath.WalkDir(filepath.Join(state, "runs"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "journal.json" {
			return nil
		}
		relative, err := filepath.Rel(filepath.Join(state, "runs"), path)
		if err != nil {
			return err
		}
		if strings.Split(filepath.ToSlash(relative), "/")[0] == setupRun {
			return nil
		}
		j, err := harness.LoadJournal(path, target)
		if err != nil {
			return err
		}
		if j.TransferPending {
			return harness.Inconclusive("another run has uncertain transfer acceptance; root cleanup preserves its resources")
		}
		for _, en := range j.Entries {
			if !en.Done {
				return harness.Blocked("cleanup unfinished case journals before deleting the shared fixture root")
			}
		}
		return nil
	})
}
