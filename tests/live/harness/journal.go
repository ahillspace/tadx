package harness

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Entry struct {
	Kind         string          `json:"kind"`
	Name         string          `json:"name"`
	Parent       string          `json:"parent,omitempty"`
	UserLogin    string          `json:"user_login,omitempty"`
	UserTemplate string          `json:"user_template,omitempty"`
	UserToken    string          `json:"user_token,omitempty"`
	Expected     map[string]any  `json:"expected,omitempty"`
	Object       *Object         `json:"object,omitempty"`
	Method       string          `json:"restore_method,omitempty"`
	Path         string          `json:"restore_path,omitempty"`
	Payload      json.RawMessage `json:"restore_payload,omitempty"`
	Done         bool            `json:"done,omitzero"`
}

type OperationClosure struct {
	Phase               string    `json:"phase"`
	WorkerAlive         *bool     `json:"worker_alive"`
	WorkerAliveObserved bool      `json:"worker_alive_observed,omitzero"`
	ObservedAt          time.Time `json:"observed_at,omitzero"`
	EvidenceSource      string    `json:"evidence_source,omitempty"`
	EvidenceCommand     int       `json:"evidence_command,omitzero"`
}

func (j *Journal) SetLabelExpectation(name, parent string, expected map[string]any) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	for index, entry := range j.Entries {
		if entry.Done || entry.Kind != "label" || entry.Name != name || entry.Parent != parent || entry.Object != nil {
			continue
		}
		copy := make(map[string]any, len(expected))
		for key, value := range expected {
			copy[key] = value
		}
		j.Entries[index].Expected = copy
		return j.save()
	}
	return errors.New("label expectation requires its exact prior ownership intent")
}

type Journal struct {
	Version              int                         `json:"version"`
	RunID                string                      `json:"run_id"`
	Target               string                      `json:"target_fingerprint"`
	Prefix               string                      `json:"prefix"`
	Entries              []Entry                     `json:"entries"`
	Jobs                 []string                    `json:"accepted_jobs,omitempty"`
	Operations           []string                    `json:"accepted_operations,omitempty"`
	OperationClosures    map[string]OperationClosure `json:"operation_closures,omitempty"`
	TransferPending      bool                        `json:"transfer_pending,omitzero"`
	RequiredCapabilities []string                    `json:"required_capabilities,omitempty"`
	path                 string
	mu                   sync.Mutex
}

func NewJournal(path, run, target, prefix string) (*Journal, error) {
	j := &Journal{Version: 1, RunID: run, Target: target, Prefix: prefix, path: path}
	if run == "" || target == "" || !strings.HasPrefix(prefix, "tadx-live-") {
		return nil, errors.New("journal requires run, target, and generated owned prefix")
	}
	if _, err := os.Lstat(path); err == nil {
		return nil, errors.New("journal already exists; resume cleanup instead of overwriting ownership")
	}
	return j, j.save()
}
func LoadJournal(path, target string) (*Journal, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) > 8<<20 {
		return nil, errors.New("journal exceeds size limit")
	}
	var j Journal
	if err = json.Unmarshal(data, &j); err != nil {
		return nil, errors.New("invalid cleanup journal")
	}
	if j.Version != 1 || j.Target != target || j.RunID == "" || !strings.HasPrefix(j.Prefix, "tadx-live-") {
		return nil, errors.New("cleanup journal target or version mismatch")
	}
	for _, en := range j.Entries {
		if en.Method == "" && !strings.HasPrefix(en.Name, j.Prefix) {
			return nil, errors.New("journal contains resource outside its owned namespace")
		}
		if en.UserLogin != "" || en.UserTemplate != "" || en.UserToken != "" {
			if en.Kind != "user" || !validControlledUserEntry(&j, en) {
				return nil, errors.New("journal contains an invalid controlled user ownership record")
			}
		}
		if en.Method != "" && !safeRestorePath(en.Path) {
			return nil, errors.New("journal contains an unsafe restoration path")
		}
	}
	for id, closure := range j.OperationClosures {
		if !slices.Contains(j.Operations, id) || !validOperationClosure(closure) {
			return nil, errors.New("cleanup journal contains invalid accepted-operation closure evidence")
		}
	}
	j.path = path
	return &j, nil
}
func (j *Journal) save() error { return WriteJSON(j.path, j) }

func WriteJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	f, err := os.CreateTemp(filepath.Dir(path), ".live-state-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	if err = f.Chmod(0o600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	return os.Rename(temp, path)
}

func (j *Journal) AddIntent(kind, name, parent string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if kind == "" || name == "" || !strings.HasPrefix(name, j.Prefix) {
		return errors.New("remote ownership intent must use the generated test namespace")
	}
	for _, en := range j.Entries {
		if !en.Done && en.Kind == kind && en.Name == name && en.Parent == parent {
			return nil
		}
	}
	j.Entries = append(j.Entries, Entry{Kind: kind, Name: name, Parent: parent})
	return j.save()
}

func (j *Journal) AddControlledUserIntent(ownerKey, login, template, token string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	entry := Entry{Kind: "user", Name: ownerKey, UserLogin: login, UserTemplate: template, UserToken: token}
	if !strings.HasPrefix(ownerKey, j.Prefix+"-user-") || !validControlledUserEntry(j, entry) {
		return errors.New("controlled user intent does not match its exact case-scoped template")
	}
	for _, existing := range j.Entries {
		if existing.Done || existing.Kind != "user" {
			continue
		}
		if existing.Name == ownerKey {
			if existing.UserLogin == login && existing.UserTemplate == template && existing.UserToken == token {
				return nil
			}
			return errors.New("controlled user owner key already has a different login intent")
		}
		if existing.UserLogin == login {
			return errors.New("controlled user login already belongs to another ownership intent")
		}
	}
	j.Entries = append(j.Entries, entry)
	return j.save()
}

func validControlledUserEntry(j *Journal, entry Entry) bool {
	if j == nil || entry.UserLogin == "" || entry.UserTemplate == "" || entry.UserToken == "" || !strings.HasPrefix(entry.Name, j.Prefix+"-user-") {
		return false
	}
	if len(j.RunID) != 20 {
		return false
	}
	if _, err := hex.DecodeString(j.RunID); err != nil {
		return false
	}
	role := strings.TrimPrefix(entry.Name, j.Prefix+"-user-")
	if !validControlledUserRole(role) {
		return false
	}
	expectedToken := controlledUserToken(j.Prefix, j.RunID, role)
	if expectedToken != entry.UserToken {
		return false
	}
	login, err := renderControlledUser(entry.UserTemplate, entry.UserToken)
	return err == nil && login == entry.UserLogin
}

func (j *Journal) ConfirmControlledUserReceipt(ownerKey, login, template, token string, returned Object) error {
	if j == nil || returned.ID == "" || (returned.Kind != "" && returned.Kind != "user") {
		return errors.New("controlled user receipt requires an exact user identity")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	for index, entry := range j.Entries {
		if entry.Done || entry.Kind != "user" || entry.Name != ownerKey || entry.UserLogin != login || entry.UserTemplate != template || entry.UserToken != token {
			continue
		}
		if !validControlledUserEntry(j, entry) {
			return errors.New("controlled user receipt intent is invalid")
		}
		if returned.Name != "" && returned.Name != login {
			return errors.New("controlled user receipt returned a different login")
		}
		if entry.Object != nil && entry.Object.ID != returned.ID {
			return errors.New("controlled user receipt conflicts with its prior returned identity")
		}
		returned.Kind = "user"
		j.Entries[index].Object = &returned
		return j.save()
	}
	return errors.New("refusing to confirm a controlled user without its exact prior ownership intent")
}
func (j *Journal) Track(o Object) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if o.ID == "" || o.Kind == "" {
		return errors.New("confirmed fixture identity is required")
	}
	for i, en := range j.Entries {
		if en.Done || en.Kind != o.Kind {
			continue
		}
		if en.Object != nil && en.Object.ID == o.ID {
			j.Entries[i].Object = &o
			return j.save()
		}
		if en.Object == nil && entryRemoteName(en) == o.Name && (en.Parent == "" || en.Parent == o.ProjectID) {
			j.Entries[i].Object = &o
			return j.save()
		}
	}
	return errors.New("refusing to adopt a resource without a prior matching ownership intent")
}

