// Package job implements exact Tableau job status and supported cancellation.
package job

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/tableau"
	"github.com/ahillspace/tadx/internal/value"
)

type Client struct {
	transport *tableau.Transport
	session   auth.Session
	server    string
}

func NewClient(transport *tableau.Transport, session auth.Session, server string) *Client {
	return &Client{transport: transport, session: session, server: server}
}

func (c *Client) request(ctx context.Context, id, method, operation string) (tableau.Response, error) {
	if c == nil || c.transport == nil || c.session == nil || strings.TrimSpace(id) == "" {
		return tableau.Response{}, errors.New("exact job client is not configured")
	}
	// Every status request is bounded independently of the remote job duration.
	requestCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	return c.transport.Do(requestCtx, c.session, tableau.Request{Method: method, ServerURL: c.server, Path: "/api/" + url.PathEscape(c.transport.APIVersion()) + "/sites/" + url.PathEscape(c.session.SiteLUID()) + "/jobs/" + url.PathEscape(id), Operation: operation, Accept: "application/xml", MaxResponseBytes: 1 << 20})
}

func (c *Client) Inspect(ctx context.Context, id string) (value.JobStatus, error) {
	response, err := c.request(ctx, id, http.MethodGet, "job.inspect")
	if err != nil {
		return value.JobStatus{}, err
	}
	if response.StatusCode != http.StatusOK {
		return value.JobStatus{}, tableau.NewProtocolError("job.inspect", response, fmt.Errorf("job inspection returned HTTP %d", response.StatusCode), true)
	}
	var envelope struct {
		Jobs []jobXML `xml:"job"`
	}
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return value.JobStatus{}, tableau.NewProtocolError("job.inspect", response, err, true)
	}
	if len(envelope.Jobs) != 1 || envelope.Jobs[0].ID != id {
		return value.JobStatus{}, tableau.NewProtocolError("job.inspect", response, errors.New("job response omitted or changed exact job identity"), true)
	}
	job := envelope.Jobs[0]
	status, err := job.status()
	if err != nil {
		return value.JobStatus{}, tableau.NewProtocolError("job.inspect", response, err, true)
	}
	result := value.JobStatus{ID: id, Type: job.Type, Status: status, Progress: job.Progress, FinishCode: job.FinishCode, CheckedAt: time.Now().UTC(), RequestID: response.TableauRequestID}
	if len(job.Datasources)+len(job.Workbooks) > 1 {
		return result, tableau.NewProtocolError("job.inspect", response, errors.New("job response has ambiguous destination identities"), true)
	}
	if len(job.Datasources) == 1 {
		result.ResourceID = job.Datasources[0].ID
	}
	if len(job.Workbooks) == 1 {
		result.ResourceID = job.Workbooks[0].ID
	}
	return result, nil
}

// Cancel acknowledges only the cancellation request. Callers confirm the state
// separately; Tableau enforces which job types and states support cancellation.
func (c *Client) Cancel(ctx context.Context, id string) (string, error) {
	response, err := c.request(ctx, id, http.MethodPut, "job.cancel")
	if err != nil {
		return tableau.RequestID(err), err
	}
	if response.StatusCode != http.StatusOK {
		return response.TableauRequestID, tableau.NewProtocolError("job.cancel", response, fmt.Errorf("job cancellation returned HTTP %d", response.StatusCode), false)
	}
	return response.TableauRequestID, nil
}

type resourceXML struct {
	ID string `xml:"id,attr"`
}
type jobXML struct {
	ID          string        `xml:"id,attr"`
	Type        string        `xml:"type,attr"`
	Progress    *int          `xml:"progress,attr"`
	FinishCode  *int          `xml:"finishCode,attr"`
	CompletedAt string        `xml:"completedAt,attr"`
	Datasources []resourceXML `xml:"datasource"`
	Workbooks   []resourceXML `xml:"workbook"`
}

func (j jobXML) status() (string, error) {
	if j.Type == "" || j.Progress == nil || j.FinishCode == nil || *j.Progress < 0 || *j.Progress > 100 {
		return "", errors.New("job response omitted valid type, progress, or finish code")
	}
	progress, finish := *j.Progress, *j.FinishCode
	// Bridge code 0 means assignment, not completion, even at progress 100.
	if strings.Contains(strings.ToLower(j.Type), "bridge") && finish == 0 {
		return "running", nil
	}
	if progress < 100 && j.CompletedAt == "" {
		if progress == 0 {
			return "pending", nil
		}
		return "running", nil
	}
	switch finish {
	case 0:
		return "succeeded", nil
	case 1:
		return "failed", nil
	case 2:
		return "cancelled", nil
	case 3:
		if strings.Contains(strings.ToLower(j.Type), "bridge") {
			return "succeeded", nil
		}
	}
	return "", fmt.Errorf("job response has unsupported terminal finish code %d", finish)
}
