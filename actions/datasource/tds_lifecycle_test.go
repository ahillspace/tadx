package datasource_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	ds "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/operationrun"
	"github.com/ahillspace/tadx/internal/value"
)

const (
	sourceUUID = "11111111-1111-1111-1111-111111111111"
	copyUUID   = "22222222-2222-2222-2222-222222222222"
	newUUID    = "33333333-3333-3333-3333-333333333333"
)

var tdsScope = value.TDSScope{Server: "https://tableau.example", SiteLUID: "site-id", CallerLUID: "caller-id"}

// This fake supplies the placement, create-only and revision guarantees that
// must be established on a disposable Tableau site before binding a real save.
type lifecycleFixture struct {
	revision, copyValue, publishedValue, destName   string
	create, submit                                  func() (value.TDSReceipt, error)
	copyCreateCalls, publishCalls, destinationCalls int
	collision                                       bool
}

func (f *lifecycleFixture) ResolveDatasource(_ context.Context, s identity.Selector) (ds.Record, error) {
	if s.LUID == sourceUUID || s.LUID == newUUID {
		return ds.Record{LUID: string(s.LUID), Name: f.destName, ProjectLUID: "target-project", Description: "private published metadata"}, nil
	}
	return ds.Record{}, errors.New("unknown datasource")
}
func (f *lifecycleFixture) CreateWorkCopy(context.Context, string) (value.TDSReceipt, error) {
	f.copyCreateCalls++
	if f.create != nil {
		return f.create()
	}
	return value.TDSReceipt{Status: "acknowledged", WorkCopyLUID: copyUUID, TableauRequestID: "create-1"}, nil
}
func (f *lifecycleFixture) ReadTDSRevision(context.Context, string) (string, error) {
	return f.revision, nil
}
func (f *lifecycleFixture) FindDatasources(context.Context, string, string) ([]ds.Record, error) {
	if f.collision {
		return []ds.Record{{LUID: "occupied"}}, nil
	}
	return nil, nil
}
func (f *lifecycleFixture) TDSSaveDestination(_ context.Context, mode, copy, source, name string) (ds.TDSDestination, error) {
	f.destinationCalls++
	if mode == "overwrite" {
		name = f.destName
	}
	return ds.TDSDestination{Name: name, ProjectLUID: "target-project", SourceRevision: f.revision}, nil
}
func (f *lifecycleFixture) SubmitTDSSave(ctx context.Context, mode, id string, d ds.TDSDestination, description *string) (value.TDSReceipt, error) {
	f.publishCalls++
	if f.submit != nil {
		return f.submit()
	}
	resultID := newUUID
	if mode == "overwrite" {
		resultID = sourceUUID
	}
	f.publishedValue = f.copyValue
	return value.TDSReceipt{Status: "acknowledged", DatasourceLUID: resultID, TableauRequestID: "publish-1"}, nil
}
func (f *lifecycleFixture) ReadTDSElements(_ context.Context, kind, id, typ, name string) (value.TDSRead, error) {
	if kind == "workcopy" && id != copyUUID && id != alternateCopyUUID {
		return value.TDSRead{}, errors.New("wrong copy")
	}
	if name != "" && name != "[Sales]" {
		return value.TDSRead{Absent: true}, nil
	}
	v := f.copyValue
	if kind == "datasource" {
		v = f.publishedValue
	}
	e := value.TDSElement{Name: "[Sales]"}
	switch typ {
	case "Column":
		e.Column = &value.TDSColumn{}
	case "FieldDescription":
		e.Description = &v
	default:
		return value.TDSRead{}, errors.New("unsupported test type")
	}
	return value.TDSRead{Elements: []value.TDSElement{e}}, nil
}
func checkoutIn() ds.CheckoutInput {
	return ds.CheckoutInput{Scope: tdsScope, Environment: "dev", Selector: identity.Selector{LUID: sourceUUID}}
}
func saveIn(mode string) ds.SaveWorkCopyInput {
	in := ds.SaveWorkCopyInput{Mode: mode, Scope: tdsScope, Environment: "dev", WorkCopyLUID: copyUUID, Fields: []string{"[Sales]"}}
	if mode == "as-new" {
		in.Name = "Report"
	}
	return in
}
func TestCheckoutAndSaveReadbackAndRecovery(t *testing.T) {
	for _, mode := range []string{"overwrite", "as-new"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			j := operationrun.TDSJournal{Directory: filepath.Join(t.TempDir(), "tds-attempts")}
			f := &lifecycleFixture{revision: "v1", copyValue: "private edit description", destName: "Report"}
			c := checkoutIn()
			var notices bytes.Buffer
			c.AttemptOutput = &notices
			preview, err := ds.Checkout(ctx, f, f, j, c, true)
			if err != nil || preview.WorkCopyLUID != "" || f.copyCreateCalls != 0 {
				t.Fatalf("preview %+v %v", preview, err)
			}
			out, err := ds.Checkout(ctx, f, f, j, c, false)
			if err != nil || out.WorkCopyLUID != copyUUID || !out.RevisionKnown || !strings.Contains(notices.String(), out.AttemptID) {
				t.Fatalf("checkout %+v %v", out, err)
			}
			origin, err := j.Origin(ctx, tdsScope, copyUUID)
			if err != nil || origin.ID != out.AttemptID || origin.SourceRevision != "v1" {
				t.Fatalf("origin %+v %v", origin, err)
			}
			s := saveIn(mode)
			if mode == "as-new" {
				s.Description = new("private published metadata")
			}
			pv, err := ds.SaveWorkCopy(ctx, f, j, s, true)
			if err != nil || pv.PreviewToken == "" || f.publishCalls != 0 {
				t.Fatalf("preview %+v %v", pv, err)
			}
			s.ExpectPreview = pv.PreviewToken
			f.copyValue = "changed elsewhere"
			if _, err := ds.SaveWorkCopy(ctx, f, j, s, false); err == nil || f.publishCalls != 0 {
				t.Fatalf("stale preview submitted a save")
			}
			f.copyValue = "private edit description"
			saved, err := ds.SaveWorkCopy(ctx, f, j, s, false)
			if err != nil || saved.Verification != "matched" || f.publishCalls != 1 {
				t.Fatalf("save %+v %v", saved, err)
			}
			rows, _, err := j.Attempts(ctx, tdsScope, "", "", copyUUID)
			if err != nil || len(rows) != 2 || rows[0].ID != saved.AttemptID || rows[0].PublishedLUID != saved.PublishedLUID {
				t.Fatalf("recovery %+v %v", rows, err)
			}
			observed, err := ds.VerifySaveAttempt(ctx, f, f, rows[0])
			if err != nil || observed.Verification != "matched" || f.publishCalls != 1 {
				t.Fatalf("verify %+v %v", observed, err)
			}
			files, _ := filepath.Glob(filepath.Join(j.Directory, "*", "attempts.json"))
			if len(files) != 1 {
				t.Fatal("missing private ledger")
			}
			data, err := os.ReadFile(files[0])
			if err != nil || bytes.Contains(data, []byte("private edit description")) || bytes.Contains(data, []byte("private published metadata")) {
				t.Fatal("raw descriptions in receipt or unreadable ledger")
			}
		})
	}
}
func TestCheckoutUnknownAndDuplicateAdmission(t *testing.T) {
	ctx := context.Background()
	j := operationrun.TDSJournal{Directory: filepath.Join(t.TempDir(), "tds-attempts")}
	entered := make(chan struct{})
	finish := make(chan struct{})
	done := make(chan error, 1)
	f := &lifecycleFixture{create: func() (value.TDSReceipt, error) {
		close(entered)
		<-finish
		return value.TDSReceipt{Status: "unknown", TableauRequestID: "lost"}, errors.New("interrupted response")
	}}
	go func() { _, err := ds.Checkout(ctx, f, f, j, checkoutIn(), false); done <- err }()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("checkout stopped before provider dispatch: %v", err)
	}
	// A second composition using a different profile resolves to the same account
	// journal and authenticated scope.
	other := operationrun.TDSJournal{Directory: j.Directory}
	otherIn := checkoutIn()
	otherIn.Environment = "alias"
	if _, err := ds.Checkout(ctx, f, f, other, otherIn, false); err == nil {
		t.Fatal("concurrent checkout accepted")
	}
	close(finish)
	err := <-done
	var e *errs.Error
	if !errors.As(err, &e) || e.Outcome != errs.OutcomeUnknown || f.copyCreateCalls != 1 {
		t.Fatal(err)
	}
	a, more, err := other.Attempts(ctx, tdsScope, "", sourceUUID, "")
	if err != nil || more || len(a) != 1 || a[0].Status != "unknown" || a[0].RequestID != "lost" {
		t.Fatalf("lost recovery %+v %v", a, err)
	}
	if _, err := ds.Checkout(ctx, f, f, j, checkoutIn(), false); err == nil || f.copyCreateCalls != 1 {
		t.Fatal("replayed unknown checkout")
	}
}
func TestSaveFailuresPreserveAttribution(t *testing.T) {
	cases := []struct {
		name, mode, status, wantID, wantErr string
		collision                           bool
	}{
		{"preflight collision", "as-new", "", "", "not_attempted", true},
		{"raced collision", "as-new", "rejected", "", "not_applied", false},
		{"lost overwrite", "overwrite", "unknown", "", "unknown", false},
		{"contradictory overwrite", "overwrite", "contradictory", newUUID, "unknown", false},
		{"original on create", "as-new", "contradictory", sourceUUID, "unknown", false},
		{"published metadata differs", "as-new", "acknowledged", newUUID, "confirmed", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			j := operationrun.TDSJournal{Directory: filepath.Join(t.TempDir(), "tds-attempts")}
			f := &lifecycleFixture{revision: "v1", copyValue: "value", publishedValue: "value", destName: "Report"}
			if _, err := ds.Checkout(ctx, f, f, j, checkoutIn(), false); err != nil {
				t.Fatal(err)
			}
			f.collision = c.collision
			f.submit = func() (value.TDSReceipt, error) {
				if c.status == "contradictory" {
					return value.TDSReceipt{Status: "acknowledged", DatasourceLUID: c.wantID}, nil
				}
				if c.status == "acknowledged" {
					f.destName = "wrong"
					return value.TDSReceipt{Status: "acknowledged", DatasourceLUID: c.wantID}, nil
				}
				return value.TDSReceipt{Status: c.status, TableauRequestID: "request-2"}, errors.New("fixture refusal or interruption")
			}
			out, err := ds.SaveWorkCopy(ctx, f, j, saveIn(c.mode), false)
			var e *errs.Error
			if !errors.As(err, &e) || string(e.Outcome) != c.wantErr {
				t.Fatalf("out=%+v err=%+v", out, e)
			}
			if !slices.Contains([]string{out.PublishedLUID, out.ReportedLUID}, c.wantID) {
				t.Fatalf("lost identity %+v", out)
			}
			if c.collision {
				if f.publishCalls != 0 {
					t.Fatal("published over existing name")
				}
				return
			}
			a, _, err := j.Attempts(ctx, tdsScope, out.AttemptID, "", "")
			if err != nil || len(a) != 1 || a[0].Status != out.Status {
				t.Fatalf("bad durable outcome %+v %v", a, err)
			}
			if c.mode == "overwrite" && out.Status == "unknown" {
				verified, err := ds.VerifySaveAttempt(ctx, f, f, a[0])
				if err == nil || verified.Status != "unknown" || verified.Verification != "matched" || f.publishCalls != 1 {
					t.Fatalf("attributed an unknown write %+v %v", verified, err)
				}
			}
		})
	}
}
func TestSaveRequiresFreshOriginAndIntent(t *testing.T) {
	ctx := context.Background()
	j := operationrun.TDSJournal{Directory: filepath.Join(t.TempDir(), "tds-attempts")}
	f := &lifecycleFixture{revision: "v1", copyValue: "before", destName: "Report"}
	if _, err := ds.SaveWorkCopy(ctx, f, j, saveIn("overwrite"), false); err == nil {
		t.Fatal("accepted unknown origin")
	}
	if _, err := ds.Checkout(ctx, f, f, j, checkoutIn(), false); err != nil {
		t.Fatal(err)
	}
	f.revision = "v2"
	if _, err := ds.SaveWorkCopy(ctx, f, j, saveIn("overwrite"), false); err == nil {
		t.Fatal("accepted stale origin")
	}
	s := saveIn("as-new")
	s.Fields = nil
	s.FromChanges = true
	s.Changes = []ds.EditChange{{Op: "set-field-description", Field: "[Sales]", Description: "intended"}}
	for _, pv := range []bool{true, false} {
		o, err := ds.SaveWorkCopy(ctx, f, j, s, pv)
		if err == nil || o.PreviewToken != "" || len(o.Observations) != 1 || o.Observations[0].Status != "mismatch" {
			t.Fatalf("stale intent %+v %v", o, err)
		}
	}
	if f.publishCalls != 0 {
		t.Fatal("submitted without preconditions")
	}
}

