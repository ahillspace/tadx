package fieldcatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/tableau"
)

const maxSchemaResponseBytes = 16 * 1024 * 1024

// Client reads datasource field metadata through VDS with a Metadata API fallback.
type Client struct {
	transport *tableau.Transport
	session   auth.Session
	serverURL string
}

// NewClient creates an authenticated field-catalog client.
func NewClient(transport *tableau.Transport, session auth.Session, serverURL string) *Client {
	return &Client{transport: transport, session: session, serverURL: serverURL}
}

// Read returns one complete normalized datasource schema.
func (c *Client) Read(ctx context.Context, datasourceLUID, datasourceName string) (Schema, error) {
	if c == nil || c.transport == nil || c.session == nil || strings.TrimSpace(c.serverURL) == "" {
		return Schema{}, errors.New("Tableau datasource field client is not configured")
	}
	datasourceLUID = strings.TrimSpace(datasourceLUID)
	datasourceName = strings.TrimSpace(datasourceName)
	if datasourceLUID == "" {
		return Schema{}, errors.New("datasource LUID is required")
	}
	if datasourceName == "" {
		datasourceName = datasourceLUID
	}

	read, readErr := c.readMetadata(ctx, datasourceLUID)
	if readErr == nil && len(read.Data) > 0 {
		result := normalizeReadMetadata(datasourceLUID, datasourceName, read)
		result.RequestID = read.requestID
		return result, nil
	}
	if isAuthenticationError(readErr) {
		return Schema{}, readErr
	}
	described, describeErr := c.describeDatasource(ctx, datasourceLUID)
	if describeErr == nil && described.populated() {
		result := normalizeDescribe(datasourceLUID, datasourceName, described)
		result.RequestID = described.requestID
		return result, nil
	}
	if isAuthenticationError(describeErr) {
		return Schema{}, describeErr
	}

	result, metadataErr := c.readMetadataGraphQL(ctx, datasourceLUID, datasourceName)
	if metadataErr == nil {
		return result, nil
	}
	if readErr != nil || describeErr != nil {
		return Schema{}, fmt.Errorf("datasource field metadata unavailable through VDS read-metadata, VDS describe-datasource, and Metadata API: %w", metadataErr)
	}
	return Schema{}, metadataErr
}

type readMetadataResponse struct {
	Data []struct {
		FieldName          string  `json:"fieldName"`
		FieldCaption       string  `json:"fieldCaption"`
		DataType           string  `json:"dataType"`
		PhysicalType       string  `json:"physicalType"`
		FieldRole          string  `json:"fieldRole"`
		FieldType          string  `json:"fieldType"`
		DefaultAggregation *string `json:"defaultAggregation"`
		ColumnClass        string  `json:"columnClass"`
		Formula            *string `json:"formula"`
		LogicalTableID     *string `json:"logicalTableId"`
		IsHidden           bool    `json:"isHidden"`
		IsInternal         bool    `json:"isInternal"`
	} `json:"data"`
	requestID string
}

func (c *Client) readMetadata(ctx context.Context, luid string) (readMetadataResponse, error) {
	body, _ := json.Marshal(map[string]any{"datasource": map[string]string{"datasourceLuid": luid}})
	response, err := c.post(ctx, "/api/v1/vizql-data-service/read-metadata", "datasource.schema.vds.read", body)
	if err != nil {
		return readMetadataResponse{}, err
	}
	var result readMetadataResponse
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return readMetadataResponse{}, tableau.NewProtocolError("datasource.schema.vds.read", response, fmt.Errorf("decode VDS read-metadata response: %w", err), true)
	}
	result.requestID = response.TableauRequestID
	return result, nil
}

type describeResponse struct {
	DatasourceModel struct {
		LogicalTables []struct {
			LogicalTableID string `json:"logicalTableId"`
			Caption        string `json:"caption"`
		} `json:"logicalTables"`
	} `json:"datasourceModel"`
	FieldGroups []struct {
		LogicalTableID *string `json:"logicalTableId"`
		Fields         []struct {
			Name               string  `json:"name"`
			Caption            *string `json:"caption"`
			DataType           string  `json:"dataType"`
			PhysicalType       *string `json:"physicalType"`
			ColumnClass        string  `json:"columnClass"`
			DefaultAggregation *string `json:"defaultAggregation"`
			Formula            *string `json:"formula"`
			Role               string  `json:"role"`
			IsHidden           bool    `json:"isHidden"`
			IsInternal         bool    `json:"isInternal"`
			LogicalTableID     *string `json:"logicalTableId"`
		} `json:"fields"`
	} `json:"fieldGroups"`
	Model     *json.RawMessage `json:"model"`
	requestID string
}

