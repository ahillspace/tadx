package jobmonitor

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/value"
)

// ObservationSession binds one fresh authentication to an exact native reader.
type ObservationSession struct {
	Server, SiteID string
	Inspect        func(context.Context, string) (value.JobStatus, error)
}

// Observer releases credential coordination between individual exact reads.
type Observer struct {
	Open        func(context.Context) (ObservationSession, error)
	Suspend     func(context.Context) error
	TargetError string
}

func (o Observer) Observe(ctx context.Context, receipts []Receipt) []CheckResult {
	results := make([]CheckResult, len(receipts))
	for i, receipt := range receipts {
		session, err := o.Open(ctx)
		if err == nil {
			if NormalizeServer(receipt.Server) != NormalizeServer(session.Server) || receipt.SiteID != session.SiteID {
				err = errors.New(o.TargetError)
			} else {
				results[i].Status, err = session.Inspect(ctx, receipt.Observation.ID)
				if err == nil && receipt.Observation.Type != "" && results[i].Status.Type != receipt.Observation.Type {
					err = errors.New("job observation changed the accepted job type")
				}
			}
		}
		results[i].Err = errors.Join(err, o.Suspend(context.WithoutCancel(ctx)))
	}
	return results
}

// NormalizeServer preserves the receipt target comparison's canonical form.
func NormalizeServer(server string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(server)), "/")
}