// A separate executable exits from inside the provider or journal. Go defers
// never run in that process, including the resource unlock and stdout render.
func TestTDSRecoveryAfterProcessExit(t *testing.T) {
	ctx := context.Background()
	if root := os.Getenv("TADX_TEST_TDS_CRASH_DIRECTORY"); root != "" {
		stage := os.Getenv("TADX_TEST_TDS_CRASH_STAGE")
		j := operationrun.TDSJournal{Directory: root}
		f := &lifecycleFixture{revision: "v1", copyValue: "intent", publishedValue: "intent", destName: "Report"}
		var store ds.TDSJournal = j
		switch stage {
		case "during-checkout":
			f.create = func() (value.TDSReceipt, error) { os.Exit(42); return value.TDSReceipt{}, nil }
		case "after-checkout":
			store = cutPowerJournal{TDSJournal: j, op: "datasource.checkout"}
		case "after-publish":
			store = cutPowerJournal{TDSJournal: j, op: "datasource.save.as-new"}
		default:
			t.Fatal("invalid crash position")
		}
		if _, err := ds.Checkout(ctx, f, f, store, checkoutIn(), false); err != nil {
			t.Fatal(err)
		}
		if _, err := ds.SaveWorkCopy(ctx, f, store, saveIn("as-new"), false); err != nil {
			t.Fatal(err)
		}
		t.Fatal("fault injection did not terminate the process")
	}
	for _, stage := range []string{"during-checkout", "after-checkout", "after-publish"} {
		t.Run(stage, func(t *testing.T) {
			j := operationrun.TDSJournal{Directory: filepath.Join(t.TempDir(), "state")}
			// No stdout, stderr, --config, or workspace survives to identify the attempt.
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestTDSRecoveryAfterProcessExit$")
			cmd.Env = append(os.Environ(), "TADX_TEST_TDS_CRASH_DIRECTORY="+j.Directory, "TADX_TEST_TDS_CRASH_STAGE="+stage)
			output, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 42 {
				t.Fatalf("child=%s err=%v", output, err)
			}
			source, copy := sourceUUID, ""
			if stage == "after-publish" {
				source = ""
				copy = copyUUID
			}
			a, _, err := j.Attempts(ctx, tdsScope, "", source, copy)
			if err != nil || len(a) == 0 {
				t.Fatalf("lookup=%+v %v", a, err)
			}
			f := &lifecycleFixture{destName: "Report", publishedValue: "intent"}
			switch stage {
			case "during-checkout":
				if a[0].Status != "pending" || a[0].WorkCopyLUID != "" {
					t.Fatalf("invented checkout receipt %+v", a[0])
				}
				if _, err := ds.Checkout(ctx, f, f, j, checkoutIn(), false); err == nil || f.copyCreateCalls != 0 {
					t.Fatal("retried interrupted submission")
				}
				in := checkoutIn()
				in.AcknowledgeUnknown = []string{a[0].ID}
				if _, err := ds.Checkout(ctx, f, f, j, in, false); err != nil {
					t.Fatal(err)
				}
			case "after-checkout":
				if a[0].Status != "acknowledged" || a[0].WorkCopyLUID != copyUUID || a[0].SourceRevision != "" {
					t.Fatalf("lost copy or invented post-checkout evidence %+v", a[0])
				}
			case "after-publish":
				if a[0].Status != "acknowledged" || a[0].PublishedLUID != newUUID || a[0].Verification != "pending" {
					t.Fatalf("lost published identity %+v", a[0])
				}
				o, err := ds.VerifySaveAttempt(ctx, f, f, a[0])
				if err != nil || o.Verification != "matched" || f.publishCalls != 0 {
					t.Fatalf("read-only recovery %+v %v", o, err)
				}
			}
		})
	}
}

