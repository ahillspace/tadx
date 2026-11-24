package datasource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/ahillspace/tadx/internal/tableau"
	"github.com/ahillspace/tadx/internal/value"
)

func (c *Client) checkTDSVersion() error {
	if err := c.validate(); err != nil {
		return err
	}
	e := c.TDSPrerequisites("")
	if e.RESTStatus != "supported" {
		return fmt.Errorf("TDS Edit requires Tableau REST API 3.30 or later (negotiated %q)", e.RESTVersion)
	}
	return nil
}

// doTDS binds the authenticated site and negotiated version. The caller supplies only
// fixed route components and an exact datasource/work-copy UUID.
func (c *Client) doTDS(ctx context.Context, kind, id, suffix string, req tableau.Request) (tableau.Response, error) {
	if err := c.checkTDSVersion(); err != nil {
		return tableau.Response{}, err
	}
	if !value.ValidTDSLUID(id) {
		return tableau.Response{}, errors.New("TDS Edit requires an exact datasource or work-copy UUID")
	}
	version := c.transport.APIVersion()
	req.ServerURL = c.serverURL
	req.Path = "/api/" + version + "/tds/sites/" + url.PathEscape(c.session.SiteLUID()) + "/" + kind + "/" + id + suffix
	req.Accept = "application/json"
	if len(req.Body) > 0 {
		req.ContentType = "application/json"
	}
	req.StructuredFailures = true
	req.MaxResponseBytes = maxListResponseBytes
	return c.transport.Do(ctx, c.session, req)
}

func (c *Client) CreateWorkCopy(ctx context.Context, sourceID string) (result value.TDSReceipt, resultErr error) {
	const op = "datasource.checkout"
	defer func() { c.qualifyTDSReceipt(op, &result, resultErr) }()
	response, err := c.doTDS(ctx, "datasource", sourceID, "/workcopy", tableau.Request{Method: http.MethodPost, Operation: op})
	if err != nil {
		return failedReceipt(err), err
	}
	return copyIdentity(op, response, true)
}

// PublishWorkCopy sends only the documented datasource properties. Destination
// admission and verification belong to the action.
func (c *Client) PublishWorkCopy(ctx context.Context, copyID, name string, description *string) (result value.TDSReceipt, resultErr error) {
	const op = "datasource.save.as-new"
	defer func() { c.qualifyTDSReceipt(op, &result, resultErr) }()
	if strings.TrimSpace(name) == "" {
		return value.TDSReceipt{Status: "not_attempted"}, errors.New("publishing a work copy requires a nonblank datasource name")
	}
	body := struct {
		Datasource struct {
			Name        string  `json:"name"`
			Description *string `json:"description,omitempty"`
		} `json:"datasource"`
	}{}
	body.Datasource.Name, body.Datasource.Description = name, description
	payload, err := json.Marshal(body)
	if err != nil {
		return value.TDSReceipt{}, err
	}
	response, err := c.doTDS(ctx, "workcopy", copyID, "/publish", tableau.Request{Method: http.MethodPost, Operation: op, Body: payload})
	if err != nil {
		return failedReceipt(err), err
	}
	return copyIdentity(op, response, false)
}

func copyIdentity(op string, response tableau.Response, checkout bool) (value.TDSReceipt, error) {
	got := value.TDSReceipt{Status: "unknown", TableauRequestID: response.TableauRequestID}
	var body struct {
		Copy struct {
			WorkCopyLUID   string `json:"workCopyLuid"`
			DatasourceLUID string `json:"datasourceLuid"`
		} `json:"tdsWorkCopy"`
	}
	err := json.Unmarshal(response.Body, &body)
	if err == nil {
		if value.ValidTDSLUID(body.Copy.WorkCopyLUID) && !response.ContainsCredential(body.Copy.WorkCopyLUID) {
			got.WorkCopyLUID = body.Copy.WorkCopyLUID
		}
		if value.ValidTDSLUID(body.Copy.DatasourceLUID) && !response.ContainsCredential(body.Copy.DatasourceLUID) {
			got.DatasourceLUID = body.Copy.DatasourceLUID
		}
	}
	valid := got.DatasourceLUID != "" && body.Copy.WorkCopyLUID == ""
	if checkout {
		valid = got.WorkCopyLUID != "" && body.Copy.DatasourceLUID == ""
	}
	if response.StatusCode != http.StatusCreated || !valid {
		return got, tableau.NewProtocolError(op, response, errors.New("expected HTTP 201 and one authoritative identifier in tdsWorkCopy"), false)
	}
	got.Status = "acknowledged"
	return got, nil
}

