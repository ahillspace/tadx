package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func resourceNames(kind string) (collection, singular string) {
	if strings.Contains(kind, "/") {
		last := pathpkg.Base(kind)
		singular = strings.TrimSuffix(last, "s")
		return kind, singular
	}
	switch kind {
	case "pulse.definition", "pulseDefinition", "definition":
		return "definitions", "definition"
	case "pulse.metric", "pulseMetric", "metric":
		return "metrics", "metric"
	case "pulse.subscription", "pulseSubscription", "subscription":
		return "subscriptions", "subscription"
	case "labelCategory", "label-category":
		return "labelCategories", "labelCategory"
	case "labelValue", "label-value":
		return "labelValues", "labelValue"
	case "task":
		return "tasks/extractRefreshes", "extractRefresh"
	case "datasource":
		return "datasources", "datasource"
	default:
		return kind + "s", kind
	}
}
func isPulse(kind string) bool {
	_, one := resourceNames(kind)
	return one == "definition" || one == "metric" || one == "subscription"
}
func (c *Client) resourcePath(kind string) string {
	many, _ := resourceNames(kind)
	if isPulse(kind) {
		return "/api/-/pulse/" + many
	}
	if kind == "schedule" {
		return "/api/" + c.Version + "/schedules"
	}
	return c.REST(many)
}

func (c *Client) List(ctx context.Context, kind string, query url.Values) ([]Object, error) {
	q := url.Values{}
	for key, values := range query {
		q[key] = append([]string(nil), values...)
	}
	many, one := resourceNames(kind)
	var result []Object
	seen := map[string]bool{}
	for page := 1; page <= 1000; page++ {
		if !isPulse(kind) {
			q.Set("pageNumber", strconv.Itoa(page))
			if q.Get("pageSize") == "" {
				q.Set("pageSize", "100")
			}
		}
		path := c.resourcePath(kind)
		if len(q) > 0 {
			path += "?" + q.Encode()
		}
		doc, err := c.JSON(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}
		var rows []any
		envelope := many
		if strings.Contains(envelope, "/") {
			envelope = pathpkg.Base(envelope)
		}
		if container, ok := doc[envelope].(map[string]any); ok {
			rows, _ = container[one].([]any)
		} else {
			rows, _ = doc[envelope].([]any)
		}
		if list, ok := doc[one+"List"].(map[string]any); ok {
			rows, _ = list[one].([]any)
		}
		// Empty REST collections are represented by an empty object or omitted.
		if rows == nil {
			if raw, ok := doc[one].([]any); ok {
				rows = raw
			}
		}
		for _, row := range rows {
			fields, ok := row.(map[string]any)
			if !ok {
				return nil, errors.New("independent list contains a non-object item")
			}
			obj := decodeObject(kind, fields)
			if obj.ID == "" {
				return nil, errors.New("independent list item omitted authoritative ID")
			}
			if seen[obj.ID] {
				return nil, errors.New("independent pagination repeated a resource identity")
			}
			seen[obj.ID] = true
			result = append(result, obj)
		}
		if isPulse(kind) {
			next := stringField(doc, "next_page_token")
			if next == "" {
				return result, nil
			}
			if next == q.Get("page_token") {
				return nil, errors.New("independent Pulse pagination repeated token")
			}
			q.Set("page_token", next)
			continue
		}
		pagination, _ := doc["pagination"].(map[string]any)
		total, err := numberField(pagination, "totalAvailable")
		if err != nil {
			if kind == "project" || kind == "group" || kind == "user" || kind == "datasource" || kind == "workbook" || kind == "flow" {
				return nil, errors.New("independent inventory omitted authoritative pagination completeness")
			}
			return result, nil
		}
		if total < 0 || total < len(result) {
			return nil, errors.New("independent inventory contradicts its declared total")
		}
		if len(result) >= total {
			return result, nil
		}
		if len(rows) == 0 {
			return nil, errors.New("independent pagination ended before declared total")
		}
	}
	return nil, errors.New("independent list exceeds 1000 pages")
}
func numberField(m map[string]any, key string) (int, error) {
	switch value := m[key].(type) {
	case float64:
		return int(value), nil
	case string:
		return strconv.Atoi(value)
	default:
		return 0, errors.New("missing integer")
	}
}