// ConfirmReceipt binds one fresh operation's announced identity to its exact
// pre-recorded request, before independent field hydration can fail.
func (c *Case) ConfirmReceipt(kind, requestedName, parent string, returned Object) error {
	if c.Journal == nil {
		return errors.New("confirmed operation receipt requires an ownership journal")
	}
	j := c.Journal
	j.mu.Lock()
	defer j.mu.Unlock()
	if kind == "" || returned.ID == "" || !strings.HasPrefix(requestedName, j.Prefix) {
		return errors.New("confirmed operation receipt requires an owned request and exact returned identity")
	}
	if returned.Kind != "" && returned.Kind != kind {
		return errors.New("confirmed operation receipt has a different resource kind")
	}
	returned.Kind = kind
	for index, entry := range j.Entries {
		if entry.Done || entry.Kind != kind || entry.Name != requestedName || entry.Parent != parent {
			continue
		}
		if entry.Object != nil && entry.Object.ID != returned.ID {
			return errors.New("confirmed operation receipt conflicts with the prior returned identity")
		}
		j.Entries[index].Object = &returned
		return j.save()
	}
	return errors.New("refusing to confirm a receipt without its exact prior ownership request")
}
func safeRestorePath(path string) bool {
	lower := strings.ToLower(path)
	if strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") {
		return false
	}
	decoded, err := url.PathUnescape(path)
	return err == nil && strings.HasPrefix(path, "/api/") && !strings.Contains(decoded, "..") && !strings.ContainsAny(decoded, "\r\n\\?#%")
}
func sameSitePath(api *Client, path string) bool {
	return api != nil && safeRestorePath(path) && (strings.HasPrefix(path, api.REST("")) || strings.HasPrefix(path, "/api/-/pulse/"))
}
func (j *Journal) Restore(method, path string, payload any) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if (method != "PUT" && method != "POST" && method != "DELETE") || !safeRestorePath(path) {
		return errors.New("restoration requires an exact same-origin API path and supported method")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if len(data) > 1<<20 {
		return errors.New("restoration payload exceeds limit")
	}
	j.Entries = append(j.Entries, Entry{Kind: "restore", Method: method, Path: path, Payload: data})
	return j.save()
}
func (c *Case) AcceptJob(id string) error {
	if id == "" {
		return errors.New("accepted job requires an exact identity")
	}
	c.Journal.mu.Lock()
	defer c.Journal.mu.Unlock()
	if !slices.Contains(c.Journal.Jobs, id) {
		c.Journal.Jobs = append(c.Journal.Jobs, id)
	}
	return c.Journal.save()
}
func (c *Case) AcceptOperation(id string) error {
	if id == "" {
		return errors.New("accepted operation requires an exact identity")
	}
	c.Journal.mu.Lock()
	defer c.Journal.mu.Unlock()
	if !slices.Contains(c.Journal.Operations, id) {
		c.Journal.Operations = append(c.Journal.Operations, id)
	}
	delete(c.Journal.OperationClosures, id)
	return c.Journal.save()
}
func (c *Case) GuardOperations(ctx context.Context) error {
	if c.Journal == nil {
		return nil
	}
	if c.Journal.TransferPending {
		return Inconclusive("a native transfer lost its acceptance response; retain local worker state and remote resources")
	}
	for _, id := range c.Journal.Operations {
		if closed, invalid := c.Journal.operationClosureStatus(id); closed {
			continue
		} else if invalid {
			return Inconclusive("accepted operation closure evidence is invalid; retain its operation and resources")
		}
		value, err := c.Run(ctx, "job", "inspect", "--operation-id", id, "--full")
		if err != nil {
			return Inconclusive("local worker observation prevents cleanup; retain its operation and resources")
		}
		tracking, _ := value["tracking"].(map[string]any)
		alive, observed := tracking["worker_alive"].(bool)
		phase, _ := tracking["phase"].(string)
		actual, _ := value["operation_id"].(string)
		if !observed || alive || actual != id || (phase != "completed" && phase != "failed") {
			return Inconclusive("accepted local operation is active or unresolved; cleanup preserves its resources")
		}
		if err := c.Journal.closeOperation(id, phase, alive); err != nil {
			return Inconclusive("could not persist accepted-operation closure evidence; retain its operation and resources")
		}
	}
	return nil
}