type cutPowerJournal struct {
	ds.TDSJournal
	op string
}

func (j cutPowerJournal) Record(ctx context.Context, a value.TDSAttempt) error {
	if err := j.TDSJournal.Record(ctx, a); err != nil {
		return err
	}
	if a.Operation == j.op && a.Status == "acknowledged" {
		os.Exit(42)
	}
	return nil
}

type persistenceFailure struct{ ds.TDSJournal }

func (j persistenceFailure) Record(context.Context, value.TDSAttempt) error {
	return errors.New("fixture persistence failure")
}

func TestCheckoutPreservesConfirmedIDOnLocalFailure(t *testing.T) {
	ctx := context.Background()
	j := operationrun.TDSJournal{Directory: filepath.Join(t.TempDir(), "state")}
	f := &lifecycleFixture{}
	out, err := ds.Checkout(ctx, f, f, persistenceFailure{j}, checkoutIn(), false)
	var e *errs.Error
	if !errors.As(err, &e) || e.Phase != errs.PhasePersistence || e.Outcome != errs.OutcomeConfirmed || out.WorkCopyLUID != copyUUID || out.AttemptID == "" {
		t.Fatalf("%+v %+v", out, e)
	}
	a, _, err := j.Attempts(ctx, tdsScope, out.AttemptID, "", "")
	if err != nil || a[0].Status != "pending" {
		t.Fatalf("bad blocker after failed persist: %+v %v", a, err)
	}
}

