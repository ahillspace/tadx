package harness

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Observe repeats only a read-only assertion. Callers must keep mutations out
// of check and retain the last contrasting observation when the deadline ends.
func Observe(ctx context.Context, budget time.Duration, check func() error) error {
	if budget <= 0 {
		return errors.New("observation requires a positive deadline")
	}
	bounded, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	delay := 100 * time.Millisecond
	for {
		err := check()
		if err == nil {
			return nil
		}
		if p, ok := errors.AsType[*PrerequisiteError](err); ok && p.Status != InconclusiveStatus {
			return err
		}
		if h, ok := errors.AsType[*HTTPError](err); ok && (h.Status == 401 || h.Status == 403) {
			return err
		}
		timer := time.NewTimer(delay)
		select {
		case <-bounded.Done():
			timer.Stop()
			if ctx.Err() != nil {
				return Inconclusive("read-only observation ended before its contract could be established")
			}
			return fmt.Errorf("read-only verification deadline: %w", err)
		case <-timer.C:
		}
		delay = min(delay*2, 2*time.Second)
	}
}
func (c *Case) Observe(ctx context.Context, check func() error) error {
	return Observe(ctx, time.Duration(c.Profile.Budgets.ObservationSeconds)*time.Second, check)
}
