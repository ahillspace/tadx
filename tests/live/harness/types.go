// Package harness runs isolated, explicitly selected Tableau CLI contracts.
package harness

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
)

type Scenario func(context.Context, *Case) error

type Binding struct {
	ID       string            `json:"id,omitempty"`
	Path     string            `json:"path,omitempty"`
	Name     string            `json:"name,omitempty"`
	Expected map[string]any    `json:"expected,omitempty"`
	Params   map[string]string `json:"params,omitempty"`
}

type Object struct {
	ID          string         `json:"id"`
	Name        string         `json:"name,omitempty"`
	Kind        string         `json:"kind"`
	ProjectID   string         `json:"project_id,omitempty"`
	OwnerID     string         `json:"owner_id,omitempty"`
	Description string         `json:"description,omitempty"`
	Fields      map[string]any `json:"fields,omitempty"`
}

type Native struct {
	Bytes    []byte
	Filename string
}

type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

type Status string

const (
	Passed             Status = "passed"
	Failed             Status = "failed"
	BlockedStatus      Status = "blocked-prerequisite"
	UnsupportedStatus  Status = "unsupported-target"
	InconclusiveStatus Status = "inconclusive-observation"
	NotSelected        Status = "not-selected"
)

type PrerequisiteError struct {
	Status Status
	Reason string
}

func (e *PrerequisiteError) Error() string { return e.Reason }
func Blocked(reason string) error          { return &PrerequisiteError{BlockedStatus, reason} }
func Unsupported(reason string) error      { return &PrerequisiteError{UnsupportedStatus, reason} }
func Inconclusive(reason string) error     { return &PrerequisiteError{InconclusiveStatus, reason} }
func ErrorStatus(err error) Status {
	if err == nil {
		return Passed
	}
	if e, ok := errors.AsType[*PrerequisiteError](err); ok {
		return e.Status
	}
	return Failed
}

func Assert(ok bool, message string) error {
	if !ok {
		return fmt.Errorf("assertion failed: %s", message)
	}
	return nil
}

type cleanupTask struct {
	name string
	run  func(context.Context) error
}

type Case struct {
	ID, Root, RunDir, NamePrefix, ConfigPath, Environment, Workspace, WorkspacePath string
	TargetFingerprint                                                               string
	ActorUserID                                                                     string
	Profile                                                                         Profile
	API                                                                             *Client
	Fixtures                                                                        map[string]Object
	Journal                                                                         *Journal
	Binary                                                                          string
	Commands                                                                        []Command
	Notes                                                                           []string
	RequiredCapabilities                                                            []string
	ExtraEnv                                                                        []string
	mu                                                                              sync.Mutex
	cleanups                                                                        []cleanupTask
	secrets                                                                         []string
}

func (c *Case) Require(key string) (Binding, error) {
	b, ok := c.Profile.Bindings[key]
	if !ok {
		return Binding{}, Blocked("target profile requires binding " + key)
	}
	return b, nil
}
func (c *Case) Assert(ok bool, message string) error { return Assert(ok, message) }
func (c *Case) Note(message string)                  { c.Notes = append(c.Notes, message) }
func (c *Case) Intent(kind, name, parent string) error {
	return c.Journal.AddIntent(kind, name, parent)
}
func (c *Case) IntentLabel(name, contentType, contentID, value, category string, active, elevated bool) error {
	if err := c.Journal.AddIntent("label", name, contentID); err != nil {
		return err
	}
	return c.Journal.SetLabelExpectation(name, contentID, map[string]any{
		"contentType": contentType, "contentId": contentID, "value": value, "category": category,
		"message": name, "active": active, "elevated": elevated,
	})
}
func (c *Case) Track(o Object) error { return c.Journal.Track(o) }
func (c *Case) RestoreRequest(method, path string, payload any) error {
	if !sameSitePath(c.API, path) {
		return fmt.Errorf("restoration path must belong to the verified site")
	}
	return c.Journal.Restore(method, path, payload)
}
func (c *Case) DeferCleanup(name string, fn func(context.Context) error) {
	c.cleanups = append(c.cleanups, cleanupTask{name, fn})
}

func (c *Case) Cleanup(ctx context.Context) error {
	if err := c.GuardOperations(ctx); err != nil {
		return err
	}
	var errs []error
	for i := len(c.cleanups) - 1; i >= 0; i-- {
		if err := c.cleanups[i].run(ctx); err != nil {
			errs = append(errs, fmt.Errorf("cleanup %s: %w", c.cleanups[i].name, err))
		}
	}
	if err := c.GuardOperations(ctx); err != nil {
		errs = append(errs, err)
		return errors.Join(errs...)
	}
	if c.Journal != nil && c.API != nil {
		errs = append(errs, c.Journal.Cleanup(ctx, c.API))
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	if c.Journal != nil {
		return c.cleanLocalState()
	}
	return nil
}