const alternateCopyUUID = "44444444-4444-4444-4444-444444444444"

func TestOverwriteFromTwoCopiesGuardsTheOriginal(t *testing.T) {
	ctx := context.Background()
	j := operationrun.TDSJournal{Directory: filepath.Join(t.TempDir(), "state")}
	f := &lifecycleFixture{revision: "v1", copyValue: "new", destName: "Report"}
	next := copyUUID
	f.create = func() (value.TDSReceipt, error) {
		return value.TDSReceipt{Status: "acknowledged", WorkCopyLUID: next}, nil
	}
	for _, id := range []string{copyUUID, alternateCopyUUID} {
		next = id
		o, err := ds.Checkout(ctx, f, f, j, checkoutIn(), false)
		if err != nil || o.WorkCopyLUID != id || !o.RevisionKnown {
			t.Fatal(o, err)
		}
	}
	entered, finish := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	f.submit = func() (value.TDSReceipt, error) {
		close(entered)
		<-finish
		f.revision = "v2"
		f.publishedValue = "new"
		return value.TDSReceipt{Status: "acknowledged", DatasourceLUID: sourceUUID}, nil
	}
	go func() { _, err := ds.SaveWorkCopy(ctx, f, j, saveIn("overwrite"), false); done <- err }()
	select {
	case <-entered:
	case err := <-done:
		t.Fatal("first save never reached dispatch", err)
	}
	second := saveIn("overwrite")
	second.WorkCopyLUID = alternateCopyUUID
	_, err := ds.SaveWorkCopy(ctx, f, j, second, false)
	close(finish)
	firstErr := <-done
	if err == nil || firstErr != nil {
		t.Fatalf("concurrent result second %v first %v", err, firstErr)
	}
	if _, err = ds.SaveWorkCopy(ctx, f, j, second, false); err == nil || f.publishCalls != 1 {
		t.Fatalf("second copy overwrote a changed initial revision: %v", err)
	}
}

