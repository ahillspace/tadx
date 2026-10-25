package harness

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	pathpkg "path"
	"strconv"
	"strings"
	"time"
)

// Client observes Tableau independently of production adapters and output parsers.
type Client struct {
	Server, Version, SiteID, UserID string
	HTTP                            *http.Client
	JournalJob                      func(string) error
	AuthorizeWrite                  func(context.Context, string, string) error
	SessionStale                    bool
	BeforeRefresh                   func(context.Context) error
	JournalLabelIntent              func(string, string, map[string]any) error
	token, patName, patSecret       string
	siteContentURL                  string
	publishObservationBudget        time.Duration
}

func (c *Client) String() string {
	if c == nil {
		return "independent Tableau verifier (nil)"
	}
	return fmt.Sprintf("independent Tableau verifier (authenticated=%t)", c.token != "")
}
func (c *Client) GoString() string { return c.String() }

// HTTPError retains status, never upstream response bodies or credentials.
type HTTPError struct {
	Status       int
	Method, Path string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("independent Tableau observation: %s %s returned HTTP %d", e.Method, e.Path, e.Status)
}

func IsNotFound(err error) bool {
	upstream, ok := errors.AsType[*HTTPError](err)
	return ok && upstream.Status == http.StatusNotFound
}

func NewClient(ctx context.Context, p Profile) (*Client, error) {
	name, secret, err := ResolvePAT(ctx, p)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if p.CAFile != "" {
		pem, err := os.ReadFile(p.CAFile)
		if err != nil {
			return nil, errors.New("read configured Tableau CA failed")
		}
		roots, err := x509.SystemCertPool()
		if err != nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(pem) {
			return nil, errors.New("configured Tableau CA contains no certificate")
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	}
	c := &Client{Server: strings.TrimRight(p.ServerURL, "/"), Version: "3.0", HTTP: &http.Client{Transport: transport, Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, patName: name, patSecret: secret, siteContentURL: p.SiteContentURL}
	c.publishObservationBudget = time.Duration(p.Budgets.ObservationSeconds) * time.Second
	info, err := c.JSON(ctx, http.MethodGet, "/api/2.4/serverinfo", nil)
	if err != nil {
		return nil, fmt.Errorf("negotiate independent REST version: %w", err)
	}
	server, _ := info["serverInfo"].(map[string]any)
	version, _ := server["restApiVersion"].(string)
	if _, err := strconv.ParseFloat(version, 64); err != nil {
		return nil, errors.New("serverinfo omitted REST API version")
	}
	c.Version = version
	if err := c.signIn(ctx); err != nil {
		return nil, err
	}
	if p.ExpectedSiteLUID != "" && p.ExpectedSiteLUID != c.SiteID {
		_ = c.Close(ctx)
		return nil, errors.New("independent sign-in site does not match profile guard")
	}
	return c, nil
}

// MarkSessionStale defers serial PAT refresh until the next independent observation.
// The runner sets this only after a candidate process has exited.
func (c *Client) MarkSessionStale() {
	if c.token != "" && c.patName != "" && c.patSecret != "" {
		c.SessionStale = true
	}
}

func (c *Client) signIn(ctx context.Context) error {
	if c.patName == "" || c.patSecret == "" {
		return errors.New("independent sign-in lacks private PAT credentials")
	}
	payload := map[string]any{"credentials": map[string]any{"personalAccessTokenName": c.patName, "personalAccessTokenSecret": c.patSecret, "site": map[string]any{"contentUrl": c.siteContentURL}}}
	result, err := c.JSON(ctx, http.MethodPost, "/api/"+c.Version+"/auth/signin", payload)
	if err != nil {
		return err
	}
	credentials, _ := result["credentials"].(map[string]any)
	token := stringField(credentials, "token")
	site, _ := credentials["site"].(map[string]any)
	user, _ := credentials["user"].(map[string]any)
	siteID, userID := stringField(site, "id"), stringField(user, "id")
	if token == "" || siteID == "" || userID == "" {
		return errors.New("independent sign-in omitted authoritative session identity")
	}
	if (c.SiteID != "" && siteID != c.SiteID) || (c.UserID != "" && userID != c.UserID) {
		return errors.New("independent session refresh changed authoritative site or user")
	}
	c.token, c.SiteID, c.UserID = token, siteID, userID
	c.SessionStale = false
	return nil
}

func (c *Client) Redact(value string) string {
	for _, secret := range []string{c.token, c.patSecret, c.patName} {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	return value
}
func (c *Client) Close(ctx context.Context) error {
	if c.token == "" {
		return nil
	}
	_, err := c.Do(ctx, http.MethodPost, "/api/"+c.Version+"/auth/signout", nil, "")
	c.token = ""
	return err
}
func (c *Client) REST(suffix string) string {
	return "/api/" + c.Version + "/sites/" + url.PathEscape(c.SiteID) + "/" + strings.TrimLeft(suffix, "/")
}

func (c *Client) Do(ctx context.Context, method, path string, body []byte, contentType string) (Response, error) {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return Response{}, errors.New("independent request path must be server-relative")
	}
	cleanPath, _, _ := strings.Cut(path, "?")
	signin := method == http.MethodPost && cleanPath == "/api/"+c.Version+"/auth/signin"
	signout := method == http.MethodPost && cleanPath == "/api/"+c.Version+"/auth/signout"
	pulseSchemaReadOnly := method == http.MethodPost && pulseSchemaReadOnlyRequest(path, contentType, body)
	consequential := !signin && !signout && consequentialRequest(method, path) && !isLabelQuery(c, method, path, body, contentType) && !pulseSchemaReadOnly
	if consequential {
		decoded, decodeErr := url.PathUnescape(cleanPath)
		if c.SiteID == "" || strings.Contains(path, "#") || decodeErr != nil || strings.ContainsRune(decoded, '\x00') || pathpkg.Clean(cleanPath) != cleanPath || !sameSitePath(c, cleanPath) {
			return Response{}, errors.New("independent mutation path must remain within the exact authenticated site")
		}
	}
	if c.AuthorizeWrite != nil && consequential {
		if err := c.AuthorizeWrite(ctx, method, path); err != nil {
			return Response{}, err
		}
	}
	if c.SessionStale && !signin && !signout {
		if c.BeforeRefresh == nil {
			return Response{}, Inconclusive("serial verifier session refresh requires a local-worker observation guard")
		}
		if err := c.BeforeRefresh(ctx); err != nil {
			return Response{}, err
		}
		if err := c.signIn(ctx); err != nil {
			return Response{}, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Server+path, bytes.NewReader(body))
	if err != nil {
		return Response{}, errors.New("invalid independent HTTP request")
	}
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.token != "" && !signin {
		req.Header.Set("X-Tableau-Auth", c.token)
	}
	if strings.HasPrefix(path, "/api/-/pulse/") || pulseSchemaReadOnly {
		req.Header.Set("X-Tableau-Site-Id", c.SiteID)
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("independent HTTP transport failed: %s", c.Redact(err.Error()))
	}
	defer resp.Body.Close()
	limit := int64(512 << 20)
	if pulseSchemaReadOnly {
		limit = pulseSchemaResponseLimit
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return Response{}, errors.New("read independent HTTP response failed")
	}
	if int64(len(data)) > limit {
		if limit == pulseSchemaResponseLimit {
			return Response{}, errors.New("datasource schema response exceeds 16 MiB")
		}
		return Response{}, errors.New("independent response exceeds 512 MiB")
	}
	result := Response{Status: resp.StatusCode, Header: resp.Header.Clone(), Body: data}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		safePath, _, _ := strings.Cut(path, "?")
		return result, &HTTPError{Status: resp.StatusCode, Method: method, Path: safePath}
	}
	return result, nil
}

func isLabelQuery(c *Client, method, path string, body []byte, contentType string) bool {
	cleanPath, rawQuery, _ := strings.Cut(path, "?")
	if c == nil || strings.TrimSpace(c.SiteID) == "" || method != http.MethodPost || cleanPath != c.REST("labels") || (contentType != "application/xml" && contentType != "text/xml") {
		return false
	}
	if rawQuery != "" {
		query, err := url.ParseQuery(rawQuery)
		if err != nil || len(query) != 1 || len(query["categories"]) != 1 || strings.TrimSpace(query.Get("categories")) == "" {
			return false
		}
	}
	var request struct {
		XMLName     xml.Name `xml:"tsRequest"`
		ContentList struct {
			Content []struct {
				Type string `xml:"contentType,attr"`
				ID   string `xml:"id,attr"`
			} `xml:"content"`
		} `xml:"contentList"`
	}
	if err := xml.Unmarshal(body, &request); err != nil || request.XMLName.Local != "tsRequest" || len(request.ContentList.Content) != 1 {
		return false
	}
	content := request.ContentList.Content[0]
	return strings.TrimSpace(content.ID) != "" && validLabelContentType(content.Type)
}

func validLabelContentType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "database", "table", "column", "datasource", "flow", "virtualconnection", "virtualconnectiontable":
		return true
	default:
		return false
	}
}