func validOperationClosure(closure OperationClosure) bool {
	if closure.WorkerAlive == nil || *closure.WorkerAlive || !closure.WorkerAliveObserved || (closure.Phase != "completed" && closure.Phase != "failed") {
		return false
	}
	if closure.EvidenceSource == "command-history" {
		return closure.ObservedAt.IsZero() && closure.EvidenceCommand > 0
	}
	return closure.EvidenceSource == "" && closure.EvidenceCommand == 0 && !closure.ObservedAt.IsZero()
}

func (j *Journal) closeOperationFromHistory(id, phase string, command int) error {
	if id == "" || command < 1 || (phase != "completed" && phase != "failed") {
		return errors.New("historical operation closure requires exact terminal command evidence")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if !slices.Contains(j.Operations, id) {
		return errors.New("historical operation closure does not match accepted operation")
	}
	if j.OperationClosures == nil {
		j.OperationClosures = make(map[string]OperationClosure)
	}
	closure := OperationClosure{
		Phase: phase, WorkerAlive: new(false), WorkerAliveObserved: true,
		EvidenceSource: "command-history", EvidenceCommand: command,
	}
	previous, existed := j.OperationClosures[id]
	j.OperationClosures[id] = closure
	if err := j.save(); err != nil {
		if existed {
			j.OperationClosures[id] = previous
		} else {
			delete(j.OperationClosures, id)
		}
		return err
	}
	return nil
}

func (j *Journal) operationClosureStatus(id string) (closed, invalid bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	closure, ok := j.OperationClosures[id]
	if !ok {
		return false, false
	}
	if !slices.Contains(j.Operations, id) || !validOperationClosure(closure) {
		return false, true
	}
	return true, false
}

func (j *Journal) closeOperation(id, phase string, alive bool) error {
	if id == "" || alive || (phase != "completed" && phase != "failed") {
		return errors.New("operation closure requires an exact terminal stopped-worker observation")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if !slices.Contains(j.Operations, id) {
		return errors.New("operation closure does not match accepted operation")
	}
	if j.OperationClosures == nil {
		j.OperationClosures = make(map[string]OperationClosure)
	}
	closure := OperationClosure{Phase: phase, WorkerAlive: new(false), WorkerAliveObserved: true, ObservedAt: time.Now().UTC()}
	previous, existed := j.OperationClosures[id]
	j.OperationClosures[id] = closure
	if err := j.save(); err != nil {
		if existed {
			j.OperationClosures[id] = previous
		} else {
			delete(j.OperationClosures, id)
		}
		return err
	}
	return nil
}

// GuardWorkerStopped permits verifier authentication only after every known
// local transfer worker has stopped. Pending remote jobs remain observable.
func (c *Case) GuardWorkerStopped(ctx context.Context) error {
	if c.Journal == nil {
		return nil
	}
	if c.Journal.TransferPending {
		return Inconclusive("native transfer acceptance is unknown; verifier authentication could invalidate its worker")
	}
	for _, id := range c.Journal.Operations {
		value, err := c.Run(ctx, "job", "inspect", "--operation-id", id, "--full")
		if err != nil {
			return Inconclusive("cannot establish that the local worker stopped before verifier authentication")
		}
		tracking, _ := value["tracking"].(map[string]any)
		alive, observed := tracking["worker_alive"].(bool)
		actual, _ := value["operation_id"].(string)
		if !observed || alive || actual != id {
			return Inconclusive("local worker remains active or unobserved; verifier authentication is deferred")
		}
	}
	return nil
}

func terminalJob(o Object) bool {
	for _, key := range []string{"status", "state"} {
		v, _ := o.Fields[key].(string)
		switch strings.ToLower(v) {
		case "success", "succeeded", "completed", "failed", "cancelled", "canceled":
			return true
		}
	}
	progress := -1
	switch n := o.Fields["progress"].(type) {
	case float64:
		progress = int(n)
	case string:
		progress, _ = strconv.Atoi(n)
	case json.Number:
		progress, _ = strconv.Atoi(n.String())
	}
	completed, _ := o.Fields["completedAt"].(string)
	if progress < 100 && completed == "" {
		return false
	}
	if strings.Contains(strings.ToLower(fmt.Sprint(o.Fields["type"])), "bridge") && fmt.Sprint(o.Fields["finishCode"]) == "0" {
		return false
	}
	for _, key := range []string{"finishCode", "finish_code"} {
		if v, ok := o.Fields[key]; ok {
			switch n := v.(type) {
			case float64:
				return n >= 0
			case string:
				return n == "0" || n == "1" || n == "2"
			case json.Number:
				return n == "0" || n == "1" || n == "2"
			}
		}
	}
	return false
}

func (j *Journal) Cleanup(ctx context.Context, api *Client) error {
	// The target lock serializes cleanup. Never hold a journal mutex across
	// observations that can persist a newly recovered operation identity.
	for _, id := range j.Jobs {
		o, err := api.Get(ctx, "job", id)
		if err != nil {
			return fmt.Errorf("accepted job observation prevents cleanup: %w", err)
		}
		if !terminalJob(o) {
			return Inconclusive("accepted job remains active or has unknown terminal state; resources remain journaled")
		}
	}
	var errs []error
	for i := len(j.Entries) - 1; i >= 0; i-- {
		en := &j.Entries[i]
		if en.Done {
			continue
		}
		if len(errs) > 0 {
			break
		}
		if en.Method != "" {
			if !sameSitePath(api, en.Path) {
				return errors.New("restoration path does not belong to the verified site")
			}
			var payload any
			if err := json.Unmarshal(en.Payload, &payload); err != nil {
				errs = append(errs, err)
				continue
			}
			if _, err := api.JSON(ctx, en.Method, en.Path, payload); err != nil && !(en.Method == "DELETE" && IsNotFound(err)) {
				errs = append(errs, err)
				continue
			}
			en.Done = true
			if err := j.save(); err != nil {
				return err
			}
			continue
		}
		o := en.Object
		if o == nil {
			if en.Kind == "label" {
				if err := j.cleanupLabel(ctx, api, en); err != nil {
					errs = append(errs, err)
				}
				continue
			}
			switch en.Kind {
			case "project", "workbook", "datasource", "flow", "group", "user", "labelCategory", "label-category", "labelValue", "label-value", "pulse.definition", "pulseDefinition", "definition":
			default:
				return Inconclusive("unnamed resource creation has no confirmed identity; retain the ownership journal and parent resources")
			}
			items, err := api.List(ctx, en.Kind, nil)
			if err != nil {
				errs = append(errs, fmt.Errorf("resolve uncertain %s intent: %w", en.Kind, err))
				continue
			}
			var matches []Object
			for _, v := range items {
				if v.Name == entryRemoteName(*en) && (en.Parent == "" || v.ProjectID == en.Parent) {
					matches = append(matches, v)
				}
			}
			if len(matches) > 1 {
				errs = append(errs, errors.New("ambiguous pending ownership intent; cleanup stopped for that resource"))
				continue
			}
			if len(matches) == 0 {
				return Inconclusive("an unconfirmed creation is not yet visible; absence does not prove an uncertain mutation never applied")
			}
			o = &matches[0]
			en.Object = o
			if err := j.save(); err != nil {
				return err
			}
		}
		if o.Kind == "label" {
			if err := j.cleanupLabel(ctx, api, en); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		if err := api.Delete(ctx, o.Kind, o.ID); err != nil {
			// Only an authoritative exact GET can establish that an earlier delete succeeded.
			if _, getErr := api.Get(ctx, o.Kind, o.ID); !IsNotFound(getErr) {
				errs = append(errs, fmt.Errorf("delete owned %s: %w", o.Kind, err))
				continue
			}
		}
		if err := Observe(ctx, 5*time.Minute, func() error {
			_, err := api.Get(ctx, o.Kind, o.ID)
			if IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return errors.New("cleanup did not confirm authoritative resource absence")
		}); err != nil {
			errs = append(errs, err)
			continue
		}
		en.Done = true
		if err := j.save(); err != nil {
			return err
		}
	}
	return errors.Join(errs...)
}

func entryRemoteName(entry Entry) string {
	if entry.UserLogin != "" {
		return entry.UserLogin
	}
	return entry.Name
}

func (j *Journal) cleanupLabel(ctx context.Context, api *Client, entry *Entry) error {
	object := entry.Object
	expected := entry.Expected
	if object != nil {
		if object.Kind != "label" || object.ID == "" || object.ProjectID == "" || object.Fields == nil {
			return Inconclusive("confirmed label receipt lacks exact ID, parent, or returned tuple fields")
		}
		expected = object.Fields
	}
	if expected == nil {
		return Inconclusive("pending label intent lacks its pre-recorded exact request tuple")
	}
	contentID, _ := expected["contentId"].(string)
	contentType, _ := expected["contentType"].(string)
	if contentID == "" || contentType == "" || entry.Parent != contentID || (object != nil && object.ProjectID != contentID) {
		return Inconclusive("label ownership intent lacks its exact parent asset context")
	}
	labels, err := api.QueryLabels(ctx, contentType, contentID)
	if err != nil {
		return fmt.Errorf("observe scoped label attachments: %w", err)
	}
	if object == nil {
		var matches []Object
		for _, label := range labels {
			if labelMatches(label, expected) {
				matches = append(matches, label)
			}
		}
		if len(matches) != 1 {
			return Inconclusive("pending label intent did not resolve to exactly one complete expected tuple")
		}
		object = &matches[0]
		entry.Object = object
		if err := j.save(); err != nil {
			return err
		}
	}
	var exact *Object
	for _, label := range labels {
		if label.ID != object.ID {
			continue
		}
		if exact != nil || !confirmedLabelIdentityMatches(label, object, contentID, contentType) {
			return Inconclusive("scoped label query returned a duplicate ID or contradictory tuple")
		}
		copy := label
		exact = &copy
	}
	if exact == nil {
		entry.Done = true
		return j.save()
	}
	if err := api.Delete(ctx, "label", object.ID); err != nil && !IsNotFound(err) {
		return fmt.Errorf("delete exact owned label: %w", err)
	}
	if err := Observe(ctx, 5*time.Minute, func() error {
		observed, err := api.QueryLabels(ctx, contentType, contentID)
		if err != nil {
			return err
		}
		for _, label := range observed {
			if label.ID == object.ID {
				return errors.New("scoped label query still contains the owned ID")
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("confirm exact label absence: %w", err)
	}
	entry.Done = true
	return j.save()
}

func confirmedLabelIdentityMatches(label Object, owned *Object, contentID, contentType string) bool {
	actualContentID, _ := label.Fields["contentId"].(string)
	actualContentType, _ := label.Fields["contentType"].(string)
	return label.ID == owned.ID && label.ProjectID == contentID && owned.ProjectID == contentID && actualContentID == contentID && strings.EqualFold(actualContentType, contentType)
}