type incompleteTDSCollection struct{ *lifecycleFixture }

func (f incompleteTDSCollection) ReadTDSElements(ctx context.Context, kind, id, element, name string) (value.TDSRead, error) {
	if name == "" {
		return value.TDSRead{Elements: []value.TDSElement{{Name: "outside exact scope", Error: &value.TDSStatus{Code: 13}}}}, nil
	}
	return f.lifecycleFixture.ReadTDSElements(ctx, kind, id, element, name)
}
func TestSaveExactScopeSurvivesIncompleteCollection(t *testing.T) {
	ctx := context.Background()
	j := operationrun.TDSJournal{Directory: filepath.Join(t.TempDir(), "state")}
	f := incompleteTDSCollection{&lifecycleFixture{revision: "v1", copyValue: "verifiable", destName: "Report"}}
	s := saveIn("as-new")
	s.Fields = nil
	if out, err := ds.SaveWorkCopy(ctx, f, j, s, true); err == nil || out.PreviewToken != "" {
		t.Fatalf("collection accepted %+v %v", out, err)
	}
	s.FromChanges = true
	s.Changes = []ds.EditChange{{Op: "set-field-description", Field: "[Sales]", Description: "verifiable"}}
	out, err := ds.SaveWorkCopy(ctx, f, j, s, false)
	if err != nil || out.Verification != "matched" || f.publishCalls != 1 {
		t.Fatal(out, err)
	}
}