func failedReceipt(err error) value.TDSReceipt {
	if !tableau.SubmissionAttempted(err) {
		return value.TDSReceipt{Status: "not_attempted"}
	}
	result := value.TDSReceipt{Status: "unknown", TableauRequestID: tableau.RequestID(err)}
	if e, ok := errors.AsType[*tableau.UpstreamError](err); ok && e.Code != "" && !tableau.MutationUncertain(err) {
		result.Status = "rejected"
	}
	return result
}

func tdsElementType(kind string) bool {
	switch kind {
	case "Column", "FieldDescription", "TableDescription":
		return true
	}
	return false
}

// ReadTDSElements uses named reads for verification. A missing named element
// has its own Tableau sub-code; a missing datasource or work copy is an error.
func (c *Client) ReadTDSElements(ctx context.Context, targetKind, id, elementType, name string) (result value.TDSRead, resultErr error) {
	defer func() {
		result.Eligibility = c.observedTDS("datasource.tds.read."+targetKind, resultErr == nil, nil, resultErr)
	}()
	const op = "datasource.tds.read"
	if targetKind != "workcopy" && targetKind != "datasource" {
		return value.TDSRead{}, errors.New("expected a workcopy or datasource TDS target")
	}
	if !tdsElementType(elementType) {
		return value.TDSRead{}, errors.New("unknown TDS element type")
	}
	query := url.Values{"elementType": {elementType}}
	if name != "" {
		query.Set("name", name)
	}
	response, err := c.doTDS(ctx, targetKind, id, "", tableau.Request{Method: http.MethodGet, Operation: op, Query: query})
	if err != nil {
		read := value.TDSRead{TableauRequestID: tableau.RequestID(err)}
		if e, ok := errors.AsType[*tableau.UpstreamError](err); ok && name != "" && e.StatusCode == 404 && e.Code == "404065" {
			read.Absent = true
			return read, nil
		}
		return read, err
	}
	read := value.TDSRead{TableauRequestID: response.TableauRequestID}
	var body struct {
		Elements []value.TDSElement `json:"tdsElementList"`
	}
	if response.StatusCode != 200 || json.Unmarshal(response.Body, &body) != nil || body.Elements == nil {
		return read, tableau.NewProtocolError(op, response, errors.New("expected HTTP 200 and a tdsElementList array"), true)
	}
	if name != "" && (len(body.Elements) != 1 || body.Elements[0].Name != name) {
		return read, tableau.NewProtocolError(op, response, errors.New("named read did not return exactly the selected TDS element"), true)
	}
	seen := make(map[string]bool, len(body.Elements))
	incomplete := false
	for _, el := range body.Elements {
		if protectedElement(response, el) {
			return value.TDSRead{TableauRequestID: response.TableauRequestID}, tableau.NewProtocolError(op, response, errors.New("TDS read contains credential-bearing metadata"), false)
		}
		if el.Name == "" || seen[el.Name] || el.ElementType != "" && el.ElementType != elementType {
			return value.TDSRead{TableauRequestID: response.TableauRequestID}, tableau.NewProtocolError(op, response, errors.New("TDS read contains missing, conflicting or repeated element identity"), true)
		}
		seen[el.Name] = true
		el.ElementType = elementType // The documented converter may omit the type.
		if el.Error != nil {
			el.Error.Message = response.Diagnostic(el.Error.Message)
			el.Column = nil
			el.Description = nil
			incomplete = true
		} else if elementType == "Column" && (el.Column == nil || el.Description != nil) || elementType != "Column" && (el.Description == nil || el.Column != nil) {
			return value.TDSRead{TableauRequestID: response.TableauRequestID}, tableau.NewProtocolError(op, response, errors.New("TDS read returned an incomplete or conflicting element payload"), true)
		}
		read.Elements = append(read.Elements, el)
	}
	if incomplete {
		return read, tableau.NewProtocolError(op, response, errors.New("Tableau reported one or more failed TDS read records"), false)
	}
	return read, nil
}