func (c *Client) Get(ctx context.Context, kind, id string) (Object, error) {
	if kind == "labelCategory" || kind == "label-category" || kind == "group" || kind == "project" {
		items, err := c.List(ctx, kind, nil)
		if err != nil {
			return Object{}, err
		}
		for _, item := range items {
			if item.ID == id {
				return item, nil
			}
		}
		return Object{}, &HTTPError{Status: 404, Method: "GET", Path: c.resourcePath(kind)}
	}
	doc, err := c.JSON(ctx, http.MethodGet, c.resourcePath(kind)+"/"+url.PathEscape(id), nil)
	if err != nil {
		return Object{}, err
	}
	object, err := unwrapObject(kind, doc)
	if err != nil {
		return Object{}, err
	}
	if object.ID != id {
		return Object{}, errors.New("independent exact resource observation returned a different ID")
	}
	return object, nil
}

// QueryLabels reads the complete label list attached to one exact asset.
func (c *Client) QueryLabels(ctx context.Context, contentType, contentID string) ([]Object, error) {
	if !validLabelContentType(contentType) || strings.TrimSpace(contentID) == "" {
		return nil, errors.New("label query requires one exact content type and ID")
	}
	type content struct {
		Type string `xml:"contentType,attr"`
		ID   string `xml:"id,attr"`
	}
	request := struct {
		XMLName xml.Name `xml:"tsRequest"`
		List    struct {
			Content []content `xml:"content"`
		} `xml:"contentList"`
	}{}
	request.List.Content = []content{{Type: contentType, ID: contentID}}
	body, err := xml.Marshal(request)
	if err != nil {
		return nil, errors.New("encode exact label query failed")
	}
	response, err := c.Do(ctx, http.MethodPost, c.REST("labels"), body, "application/xml")
	if err != nil {
		return nil, err
	}
	var document map[string]any
	if err := json.Unmarshal(response.Body, &document); err != nil {
		if explicitEmptyXMLLabelList(response.Body) {
			return []Object{}, nil
		}
		return nil, Inconclusive("label query response is not a complete JSON object or explicit empty XML label list")
	}
	container, exists := document["labelList"]
	if !exists || container == nil {
		return nil, Inconclusive("label query response omitted labelList envelope")
	}
	var raw []any
	switch labels := container.(type) {
	case map[string]any:
		if len(labels) == 0 {
			return []Object{}, nil
		}
		value, exists := labels["label"]
		if !exists || len(labels) != 1 {
			return nil, Inconclusive("label query response omitted complete label rows")
		}
		var ok bool
		raw, ok = value.([]any)
		if !ok {
			return nil, Inconclusive("label query response contains malformed label rows")
		}
	case []any:
		if len(labels) == 0 {
			return []Object{}, nil
		}
		return nil, Inconclusive("label query response omitted complete label rows")
	default:
		return nil, Inconclusive("label query response contains malformed labelList")
	}
	labels := make([]Object, 0, len(raw))
	for _, value := range raw {
		fields, ok := value.(map[string]any)
		if !ok {
			return nil, Inconclusive("label query contains a non-object row")
		}
		object, err := decodeLabel(contentType, contentID, fields)
		if err != nil {
			return nil, Inconclusive("label query row omitted or contradicted its exact tuple")
		}
		labels = append(labels, object)
	}
	return labels, nil
}

func explicitEmptyXMLLabelList(body []byte) bool {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	root, err := nextXMLSignificantToken(decoder)
	start, ok := root.(xml.StartElement)
	if err != nil || !ok || start.Name.Local != "tsResponse" {
		return false
	}
	labelList, err := nextXMLSignificantToken(decoder)
	start, ok = labelList.(xml.StartElement)
	if err != nil || !ok || start.Name.Local != "labelList" {
		return false
	}
	endList, err := nextXMLSignificantToken(decoder)
	end, ok := endList.(xml.EndElement)
	if err != nil || !ok || end.Name.Local != "labelList" {
		return false
	}
	endRoot, err := nextXMLSignificantToken(decoder)
	end, ok = endRoot.(xml.EndElement)
	if err != nil || !ok || end.Name.Local != "tsResponse" {
		return false
	}
	_, err = nextXMLSignificantToken(decoder)
	return err == io.EOF
}