type unwritableAttemptStream struct{}

func (unwritableAttemptStream) Write([]byte) (int, error) {
	return 0, errors.New("closed before dispatch")
}

func TestStoppedCheckoutPreservesEarlierBlocker(t *testing.T) {
	ctx := context.Background()
	j := operationrun.TDSJournal{Directory: filepath.Join(t.TempDir(), "state")}
	f := &lifecycleFixture{create: func() (value.TDSReceipt, error) {
		return value.TDSReceipt{Status: "unknown"}, errors.New("response lost")
	}}
	in := checkoutIn()
	out, err := ds.Checkout(ctx, f, f, j, in, false)
	if err == nil || out.Status != "unknown" {
		t.Fatal(out, err)
	}
	in.AcknowledgeUnknown = []string{out.AttemptID}
	in.AttemptOutput = unwritableAttemptStream{}
	stopped, err := ds.Checkout(ctx, f, f, j, in, false)
	if err == nil || stopped.AttemptID == "" || f.copyCreateCalls != 1 {
		t.Fatal(stopped, err)
	}
	prior, _, err := j.Attempts(ctx, tdsScope, out.AttemptID, "", "")
	if err != nil || len(prior) != 1 || prior[0].SupersededBy != "" {
		t.Fatal(prior, err)
	}
	in.AcknowledgeUnknown = nil
	if _, err := ds.Checkout(ctx, f, f, j, in, false); err == nil || f.copyCreateCalls != 1 {
		t.Fatal("local output failure consumed prior uncertainty")
	}
}

