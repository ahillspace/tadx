package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

const pulseSchemaResponseLimit = 16 << 20

type PulseSchema struct {
	Fields []PulseSchemaField
}

type PulseSchemaField struct {
	ID          string
	Name        string
	Caption     string
	Role        string
	DataType    string
	ColumnClass string
	Hidden      bool
	Internal    bool
	Excluded    bool
}

type pulseReadMetadataRequest struct {
	Datasource struct {
		DatasourceLUID string `json:"datasourceLuid"`
	} `json:"datasource"`
}

type pulseDescribeRequest struct {
	DatasourceLUID string `json:"datasourceLuid"`
}

func pulseSchemaReadOnlyRequest(path, contentType string, body []byte) bool {
	if contentType != "application/json" {
		return false
	}
	switch path {
	case "/api/v1/vizql-data-service/read-metadata":
		var request pulseReadMetadataRequest
		if !decodePulseSchemaRequest(body, &request) || strings.TrimSpace(request.Datasource.DatasourceLUID) == "" {
			return false
		}
		canonical, err := json.Marshal(request)
		return err == nil && bytes.Equal(canonical, body)
	case "/api/v1/vizql-data-service/describe-datasource":
		var request pulseDescribeRequest
		if !decodePulseSchemaRequest(body, &request) || strings.TrimSpace(request.DatasourceLUID) == "" {
			return false
		}
		canonical, err := json.Marshal(request)
		return err == nil && bytes.Equal(canonical, body)
	default:
		return false
	}
}

func decodePulseSchemaRequest(body []byte, destination any) bool {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(destination) != nil {
		return false
	}
	var trailing any
	return errors.Is(decoder.Decode(&trailing), io.EOF)
}

// ReadPulseSchema independently reads only datasource field metadata. It never
// requests worksheet data, extract rows, or Pulse metric values.
func (c *Client) ReadPulseSchema(ctx context.Context, datasourceLUID string) (PulseSchema, error) {
	datasourceLUID = strings.TrimSpace(datasourceLUID)
	if c == nil || datasourceLUID == "" || c.SiteID == "" {
		return PulseSchema{}, errors.New("Pulse schema read requires an exact datasource and authenticated site")
	}
	var readRequest pulseReadMetadataRequest
	readRequest.Datasource.DatasourceLUID = datasourceLUID
	metadataBody, err := json.Marshal(readRequest)
	if err != nil {
		return PulseSchema{}, err
	}
	metadata, readErr := c.pulseSchemaJSON(ctx, "/api/v1/vizql-data-service/read-metadata", metadataBody)
	if readErr == nil {
		fields, parseErr := pulseMetadataFields(metadata)
		if parseErr != nil {
			return PulseSchema{}, parseErr
		}
		if len(fields) > 0 {
			return PulseSchema{Fields: fields}, nil
		}
	} else if !pulseSchemaFallbackAllowed(readErr) {
		return PulseSchema{}, readErr
	}
	describeBody, err := json.Marshal(pulseDescribeRequest{DatasourceLUID: datasourceLUID})
	if err != nil {
		return PulseSchema{}, err
	}
	described, describeErr := c.pulseSchemaJSON(ctx, "/api/v1/vizql-data-service/describe-datasource", describeBody)
	if describeErr != nil {
		if pulseSchemaFallbackAllowed(describeErr) {
			return PulseSchema{}, Unsupported("target does not expose the documented datasource field metadata endpoints required for Pulse setup")
		}
		return PulseSchema{}, describeErr
	}
	fields, err := pulseDescribeFields(described)
	if err != nil {
		return PulseSchema{}, err
	}
	if len(fields) == 0 {
		return PulseSchema{}, Inconclusive("target datasource schema is not yet ready")
	}
	return PulseSchema{Fields: fields}, nil
}

func (c *Client) pulseSchemaJSON(ctx context.Context, path string, body []byte) (map[string]any, error) {
	response, err := c.Do(ctx, http.MethodPost, path, body, "application/json")
	if err != nil {
		return nil, err
	}
	if len(response.Body) > pulseSchemaResponseLimit {
		return nil, errors.New("datasource schema response exceeds 16 MiB")
	}
	var document map[string]any
	if err := json.Unmarshal(response.Body, &document); err != nil || document == nil {
		return nil, errors.New("datasource schema response is not a JSON object")
	}
	return document, nil
}