func nextXMLSignificantToken(decoder *xml.Decoder) (xml.Token, error) {
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		if text, ok := token.(xml.CharData); ok && strings.TrimSpace(string(text)) == "" {
			continue
		}
		return token, nil
	}
}

func decodeLabel(contentType, contentID string, fields map[string]any) (Object, error) {
	id := stringField(fields, "id")
	actualContentID := stringField(fields, "contentId")
	actualContentType := stringField(fields, "contentType")
	message := stringField(fields, "message")
	value := stringField(fields, "value")
	category := stringField(fields, "category")
	if id == "" || actualContentID != contentID || !strings.EqualFold(actualContentType, contentType) || message == "" || value == "" || category == "" {
		return Object{}, errors.New("label query row omitted or contradicted its exact asset and label fields")
	}
	for _, key := range []string{"active", "elevated"} {
		if _, ok := fields[key].(bool); !ok {
			return Object{}, fmt.Errorf("label query row omitted boolean %s", key)
		}
	}
	return Object{ID: id, Name: message, Kind: "label", ProjectID: actualContentID, Fields: fields}, nil
}
func unwrapObject(kind string, doc map[string]any) (Object, error) {
	_, one := resourceNames(kind)
	fields, ok := doc[one].(map[string]any)
	if !ok {
		fields = doc
	}
	obj := decodeObject(kind, fields)
	if obj.ID == "" {
		return Object{}, errors.New("independent resource response omitted authoritative ID")
	}
	return obj, nil
}
func (c *Client) Create(ctx context.Context, kind string, fields map[string]any) (Object, error) {
	_, one := resourceNames(kind)
	var payload any = map[string]any{one: fields}
	if isPulse(kind) {
		payload = fields
	}
	method := http.MethodPost
	if kind == "labelValue" || kind == "label-value" {
		method = http.MethodPut
	}
	doc, err := c.JSON(ctx, method, c.resourcePath(kind), payload)
	if err != nil {
		return Object{}, err
	}
	return unwrapObject(kind, doc)
}

type LabelAttachment struct {
	ContentType string
	ContentID   string
	Value       string
	Category    string
	Message     string
	Active      bool
	Elevated    bool
}

// CreateLabel submits one attachment once, then requires an exact returned or freshly queried identity.
func (c *Client) CreateLabel(ctx context.Context, attachment LabelAttachment) (Object, error) {
	if !validLabelContentType(attachment.ContentType) || attachment.ContentID == "" || attachment.Value == "" || attachment.Category == "" || attachment.Message == "" {
		return Object{}, errors.New("label attachment requires a complete exact request tuple")
	}
	expected := map[string]any{
		"contentType": attachment.ContentType, "contentId": attachment.ContentID, "value": attachment.Value,
		"category": attachment.Category, "message": attachment.Message, "active": attachment.Active, "elevated": attachment.Elevated,
	}
	if c.JournalLabelIntent != nil {
		if err := c.JournalLabelIntent(attachment.Message, attachment.ContentID, expected); err != nil {
			return Object{}, err
		}
	}
	payload := map[string]any{
		"contentList": map[string]any{"content": []any{map[string]any{"contentType": attachment.ContentType, "id": attachment.ContentID}}},
		"label":       map[string]any{"value": attachment.Value, "message": attachment.Message, "active": attachment.Active, "elevated": attachment.Elevated},
	}
	document, err := c.JSON(ctx, http.MethodPut, c.REST("labels"), payload)
	if err != nil {
		return Object{}, err
	}
	rows, hasRows := labelResponseRows(document)
	if hasRows {
		var matches []Object
		for _, row := range rows {
			object, err := decodeLabel(attachment.ContentType, attachment.ContentID, row)
			if err != nil {
				return Object{}, err
			}
			if labelMatches(object, expected) {
				matches = append(matches, object)
			}
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
		if len(matches) > 1 {
			return Object{}, Inconclusive("label creation response contains multiple exact attachment identities")
		}
	}
	labels, err := c.QueryLabels(ctx, attachment.ContentType, attachment.ContentID)
	if err != nil {
		return Object{}, fmt.Errorf("observe accepted label attachment: %w", err)
	}
	var matches []Object
	for _, label := range labels {
		if labelMatches(label, expected) {
			matches = append(matches, label)
		}
	}
	if len(matches) != 1 {
		return Object{}, Inconclusive("label attachment did not resolve to exactly one complete tuple with an authoritative ID")
	}
	return matches[0], nil
}

func labelResponseRows(document map[string]any) ([]map[string]any, bool) {
	container, ok := document["labelList"].(map[string]any)
	if !ok {
		return nil, false
	}
	raw, ok := container["label"].([]any)
	if !ok {
		return nil, false
	}
	rows := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		row, ok := item.(map[string]any)
		if !ok {
			return nil, true
		}
		rows = append(rows, row)
	}
	return rows, true
}