func consequentialRequest(method, path string) bool {
	if method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch && method != http.MethodDelete {
		return false
	}
	clean, _, _ := strings.Cut(path, "?")
	if method == http.MethodPost {
		if clean == "/api/metadata/graphql" {
			return false
		}
		if clean == "/api/-/pulse/definitions:batchGet" || clean == "/api/-/pulse/metrics:batchGet" {
			return false
		}
	}
	return true
}

func (c *Client) JSON(ctx context.Context, method, path string, payload any) (map[string]any, error) {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	contentType := "application/json"
	if method == http.MethodPost && strings.HasPrefix(path, "/api/-/pulse/") {
		switch {
		case strings.HasSuffix(path, "/definitions"):
			contentType = "application/vnd.tableau.metricqueryservice.v1.CreateDefinitionRequest+json"
		case strings.Contains(path, "metrics:getOrCreate"):
			contentType = "application/vnd.tableau.metricqueryservice.v1.GetOrCreateMetricRequest+json"
		case strings.Contains(path, "subscriptions:batchCreate"):
			contentType = "application/vnd.tableau.pulse.subscriptionservice.v1.BatchCreateSubscriptionsRequest+json"
		}
	}
	response, err := c.Do(ctx, method, path, body, contentType)
	if err != nil {
		return nil, err
	}
	if len(response.Body) == 0 {
		return map[string]any{}, nil
	}
	var document map[string]any
	if err := json.Unmarshal(response.Body, &document); err != nil {
		return nil, errors.New("independent response is not a JSON object")
	}
	if document == nil {
		return nil, errors.New("independent response JSON object is null")
	}
	return document, nil
}