func pulseSchemaFallbackAllowed(err error) bool {
	response, ok := errors.AsType[*HTTPError](err)
	return ok && (response.Status == http.StatusNotFound || response.Status == http.StatusMethodNotAllowed || response.Status == http.StatusNotImplemented)
}

func pulseMetadataFields(document map[string]any) ([]PulseSchemaField, error) {
	rows, ok := document["data"].([]any)
	if !ok {
		return nil, errors.New("read-metadata response omitted data fields")
	}
	fields := make([]PulseSchemaField, 0, len(rows))
	for _, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			return nil, errors.New("read-metadata response contains a malformed field")
		}
		hidden, internal, excluded, err := schemaVisibility(row)
		if err != nil {
			return nil, err
		}
		field := PulseSchemaField{ID: schemaString(row, "fieldName"), Name: schemaString(row, "fieldName"), Caption: schemaString(row, "fieldCaption"), Role: normalizePulseRole(schemaString(row, "fieldRole")), DataType: normalizePulseType(schemaString(row, "dataType")), ColumnClass: normalizePulseType(schemaString(row, "columnClass")), Hidden: hidden, Internal: internal, Excluded: excluded}
		if field.ID == "" || field.DataType == "" {
			return nil, errors.New("read-metadata response omitted provider field identity or type")
		}
		fields = append(fields, field)
	}
	return pulseUniqueFields(fields)
}

func pulseDescribeFields(document map[string]any) ([]PulseSchemaField, error) {
	groups, ok := document["fieldGroups"].([]any)
	if !ok {
		if nested, ok := document["model"].(map[string]any); ok {
			return pulseDescribeFields(nested)
		}
		return nil, errors.New("describe-datasource response omitted field groups")
	}
	var fields []PulseSchemaField
	for _, rawGroup := range groups {
		group, ok := rawGroup.(map[string]any)
		if !ok {
			return nil, errors.New("describe-datasource response contains a malformed field group")
		}
		rows, ok := group["fields"].([]any)
		if !ok {
			return nil, errors.New("describe-datasource response contains a malformed field list")
		}
		for _, raw := range rows {
			row, ok := raw.(map[string]any)
			if !ok {
				return nil, errors.New("describe-datasource response contains a malformed field")
			}
			name := schemaString(row, "name")
			hidden, internal, excluded, err := schemaVisibility(row)
			if err != nil {
				return nil, err
			}
			field := PulseSchemaField{ID: name, Name: name, Caption: schemaString(row, "caption"), Role: normalizePulseRole(schemaString(row, "role")), DataType: normalizePulseType(schemaString(row, "dataType")), ColumnClass: normalizePulseType(schemaString(row, "columnClass")), Hidden: hidden, Internal: internal, Excluded: excluded}
			if field.ID == "" || field.DataType == "" {
				return nil, errors.New("describe-datasource response omitted provider field identity or type")
			}
			fields = append(fields, field)
		}
	}
	return pulseUniqueFields(fields)
}

func pulseUniqueFields(fields []PulseSchemaField) ([]PulseSchemaField, error) {
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		if seen[field.ID] {
			return nil, errors.New("datasource schema repeats a provider field identity")
		}
		seen[field.ID] = true
	}
	return fields, nil
}

func schemaString(row map[string]any, key string) string {
	value, _ := row[key].(string)
	return strings.TrimSpace(value)
}

func schemaVisibility(row map[string]any) (bool, bool, bool, error) {
	hidden, err := optionalSchemaFlag(row, "hidden", "isHidden")
	if err != nil {
		return false, false, false, err
	}
	internal, err := optionalSchemaFlag(row, "isInternal")
	if err != nil {
		return false, false, false, err
	}
	excluded, err := optionalSchemaFlag(row, "excluded")
	if err != nil {
		return false, false, false, err
	}
	return hidden, internal, excluded, nil
}

func optionalSchemaFlag(row map[string]any, keys ...string) (bool, error) {
	var value *bool
	for _, key := range keys {
		raw, exists := row[key]
		if !exists {
			continue
		}
		flag, ok := raw.(bool)
		if !ok {
			return false, errors.New("datasource schema response contains an invalid field eligibility flag")
		}
		if value != nil && *value != flag {
			return false, errors.New("datasource schema response contains conflicting field visibility flags")
		}
		value = &flag
	}
	if value == nil {
		return false, nil
	}
	return *value, nil
}

func normalizePulseRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "measure", "dimension":
		return strings.ToLower(value)
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizePulseType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