func labelMatches(object Object, expected map[string]any) bool {
	for _, key := range []string{"contentId", "value", "category", "message", "active", "elevated"} {
		if object.Fields[key] != expected[key] {
			return false
		}
	}
	contentType, _ := object.Fields["contentType"].(string)
	wantedType, _ := expected["contentType"].(string)
	return strings.EqualFold(contentType, wantedType)
}
func (c *Client) Update(ctx context.Context, kind, id string, fields map[string]any) (Object, error) {
	_, one := resourceNames(kind)
	var payload any = map[string]any{one: fields}
	method := http.MethodPut
	if isPulse(kind) {
		payload = fields
		method = http.MethodPatch
	}
	doc, err := c.JSON(ctx, method, c.resourcePath(kind)+"/"+url.PathEscape(id), payload)
	if err != nil {
		return Object{}, err
	}
	if len(doc) == 0 {
		return c.Get(ctx, kind, id)
	}
	return unwrapObject(kind, doc)
}
func (c *Client) Delete(ctx context.Context, kind, id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("independent delete requires exact resource ID")
	}
	_, err := c.Do(ctx, http.MethodDelete, c.resourcePath(kind)+"/"+url.PathEscape(id), nil, "")
	return err
}

func (c *Client) Download(ctx context.Context, kind, id string) (Native, error) {
	response, err := c.Do(ctx, http.MethodGet, c.resourcePath(kind)+"/"+url.PathEscape(id)+"/content", nil, "")
	if err != nil {
		return Native{}, err
	}
	disposition := response.Header.Get("Content-Disposition")
	if strings.HasPrefix(disposition, "name=") {
		disposition = "attachment; " + disposition
	}
	_, params, _ := mime.ParseMediaType(disposition)
	filename := params["filename"]
	if filename == "" {
		return Native{}, errors.New("independent native download omitted filename")
	}
	if decoded, err := url.QueryUnescape(filename); err == nil {
		filename = decoded
	}
	if strings.ContainsAny(filename, "/\\\x00\r\n") || filepath.Base(filename) != filename {
		return Native{}, errors.New("independent download filename contains path separators")
	}
	native := Native{Bytes: response.Body, Filename: filename}
	if _, err := ValidateNative(kind, native); err != nil {
		return Native{}, err
	}
	return native, nil
}