func (c *Client) Metadata(ctx context.Context, query string, variables map[string]any) (map[string]any, error) {
	doc, err := c.JSON(ctx, http.MethodPost, "/api/metadata/graphql", map[string]any{"query": query, "variables": variables})
	if err != nil {
		return nil, err
	}
	if errs, ok := doc["errors"].([]any); ok && len(errs) > 0 {
		return nil, errors.New("independent Metadata query returned GraphQL errors")
	}
	data, ok := doc["data"].(map[string]any)
	if !ok {
		return nil, errors.New("independent Metadata response omitted data")
	}
	return data, nil
}

func stringField(fields map[string]any, key string) string {
	value, _ := fields[key].(string)
	return value
}
func decodeObject(kind string, fields map[string]any) Object {
	id := stringField(fields, "id")
	name := stringField(fields, "name")
	description := stringField(fields, "description")
	if id == "" {
		id = stringField(fields, "luid")
	}
	if metadata, ok := fields["metadata"].(map[string]any); ok {
		if id == "" {
			id = stringField(metadata, "id")
		}
		if name == "" {
			name = stringField(metadata, "name")
		}
		if description == "" {
			description = stringField(metadata, "description")
		}
	}
	if id == "" && (kind == "labelCategory" || kind == "labelValue" || kind == "label-category" || kind == "label-value") {
		id = stringField(fields, "name")
	}
	project, _ := fields["project"].(map[string]any)
	owner, _ := fields["owner"].(map[string]any)
	parent := stringField(project, "id")
	if kind == "project" {
		parent = stringField(fields, "parentProjectId")
	}
	if kind == "labelValue" || kind == "label-value" {
		parent = stringField(fields, "category")
	}
	if isPulse(kind) {
		if specification, ok := fields["specification"].(map[string]any); ok {
			if datasource, ok := specification["datasource"].(map[string]any); ok {
				parent = stringField(datasource, "id")
			}
		}
	}
	return Object{ID: id, Name: name, Kind: kind, ProjectID: parent, OwnerID: stringField(owner, "id"), Description: description, Fields: fields}
}
