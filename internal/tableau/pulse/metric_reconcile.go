package pulse

import (
	"context"
	"errors"
	"time"
)

// ReconcileMetric verifies exact metric, definition, datasource, and site ownership.
// It also waits a bounded time for definition inventory visibility.
func (c *Client) ReconcileMetric(ctx context.Context, expected ExpectedMetric) (Reconciliation, error) {
	if expected.MetricLUID == "" || expected.DefinitionLUID == "" || expected.DatasourceLUID == "" {
		return Reconciliation{}, errors.New("Pulse metric reconciliation requires metric, definition, and datasource LUIDs")
	}
	pollCtx, cancel := context.WithTimeout(ctx, c.pollTimeout)
	defer cancel()
	attempts := 0
	ownershipVerified := false
	lastRequestID := ""
	for {
		attempts++
		metric, err := c.GetMetric(pollCtx, expected.MetricLUID)
		if err != nil {
			if statusCode(err) != 404 {
				return Reconciliation{Status: "failed", Attempts: attempts}, err
			}
		} else {
			lastRequestID = metric.TableauRequestID
			if metric.LUID != expected.MetricLUID || metric.DefinitionLUID != expected.DefinitionLUID {
				return Reconciliation{Status: "ownership_mismatch", Attempts: attempts, TableauRequestID: lastRequestID}, nil
			}
			if expected.SiteLUID != "" && metric.SiteLUID != "" && metric.SiteLUID != expected.SiteLUID {
				return Reconciliation{Status: "ownership_mismatch", Attempts: attempts, TableauRequestID: lastRequestID}, nil
			}
			definition, getErr := c.GetDefinition(pollCtx, expected.DefinitionLUID)
			if getErr != nil {
				return Reconciliation{Status: "failed", Attempts: attempts, TableauRequestID: lastRequestID}, getErr
			}
			lastRequestID = definition.TableauRequestID
			if definition.LUID != expected.DefinitionLUID || definition.DatasourceLUID != expected.DatasourceLUID {
				return Reconciliation{Status: "ownership_mismatch", Attempts: attempts, TableauRequestID: lastRequestID}, nil
			}
			ownershipVerified = true
			page, listErr := c.ListMetrics(pollCtx, expected.DefinitionLUID, PageRequest{PageSize: 100})
			if listErr != nil {
				return Reconciliation{Status: "failed", Attempts: attempts, OwnershipVerified: true, TableauRequestID: lastRequestID}, listErr
			}
			lastRequestID = page.TableauRequestID
			for _, item := range page.Metrics {
				if item.LUID == expected.MetricLUID {
					return Reconciliation{Status: "visible", Attempts: attempts, OwnershipVerified: true, InventoryVisible: true, TableauRequestID: lastRequestID}, nil
				}
			}
		}
		select {
		case <-pollCtx.Done():
			if ctx.Err() != nil {
				return Reconciliation{Status: "failed", Attempts: attempts, OwnershipVerified: ownershipVerified, TableauRequestID: lastRequestID}, ctx.Err()
			}
			return Reconciliation{Status: "pending_visibility", Attempts: attempts, OwnershipVerified: ownershipVerified, TableauRequestID: lastRequestID}, nil
		case <-time.After(c.pollInterval):
		}
	}
}

func statusCode(err error) int {
	var status interface{ HTTPStatus() int }
	if errors.As(err, &status) {
		return status.HTTPStatus()
	}
	return 0
}