// Publish uses Tableau's documented native multipart contract, independently of CLI publishing.
func (c *Client) Publish(ctx context.Context, kind, path, project, name string, overwrite bool) (Object, error) {
	if kind != "workbook" && kind != "datasource" && kind != "flow" {
		return Object{}, errors.New("unsupported native publish kind")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Object{}, err
	}
	if _, err := ValidateNative(kind, Native{Bytes: data, Filename: filepath.Base(path)}); err != nil {
		return Object{}, err
	}
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	request := struct {
		XMLName xml.Name `xml:"tsRequest"`
		Content any      `xml:",any"`
	}{Content: struct {
		XMLName xml.Name
		Name    string `xml:"name,attr"`
		Project struct {
			ID string `xml:"id,attr"`
		} `xml:"project"`
	}{XMLName: xml.Name{Local: kind}, Name: name, Project: struct {
		ID string `xml:"id,attr"`
	}{ID: project}}}
	xmlBody, err := xml.Marshal(request)
	if err != nil {
		return Object{}, err
	}
	part, err := writer.CreatePart(textproto.MIMEHeader{"Content-Disposition": {`name="request_payload"`}, "Content-Type": {"text/xml"}})
	if err != nil {
		return Object{}, err
	}
	if _, err = part.Write(xmlBody); err != nil {
		return Object{}, err
	}
	filename := strings.ReplaceAll(filepath.Base(path), `"`, "")
	part, err = writer.CreatePart(textproto.MIMEHeader{"Content-Disposition": {fmt.Sprintf(`name="tableau_%s"; filename="%s"`, kind, filename)}, "Content-Type": {"application/octet-stream"}})
	if err != nil {
		return Object{}, err
	}
	if _, err = part.Write(data); err != nil {
		return Object{}, err
	}
	if err = writer.Close(); err != nil {
		return Object{}, err
	}
	query := url.Values{"overwrite": {strconv.FormatBool(overwrite)}, kind + "Type": {strings.TrimPrefix(filepath.Ext(path), ".")}, "asJob": {"true"}}
	response, err := c.Do(ctx, http.MethodPost, c.resourcePath(kind)+"?"+query.Encode(), buffer.Bytes(), "multipart/mixed; boundary="+writer.Boundary())
	if err != nil {
		return Object{}, err
	}
	var doc map[string]any
	if json.Unmarshal(response.Body, &doc) != nil {
		return Object{}, errors.New("independent publish returned invalid JSON")
	}
	if job, ok := doc["job"].(map[string]any); ok {
		jobID := stringField(job, "id")
		if jobID == "" {
			return Object{}, errors.New("independent publish job omitted ID")
		}
		if c.JournalJob != nil {
			if err := c.JournalJob(jobID); err != nil {
				return Object{}, err
			}
		}
		done, err := c.WaitJob(ctx, jobID)
		if err != nil {
			return Object{}, err
		}
		if child, ok := done.Fields[kind].(map[string]any); ok {
			object := decodeObject(kind, child)
			if object.ID != "" {
				return object, nil
			}
		}
		return c.reconcilePublished(ctx, kind, project, name)
	}
	if child, ok := doc[kind].(map[string]any); ok {
		object := decodeObject(kind, child)
		if object.ID != "" {
			return object, nil
		}
	}
	return Object{}, errors.New("independent publish response omitted its native resource envelope")
}

func (c *Client) reconcilePublished(ctx context.Context, kind, project, name string) (Object, error) {
	budget := c.publishObservationBudget
	if budget <= 0 {
		budget = 5 * time.Minute
	}
	bounded, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	delay := 100 * time.Millisecond
	for {
		items, err := c.List(bounded, kind, nil)
		if err != nil {
			return Object{}, fmt.Errorf("observe completed publish inventory: %w", err)
		}
		var matches []Object
		for _, item := range items {
			if item.Name == name && item.ProjectID == project {
				matches = append(matches, item)
			}
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
		if len(matches) > 1 {
			return Object{}, Inconclusive("completed publish has multiple exact name and project matches; retain its accepted job and pending ownership intent")
		}
		timer := time.NewTimer(delay)
		select {
		case <-bounded.Done():
			timer.Stop()
			return Object{}, Inconclusive("completed publish resource is not independently visible; retain its accepted job and pending ownership intent")
		case <-timer.C:
		}
		delay = min(delay*2, 2*time.Second)
	}
}

// WaitJob observes only the submitted job ID and rejects unsuccessful terminal jobs.
func (c *Client) WaitJob(ctx context.Context, id string) (Object, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	for {
		job, err := c.Get(ctx, "job", id)
		if err != nil {
			return Object{}, err
		}
		if job.ID != id {
			return Object{}, errors.New("independent job observation returned another job")
		}
		code := stringField(job.Fields, "finishCode")
		if number, ok := job.Fields["finishCode"].(float64); ok {
			code = strconv.Itoa(int(number))
		}
		progress, progressErr := numberField(job.Fields, "progress")
		complete := stringField(job.Fields, "completedAt") != "" || (progressErr == nil && progress == 100)
		bridge := strings.Contains(strings.ToLower(stringField(job.Fields, "type")), "bridge")
		if bridge && code == "0" {
			complete = false
		}
		if complete && code != "" && code != "-1" {
			if code != "0" && !(bridge && code == "3") {
				return Object{}, fmt.Errorf("independent job finished unsuccessfully: code %s", code)
			}
			return job, nil
		}
		select {
		case <-ctx.Done():
			return Object{}, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