func protectedElement(response tableau.Response, el value.TDSElement) bool {
	if response.ContainsCredential(el.Name) {
		return true
	}
	contains := func(s *string) bool { return s != nil && response.ContainsCredential(*s) }
	if contains(el.Description) {
		return true
	}
	return el.Column != nil && (contains(el.Column.Caption) || contains(el.Column.Role) || contains(el.Column.Type))
}

// EditTDSElement dispatches one property at a time. An edit response contains
// only failures; a successful acknowledgement still needs exact readback.
func (c *Client) EditTDSElement(ctx context.Context, copyID, verb string, element value.TDSElement) (result value.TDSReceipt, resultErr error) {
	op := "datasource.tds." + verb
	defer func() {
		c.qualifyTDSReceipt(op, &result, resultErr)
	}()
	if !tdsElementType(element.ElementType) || element.Name == "" || element.Error != nil {
		return value.TDSReceipt{}, errors.New("invalid TDS edit element")
	}
	method := http.MethodPut
	success := http.StatusOK
	switch verb {
	case "add":
		method, success = http.MethodPost, http.StatusCreated
		if element.ElementType == "Column" {
			return value.TDSReceipt{}, errors.New("Tableau does not add TDS columns")
		}
	case "update":
	case "delete":
		method = http.MethodDelete
		if element.Description != nil || element.Column != nil {
			return value.TDSReceipt{}, errors.New("TDS element deletion takes only type and name")
		}
	default:
		return value.TDSReceipt{}, errors.New("unknown TDS edit verb")
	}
	if method != http.MethodDelete {
		if element.ElementType == "Column" {
			if element.Description != nil || element.Column == nil || !singleColumnProperty(*element.Column) {
				return value.TDSReceipt{}, errors.New("TDS column update requires exactly one caption, role or type")
			}
		} else if element.Column != nil || element.Description == nil {
			return value.TDSReceipt{}, errors.New("TDS description edits require an explicit description")
		}
	}
	body, err := json.Marshal(struct {
		Elements []value.TDSElement `json:"tdsElementList"`
	}{[]value.TDSElement{element}})
	if err != nil {
		return value.TDSReceipt{}, err
	}
	response, err := c.doTDS(ctx, "workcopy", copyID, "", tableau.Request{Method: method, Operation: op, Body: body})
	if err != nil {
		got := failedReceipt(err)
		var body struct {
			Failures []struct {
				Type   string           `json:"elementType"`
				Name   string           `json:"name"`
				Status *value.TDSStatus `json:"status"`
			} `json:"tdsElementResult"`
		}
		if json.Unmarshal(response.Body, &body) == nil && body.Failures != nil {
			got.Status = "unknown"
			if len(body.Failures) == 1 {
				f := body.Failures[0]
				if f.Type == element.ElementType && f.Name == element.Name && f.Status != nil && f.Status.Code == response.StatusCode {
					f.Status.Message = response.Diagnostic(f.Status.Message)
					got.Failure = f.Status
					// Server-side or intermediary failures do not establish that no
					// state changed. Keep the per-element status for exact recovery.
					if f.Status.Code < http.StatusInternalServerError {
						got.Status = "rejected"
					}
				}
			}
		}
		return got, err
	}
	got := value.TDSReceipt{Status: "unknown", TableauRequestID: response.TableauRequestID}
	var envelope map[string]json.RawMessage
	if response.StatusCode != success || json.Unmarshal(response.Body, &envelope) != nil || envelope == nil || len(envelope) != 0 {
		return got, tableau.NewProtocolError(op, response, errors.New("unexpected TDS edit acknowledgement"), false)
	}
	got.Status = "acknowledged"
	return got, nil
}

func singleColumnProperty(column value.TDSColumn) bool {
	fields := 0
	for _, val := range []*string{column.Caption, column.Role, column.Type} {
		if val != nil {
			fields++
		}
	}
	return fields == 1 && (column.Role == nil || (*column.Role != "" && *column.Role != "FIELD_ROLE_UNKNOWN" && *column.Role != "UNRECOGNIZED")) && (column.Type == nil || (*column.Type != "" && *column.Type != "FIELD_TYPE_UNKNOWN" && *column.Type != "UNRECOGNIZED"))
}
