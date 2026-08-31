// Package datasource implements the released Tableau datasource REST client family.
package datasource

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/tableau"
)

// Datasource is the authoritative identity projection needed by workbook dependency acquisition.
type Datasource struct {
	LUID        string
	Name        string
	ProjectLUID string
	ProjectName string
}

// Download is a preserved native datasource response.
type Download struct {
	Filename         string
	ContentType      string
	Content          []byte
	TableauRequestID string
}

// Client reads published datasource metadata and native content.
type Client struct {
	transport        *tableau.Transport
	session          auth.Session
	serverURL        string
	maxDownloadBytes int64
}

// NewClient creates an authenticated datasource REST client.
func NewClient(transport *tableau.Transport, session auth.Session, serverURL string) *Client {
	return &Client{transport: transport, session: session, serverURL: serverURL}
}

// SetMaxDownloadBytes bounds the buffered native datasource download below the shared transport ceiling.
// Zero keeps the shared 256 MiB limit.
func (c *Client) SetMaxDownloadBytes(limit int64) {
	if limit > 0 {
		c.maxDownloadBytes = limit
	}
}

// Get returns one published datasource by its authoritative LUID.
func (c *Client) Get(ctx context.Context, datasourceLUID string) (Datasource, error) {
	if datasourceLUID == "" {
		return Datasource{}, errors.New("datasource LUID is required")
	}
	if err := c.validate(); err != nil {
		return Datasource{}, err
	}
	response, err := c.do(ctx, c.sitePath("datasources", datasourceLUID), "datasource.get", 0, nil)
	if err != nil {
		return Datasource{}, err
	}
	var envelope datasourceGetEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return Datasource{}, tableau.NewProtocolError("datasource.get", response, fmt.Errorf("decode datasource response: %w", err), true)
	}
	result := Datasource{
		LUID:        envelope.Datasource.ID,
		Name:        envelope.Datasource.Name,
		ProjectLUID: envelope.Datasource.Project.ID,
		ProjectName: envelope.Datasource.Project.Name,
	}
	if result.LUID != datasourceLUID {
		return Datasource{}, tableau.NewProtocolError("datasource.get", response, fmt.Errorf("datasource response returned LUID %q, expected %q", result.LUID, datasourceLUID), true)
	}
	if result.Name == "" {
		return Datasource{}, tableau.NewProtocolError("datasource.get", response, errors.New("datasource response omitted name"), true)
	}
	if result.ProjectLUID == "" {
		return Datasource{}, tableau.NewProtocolError("datasource.get", response, errors.New("datasource response omitted project LUID"), true)
	}
	if result.ProjectName == "" {
		return Datasource{}, tableau.NewProtocolError("datasource.get", response, errors.New("datasource response omitted project name"), true)
	}
	return result, nil
}

// Download preserves the native TDS or TDSX bytes and filename.
func (c *Client) Download(ctx context.Context, datasourceLUID string, includeExtract *bool) (Download, error) {
	if datasourceLUID == "" {
		return Download{}, errors.New("datasource LUID is required")
	}
	if err := c.validate(); err != nil {
		return Download{}, err
	}
	query := url.Values{}
	if includeExtract != nil {
		query.Set("includeExtract", strconv.FormatBool(*includeExtract))
	}
	response, err := c.do(ctx, c.sitePath("datasources", datasourceLUID, "content"), "datasource.pull", c.maxDownloadBytes, query)
	if err != nil {
		return Download{}, err
	}
	filename := dispositionFilename(response.Header.Get("Content-Disposition"))
	if filename == "" {
		return Download{}, tableau.NewProtocolError("datasource.pull", response, errors.New("datasource download response omitted filename"), true)
	}
	if !validFilename(filename) {
		return Download{}, tableau.NewProtocolError("datasource.pull", response, fmt.Errorf("datasource download returned invalid filename %q", filename), true)
	}
	extension := strings.ToLower(filepath.Ext(filename))
	if extension != ".tds" && extension != ".tdsx" {
		return Download{}, tableau.NewProtocolError("datasource.pull", response, fmt.Errorf("datasource download returned unsupported filename %q", filename), true)
	}
	return Download{
		Filename:         filename,
		ContentType:      response.Header.Get("Content-Type"),
		Content:          response.Body,
		TableauRequestID: response.TableauRequestID,
	}, nil
}

func (c *Client) do(ctx context.Context, path, operation string, maxResponseBytes int64, query url.Values) (tableau.Response, error) {
	if err := c.validate(); err != nil {
		return tableau.Response{}, err
	}
	return c.transport.Do(ctx, c.session, tableau.Request{
		Method:           http.MethodGet,
		ServerURL:        c.serverURL,
		Path:             path,
		Query:            query,
		Accept:           "application/xml",
		Operation:        operation,
		MaxResponseBytes: maxResponseBytes,
	})
}

func (c *Client) validate() error {
	if c == nil || c.transport == nil || c.session == nil {
		return errors.New("authenticated datasource client is not configured")
	}
	return nil
}

func (c *Client) sitePath(parts ...string) string {
	segments := []string{"api", c.transport.APIVersion(), "sites", c.session.SiteLUID()}
	segments = append(segments, parts...)
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	return "/" + strings.Join(segments, "/")
}

func dispositionFilename(value string) string {
	for _, candidate := range []string{value, "attachment; " + value} {
		_, parameters, err := mime.ParseMediaType(candidate)
		if err == nil && parameters["filename"] != "" {
			return parameters["filename"]
		}
	}
	return ""
}

func validFilename(filename string) bool {
	return filename != "." && filename != ".." &&
		!strings.ContainsAny(filename, `/\`) && filepath.Base(filename) == filename
}

type datasourceGetEnvelope struct {
	Datasource struct {
		ID      string `xml:"id,attr"`
		Name    string `xml:"name,attr"`
		Project struct {
			ID   string `xml:"id,attr"`
			Name string `xml:"name,attr"`
		} `xml:"project"`
	} `xml:"datasource"`
}