func (d describeResponse) populated() bool {
	for _, group := range d.FieldGroups {
		if len(group.Fields) > 0 {
			return true
		}
	}
	return false
}

func (c *Client) describeDatasource(ctx context.Context, luid string) (describeResponse, error) {
	body, _ := json.Marshal(map[string]string{"datasourceLuid": luid})
	response, err := c.post(ctx, "/api/v1/vizql-data-service/describe-datasource", "datasource.schema.vds.describe", body)
	if err != nil {
		return describeResponse{}, err
	}
	var result describeResponse
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return describeResponse{}, tableau.NewProtocolError("datasource.schema.vds.describe", response, fmt.Errorf("decode VDS describe-datasource response: %w", err), true)
	}
	if !result.populated() && result.Model != nil {
		var nested describeResponse
		if err := json.Unmarshal(*result.Model, &nested); err != nil {
			return describeResponse{}, tableau.NewProtocolError("datasource.schema.vds.describe", response, fmt.Errorf("decode nested VDS datasource model: %w", err), true)
		}
		result = nested
	}
	result.requestID = response.TableauRequestID
	return result, nil
}

func (c *Client) post(ctx context.Context, path, operation string, body []byte) (tableau.Response, error) {
	header := http.Header{}
	header.Set("X-Tableau-Site-Id", strings.TrimSpace(c.session.SiteLUID()))
	return c.transport.Do(ctx, c.session, tableau.Request{Method: http.MethodPost, ServerURL: c.serverURL, Path: path, Header: header, Body: body, Operation: operation, Accept: "application/json", ContentType: "application/json", MaxResponseBytes: maxSchemaResponseBytes})
}

func isAuthenticationError(err error) bool {
	if err == nil {
		return false
	}
	var carrier interface{ HTTPStatus() int }
	return errors.As(err, &carrier) && (carrier.HTTPStatus() == http.StatusUnauthorized || carrier.HTTPStatus() == http.StatusForbidden)
}

func normalizeReadMetadata(luid, name string, response readMetadataResponse) Schema {
	tables := make(map[string]string)
	fields := make([]rawField, 0, len(response.Data))
	for _, row := range response.Data {
		logicalTableID := value(row.LogicalTableID)
		if logicalTableID != "" {
			tables[logicalTableID] = tableCaption(logicalTableID)
		}
		fields = append(fields, rawField{Name: row.FieldName, Caption: row.FieldCaption, DataType: row.DataType, PhysicalType: row.PhysicalType, ColumnClass: row.ColumnClass, DefaultAggregation: value(row.DefaultAggregation), Formula: value(row.Formula), Role: row.FieldRole, LogicalTableID: logicalTableID, Hidden: row.IsHidden, Internal: row.IsInternal, Provenance: "vds_read_metadata"})
	}
	return normalizeSchema(luid, name, tables, fields, "vds_read_metadata")
}

func normalizeDescribe(luid, name string, response describeResponse) Schema {
	tables := make(map[string]string)
	for _, table := range response.DatasourceModel.LogicalTables {
		tables[table.LogicalTableID] = fallback(strings.TrimSpace(table.Caption), tableCaption(table.LogicalTableID))
	}
	fields := make([]rawField, 0)
	for _, group := range response.FieldGroups {
		groupID := value(group.LogicalTableID)
		for _, field := range group.Fields {
			logicalTableID := value(field.LogicalTableID)
			if logicalTableID == "" {
				logicalTableID = groupID
			}
			fields = append(fields, rawField{Name: field.Name, Caption: value(field.Caption), DataType: field.DataType, PhysicalType: value(field.PhysicalType), ColumnClass: field.ColumnClass, DefaultAggregation: value(field.DefaultAggregation), Formula: value(field.Formula), Role: field.Role, LogicalTableID: logicalTableID, Hidden: field.IsHidden, Internal: field.IsInternal, Provenance: "vds_describe_datasource"})
		}
	}
	return normalizeSchema(luid, name, tables, fields, "vds_describe_datasource")
}

func value(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