func TestLifecycleCompactRetainsLateMismatchesAndPreviewSettings(t *testing.T) {
	o := ds.LifecycleOutput{Operation: "datasource.save.as-new", Mode: "preview", RequestID: "req-1", Name: "Published", ProjectLUID: "project", Description: new("destination"), WorkCopyLUID: copyUUID}
	for i := 0; i < 13; i++ {
		o.Observations = append(o.Observations, ds.SaveObservation{Selector: ds.EditSelector{Name: fmt.Sprintf("field-%d", i)}, Status: "observed"})
	}
	o.Observations[12].Status = "mismatch"
	compact := o.CompactOutput().(ds.LifecycleOutput)
	full := o.FullOutput().(ds.LifecycleOutput)
	if compact.RequestID != "" || compact.Name != o.Name || compact.Description != o.Description || compact.OmittedObservations != 2 || compact.Observations[10].Status != "mismatch" || compact.Details != "--full" || len(full.Observations) != 13 || full.RequestID != o.RequestID {
		t.Fatal(compact, full)
	}
	attempts := ds.DescribeTDSAttempts("other profile", []value.TDSAttempt{{ID: "attempt", Scope: tdsScope, SourceLUID: sourceUUID, WorkCopyLUID: copyUUID, PublishedLUID: newUUID, Operation: "datasource.save.as-new", Status: "acknowledged", DestinationDigest: "sensitive-hash"}}, false)
	cb, _ := json.Marshal(attempts.CompactOutput())
	fb, _ := json.Marshal(attempts.FullOutput())
	if bytes.Contains(cb, []byte("sensitive-hash")) || !bytes.Contains(fb, []byte("sensitive-hash")) || !bytes.Contains(cb, []byte("--verify")) || !bytes.Contains(cb, []byte("--work-copy")) {
		t.Fatal(string(cb), string(fb))
	}
}

type publishedDescription struct {
	*lifecycleFixture
	present bool
}

func (p publishedDescription) ResolveDatasource(ctx context.Context, s identity.Selector) (ds.Record, error) {
	r, err := p.lifecycleFixture.ResolveDatasource(ctx, s)
	r.Description = ""
	r.DescriptionPresent = p.present
	return r, err
}

func TestSaveChecksExplicitlyEmptyPublishedDescription(t *testing.T) {
	for _, present := range []bool{true, false} {
		j := operationrun.TDSJournal{Directory: filepath.Join(t.TempDir(), "state")}
		f := publishedDescription{&lifecycleFixture{destName: "Report"}, present}
		in := saveIn("as-new")
		in.Description = new("")
		o, err := ds.SaveWorkCopy(t.Context(), f, j, in, false)
		if o.PublishedLUID != newUUID || (err == nil) != present {
			t.Fatalf("present=%t %+v %v", present, o, err)
		}
	}
}

type protectedCheckout struct{ *lifecycleFixture }

func (p protectedCheckout) TDSRedactor() func(string) string {
	return func(s string) string { return strings.ReplaceAll(s, "fixture-protected", "[REDACTED]") }
}
func TestCheckoutProtectsSourceMetadataBeforeReceipt(t *testing.T) {
	j := operationrun.TDSJournal{Directory: filepath.Join(t.TempDir(), "state")}
	f := protectedCheckout{&lifecycleFixture{destName: "observed fixture-protected data"}}
	for _, preview := range []bool{true, false} {
		o, err := ds.Checkout(t.Context(), f, f, j, checkoutIn(), preview)
		projection, _ := json.Marshal(o)
		if err == nil || o.AttemptID != "" || f.copyCreateCalls != 0 || bytes.Contains(projection, []byte("fixture-protected")) || strings.Contains(err.Error(), "fixture-protected") {
			t.Fatal("credential-bearing source escaped checkout boundary")
		}
	}
	attempts, _, err := j.Attempts(t.Context(), tdsScope, "", "", "")
	if err != nil || len(attempts) != 0 {
		t.Fatal("credential-bearing attempt reached journal")
	}
}
