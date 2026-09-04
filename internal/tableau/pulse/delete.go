package pulse

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// DeleteResult preserves the exact identity and upstream response status.
type DeleteResult struct {
	Status           string
	LUID             string
	HTTPStatus       int
	TableauRequestID string
}

// DeleteDefinition deletes one definition and leaves dependency behavior to Tableau.
func (c *Client) DeleteDefinition(ctx context.Context, luid string) (DeleteResult, error) {
	return c.deleteExact(ctx, "definition", luid)
}

// DeleteMetric deletes one metric without deleting its parent or subscriptions client-side.
func (c *Client) DeleteMetric(ctx context.Context, luid string) (DeleteResult, error) {
	return c.deleteExact(ctx, "metric", luid)
}

func (c *Client) deleteExact(ctx context.Context, kind, luid string) (DeleteResult, error) {
	if strings.TrimSpace(luid) == "" {
		return DeleteResult{}, errors.New("Pulse " + kind + " LUID is required")
	}
	response, err := c.do(ctx, http.MethodDelete, pulsePath+"/"+kind+"s/"+url.PathEscape(luid), nil, nil, "", "", "pulse."+kind+".delete")
	if err != nil {
		return DeleteResult{}, err
	}
	status := "deleted"
	if response.StatusCode == http.StatusAccepted {
		status = "accepted"
	}
	return DeleteResult{Status: status, LUID: luid, HTTPStatus: response.StatusCode, TableauRequestID: response.TableauRequestID}, nil
}
