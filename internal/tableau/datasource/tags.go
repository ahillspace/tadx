package datasource

import (
	"context"
	"encoding/xml"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/tableau"
	"github.com/ahillspace/tadx/internal/value"
)

type tagEntry struct {
	Label string `xml:"label,attr"`
}
type tagList struct {
	Entries []tagEntry `xml:"tag"`
}

func tagLabels(response tableau.Response, lists []tagList) ([]string, error) {
	if len(lists) != 1 {
		return nil, errors.New("expected exactly one tags container")
	}
	labels := make([]string, 0, len(lists[0].Entries))
	for _, entry := range lists[0].Entries {
		if entry.Label == "" || response.ContainsCredential(entry.Label) {
			return nil, errors.New("tag response contains an unusable label")
		}
		labels = append(labels, entry.Label)
	}
	return labels, nil
}

// Inventory display normalization trims labels. A membership edit needs the
// exact text and proof the tags container was present, even for an empty set.
func (c *Client) ReadDatasourceTags(ctx context.Context, id string) ([]string, error) {
	const op = "datasource.tags.read"
	if err := c.validateTagTarget(id); err != nil {
		return nil, err
	}
	response, err := c.transport.Do(ctx, c.session, tableau.Request{Method: http.MethodGet, ServerURL: c.serverURL, Path: c.sitePath("datasources", id), Operation: op, Accept: "application/xml", MaxResponseBytes: maxListResponseBytes, StructuredFailures: true})
	if err != nil {
		return nil, err
	}
	var body struct {
		XMLName     xml.Name `xml:"tsResponse"`
		Datasources []struct {
			ID   string    `xml:"id,attr"`
			Tags []tagList `xml:"tags"`
		} `xml:"datasource"`
	}
	if response.StatusCode != http.StatusOK || xml.Unmarshal(response.Body, &body) != nil || len(body.Datasources) != 1 {
		return nil, tableau.NewProtocolError(op, response, errors.New("tag read did not return the exact datasource"), true)
	}
	got := body.Datasources[0].ID
	if got != id && !(value.ValidTDSLUID(id) && strings.EqualFold(got, id)) {
		return nil, tableau.NewProtocolError(op, response, errors.New("tag read returned a different datasource identity"), true)
	}
	labels, err := tagLabels(response, body.Datasources[0].Tags)
	if err != nil {
		return nil, tableau.NewProtocolError(op, response, err, true)
	}
	return labels, nil
}

func (c *Client) validateTagTarget(id string) error {
	if strings.TrimSpace(id) == "" || id == "." || id == ".." {
		return errors.New("datasource tags require an exact datasource LUID")
	}
	return c.validate()
}

func (c *Client) AddDatasourceTags(ctx context.Context, id string, labels []string) (value.TDSReceipt, error) {
	const op = "datasource.tags.add"
	if err := c.validateTagTarget(id); err != nil {
		return value.TDSReceipt{}, err
	}
	if len(labels) == 0 {
		return value.TDSReceipt{}, errors.New("tag addition requires at least one label")
	}
	var request struct {
		XMLName xml.Name `xml:"tsRequest"`
		Tags    tagList  `xml:"tags"`
	}
	for _, label := range labels {
		if strings.TrimSpace(label) == "" {
			return value.TDSReceipt{}, errors.New("tag label cannot be blank")
		}
		request.Tags.Entries = append(request.Tags.Entries, tagEntry{Label: label})
	}
	payload, err := xml.Marshal(request)
	if err != nil {
		return value.TDSReceipt{}, err
	}
	response, err := c.transport.Do(ctx, c.session, tableau.Request{Method: http.MethodPut, ServerURL: c.serverURL, Path: c.sitePath("datasources", id, "tags"), Operation: op, Body: payload, ContentType: "application/xml", Accept: "application/xml", MaxResponseBytes: maxListResponseBytes, StructuredFailures: true})
	if err != nil {
		return failedReceipt(err), err
	}
	got := value.TDSReceipt{Status: "unknown", DatasourceLUID: id, TableauRequestID: response.TableauRequestID}
	var body struct {
		XMLName xml.Name  `xml:"tsResponse"`
		Tags    []tagList `xml:"tags"`
	}
	if response.StatusCode != http.StatusOK || xml.Unmarshal(response.Body, &body) != nil {
		return got, tableau.NewProtocolError(op, response, errors.New("expected HTTP 200 and a datasource tags receipt"), false)
	}
	present, err := tagLabels(response, body.Tags)
	if err != nil {
		return got, tableau.NewProtocolError(op, response, err, false)
	}
	for _, label := range labels {
		if !slices.Contains(present, label) {
			return got, tableau.NewProtocolError(op, response, errors.New("tag receipt omitted a requested label"), false)
		}
	}
	got.Status = "acknowledged"
	return got, nil
}

func (c *Client) DeleteDatasourceTag(ctx context.Context, id, label string) (value.TDSReceipt, error) {
	const op = "datasource.tags.remove"
	if err := c.validateTagTarget(id); err != nil {
		return value.TDSReceipt{}, err
	}
	if strings.TrimSpace(label) == "" || label == "." || label == ".." {
		return value.TDSReceipt{}, errors.New("tag removal requires a nonblank label that is not a dot path segment")
	}
	response, err := c.transport.Do(ctx, c.session, tableau.Request{Method: http.MethodDelete, ServerURL: c.serverURL, Path: c.sitePath("datasources", id, "tags", label), Operation: op, Accept: "application/xml", MaxResponseBytes: maxListResponseBytes, StructuredFailures: true})
	if err != nil {
		return failedReceipt(err), err
	}
	got := value.TDSReceipt{Status: "unknown", DatasourceLUID: id, TableauRequestID: response.TableauRequestID}
	if response.StatusCode != http.StatusNoContent || len(response.Body) != 0 {
		return got, tableau.NewProtocolError(op, response, errors.New("expected empty HTTP 204 after tag removal"), false)
	}
	got.Status = "acknowledged"
	return got, nil
}
