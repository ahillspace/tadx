package jobmonitor

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// PublicationTarget contains the immutable facts known before native submission.
type PublicationTarget struct {
	Kind, Environment, Server, Site, SiteID string
	ConfigPath, SourcePath, ProjectID, Name string
	CoordinationKey, OperationID            string
}

// PublicationHooks isolate worker, observer, and progress boundaries.
type PublicationHooks struct {
	Deadline       func() time.Time
	StopRequested  func() bool
	Prepare        func(context.Context, Receipt) error
	OnAccepted     func(context.Context, string) error
	NotifyAccepted func(string, string)
	StartWaiting   func(context.Context)
	Suspend        func(context.Context) error
	Observe        func(context.Context, []Receipt) []CheckResult
}

// NewStore resolves the default receipt directory without creating it.
func NewStore(directory string) (Store, error) {
	if directory == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return Store{}, err
		}
		directory = filepath.Join(cache, "tadx", "jobs")
	}
	return Store{Directory: directory}, nil
}

// StartPublication binds durable identity before a remote write can begin.
func StartPublication(ctx context.Context, store Store, target PublicationTarget, hooks PublicationHooks) (*Publication, error) {
	p := &Publication{
		Store: store,
		Base: Receipt{
			ReceiptID: rand.Text(), Version: 1, Operation: target.Kind + ".publish",
			Environment: target.Environment, Server: target.Server, Site: target.Site, SiteID: target.SiteID,
			ConfigPath: target.ConfigPath, SourcePath: filepath.ToSlash(target.SourcePath), ProjectID: target.ProjectID,
			Name: target.Name, CoordinationKey: target.CoordinationKey, OperationID: target.OperationID,
		},
		Deadline: hooks.Deadline, StopRequested: hooks.StopRequested, OnAccepted: hooks.OnAccepted,
		NotifyAccepted: hooks.NotifyAccepted, StartWaiting: hooks.StartWaiting, Suspend: hooks.Suspend, Observe: hooks.Observe,
	}
	if hooks.Prepare != nil {
		if err := hooks.Prepare(ctx, p.Base); err != nil {
			return nil, fmt.Errorf("record publication receipt intent before submission: %w", err)
		}
	}
	return p, nil
}
