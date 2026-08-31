// Package metadata implements the Tableau Metadata API client family.
package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/tableau"
)

const (
	defaultPageSize  = 100
	maxResponseBytes = 4 * 1024 * 1024
	queryOperation   = "metadata.query"
)

const workbookPublishedDatasourcesQuery = `query WorkbookDirectPublishedDatasources(
  $workbookLuid: String!
  $embeddedAfter: String
  $pageSize: Int!
) {
  workbooksConnection(
    first: 2
    filter: {luid: $workbookLuid}
    permissionMode: OBFUSCATE_RESULTS
  ) {
    totalCount
    nodes {
      luid
      embeddedDatasourcesConnection(
        first: $pageSize
        after: $embeddedAfter
        orderBy: {field: ID, direction: ASC}
        permissionMode: OBFUSCATE_RESULTS
      ) {
        totalCount
        pageInfo {
          hasNextPage
          endCursor
        }
        nodes {
          id
          name
          parentPublishedDatasourcesConnection(
            first: $pageSize
            orderBy: {field: ID, direction: ASC}
            permissionMode: OBFUSCATE_RESULTS
          ) {
            totalCount
            pageInfo {
              hasNextPage
              endCursor
            }
            nodes {
              luid
              name
            }
          }
        }
      }
    }
  }
}`

const embeddedPublishedDatasourcesQuery = `query EmbeddedDatasourceParentPublishedDatasources(
  $embeddedDatasourceId: ID!
  $parentAfter: String
  $pageSize: Int!
) {
  embeddedDatasourcesConnection(
    first: 2
    filter: {id: $embeddedDatasourceId}
    permissionMode: OBFUSCATE_RESULTS
  ) {
    totalCount
    nodes {
      id
      name
      parentPublishedDatasourcesConnection(
        first: $pageSize
        after: $parentAfter
        orderBy: {field: ID, direction: ASC}
        permissionMode: OBFUSCATE_RESULTS
      ) {
        totalCount
        pageInfo {
          hasNextPage
          endCursor
        }
        nodes {
          luid
          name
        }
      }
    }
  }
}`

// PublishedDatasource is one direct workbook dependency identified by its REST LUID.
type PublishedDatasource struct {
	LUID     string
	Name     string
	SiteLUID string
}

// Client queries Tableau metadata through the shared authenticated transport.
type Client struct {
	transport *tableau.Transport
	session   auth.Session
	serverURL string
}

// NewClient creates an authenticated Metadata API client.
func NewClient(transport *tableau.Transport, session auth.Session, serverURL string) *Client {
	return &Client{transport: transport, session: session, serverURL: serverURL}
}

// DirectPublishedDatasources returns every directly referenced published datasource.
func (c *Client) DirectPublishedDatasources(ctx context.Context, workbookLUID string) ([]PublishedDatasource, error) {
	if c == nil || c.transport == nil || c.session == nil || strings.TrimSpace(c.serverURL) == "" {
		return nil, errors.New("Tableau metadata client is not configured")
	}
	workbookLUID = strings.TrimSpace(workbookLUID)
	if workbookLUID == "" {
		return nil, errors.New("workbook LUID is required")
	}
	siteLUID := strings.TrimSpace(c.session.SiteLUID())
	if siteLUID == "" {
		return nil, errors.New("Tableau metadata client session omitted the source site LUID")
	}

	byLUID := make(map[string]PublishedDatasource)
	seenEmbedded := make(map[string]struct{})
	var embeddedAfter any
	embeddedTotal := -1
	embeddedCount := 0
	seenEmbeddedCursors := make(map[string]struct{})

	for {
		response, envelope, err := c.query(ctx, workbookPublishedDatasourcesQuery, map[string]any{
			"workbookLuid":  workbookLUID,
			"embeddedAfter": embeddedAfter,
			"pageSize":      defaultPageSize,
		})
		if err != nil {
			return nil, err
		}
		connection := envelope.Data.WorkbooksConnection
		if connection == nil || connection.TotalCount == nil || connection.Nodes == nil || *connection.TotalCount != 1 || len(*connection.Nodes) != 1 {
			return nil, protocolError(response, "expected exactly one workbook for REST LUID %q", workbookLUID)
		}
		workbook := (*connection.Nodes)[0]
		if workbook.LUID != workbookLUID {
			return nil, protocolError(response, "workbook query returned REST LUID %q, expected %q", workbook.LUID, workbookLUID)
		}
		if workbook.EmbeddedDatasourcesConnection == nil {
			return nil, protocolError(response, "workbook %q omitted embeddedDatasourcesConnection", workbookLUID)
		}
		embedded := workbook.EmbeddedDatasourcesConnection
		if embedded.TotalCount == nil {
			return nil, protocolError(response, "workbook %q embedded datasource connection omitted totalCount", workbookLUID)
		}
		if *embedded.TotalCount < 0 {
			return nil, protocolError(response, "workbook %q returned a negative embedded datasource total", workbookLUID)
		}
		if embedded.PageInfo == nil || embedded.Nodes == nil {
			return nil, protocolError(response, "workbook %q returned an incomplete embedded datasource connection", workbookLUID)
		}
		if embeddedTotal < 0 {
			embeddedTotal = *embedded.TotalCount
		} else if *embedded.TotalCount != embeddedTotal {
			return nil, protocolError(response, "workbook %q changed embedded datasource total from %d to %d while paging", workbookLUID, embeddedTotal, *embedded.TotalCount)
		}
		for _, datasource := range *embedded.Nodes {
			id := strings.TrimSpace(datasource.ID)
			if id == "" {
				return nil, protocolError(response, "workbook %q returned an embedded datasource without a Metadata API ID", workbookLUID)
			}
			if _, exists := seenEmbedded[id]; exists {
				return nil, protocolError(response, "workbook %q returned duplicate embedded datasource ID %q", workbookLUID, id)
			}
			seenEmbedded[id] = struct{}{}
			embeddedCount++
			if datasource.ParentPublishedDatasourcesConnection == nil {
				return nil, protocolError(response, "embedded datasource %q omitted parentPublishedDatasourcesConnection", id)
			}
			if err := c.collectParentConnection(ctx, response, id, datasource.ParentPublishedDatasourcesConnection, siteLUID, byLUID); err != nil {
				return nil, err
			}
		}
		if embeddedCount > embeddedTotal {
			return nil, protocolError(response, "workbook %q returned more embedded datasources than totalCount", workbookLUID)
		}
		if embedded.PageInfo.HasNextPage == nil {
			return nil, protocolError(response, "workbook %q embedded datasource connection omitted hasNextPage", workbookLUID)
		}
		if !*embedded.PageInfo.HasNextPage {
			if embeddedCount != embeddedTotal {
				return nil, protocolError(response, "workbook %q embedded datasource connection ended at %d of %d nodes", workbookLUID, embeddedCount, embeddedTotal)
			}
			break
		}
		cursor := strings.TrimSpace(embedded.PageInfo.EndCursor)
		if cursor == "" {
			return nil, protocolError(response, "workbook %q embedded datasource connection has another page without an end cursor", workbookLUID)
		}
		if _, exists := seenEmbeddedCursors[cursor]; exists {
			return nil, protocolError(response, "workbook %q repeated embedded datasource cursor %q", workbookLUID, cursor)
		}
		seenEmbeddedCursors[cursor] = struct{}{}
		embeddedAfter = cursor
	}

	result := make([]PublishedDatasource, 0, len(byLUID))
	for _, datasource := range byLUID {
		result = append(result, datasource)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].SiteLUID != result[j].SiteLUID {
			return result[i].SiteLUID < result[j].SiteLUID
		}
		if result[i].LUID != result[j].LUID {
			return result[i].LUID < result[j].LUID
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func (c *Client) collectParentConnection(ctx context.Context, response tableau.Response, embeddedID string, connection *publishedDatasourceConnection, siteLUID string, byLUID map[string]PublishedDatasource) error {
	currentResponse := response
	if connection.TotalCount == nil {
		return protocolError(currentResponse, "embedded datasource %q parent connection omitted totalCount", embeddedID)
	}
	total := *connection.TotalCount
	if total < 0 {
		return protocolError(currentResponse, "embedded datasource %q returned a negative parent published datasource total", embeddedID)
	}
	if connection.PageInfo == nil || connection.Nodes == nil {
		return protocolError(currentResponse, "embedded datasource %q returned an incomplete parent published datasource connection", embeddedID)
	}
	count := 0
	seenCursors := make(map[string]struct{})
	seenParentLUIDs := make(map[string]struct{})
	for {
		for _, datasource := range *connection.Nodes {
			luid := strings.TrimSpace(datasource.LUID)
			if luid == "" {
				return protocolError(currentResponse, "embedded datasource %q returned a parent published datasource with a blank REST LUID", embeddedID)
			}
			if _, exists := seenParentLUIDs[luid]; exists {
				return protocolError(currentResponse, "embedded datasource %q returned duplicate parent published datasource REST LUID %q", embeddedID, luid)
			}
			seenParentLUIDs[luid] = struct{}{}
			name := strings.TrimSpace(datasource.Name)
			if existing, exists := byLUID[luid]; exists {
				if existing.Name != name {
					return protocolError(currentResponse, "published datasource REST LUID %q has conflicting labels %q and %q", luid, existing.Name, name)
				}
			} else {
				byLUID[luid] = PublishedDatasource{LUID: luid, Name: name, SiteLUID: siteLUID}
			}
			count++
		}
		if count > total {
			return protocolError(currentResponse, "embedded datasource %q returned more parent published datasources than totalCount", embeddedID)
		}
		if connection.PageInfo.HasNextPage == nil {
			return protocolError(currentResponse, "embedded datasource %q parent connection omitted hasNextPage", embeddedID)
		}
		if !*connection.PageInfo.HasNextPage {
			if count != total {
				return protocolError(currentResponse, "embedded datasource %q parent connection ended at %d of %d nodes", embeddedID, count, total)
			}
			return nil
		}
		cursor := strings.TrimSpace(connection.PageInfo.EndCursor)
		if cursor == "" {
			return protocolError(currentResponse, "embedded datasource %q parent connection has another page without an end cursor", embeddedID)
		}
		if _, exists := seenCursors[cursor]; exists {
			return protocolError(currentResponse, "embedded datasource %q repeated parent cursor %q", embeddedID, cursor)
		}
		seenCursors[cursor] = struct{}{}

		var envelope graphqlEnvelope
		nextResponse, envelope, err := c.query(ctx, embeddedPublishedDatasourcesQuery, map[string]any{
			"embeddedDatasourceId": embeddedID,
			"parentAfter":          cursor,
			"pageSize":             defaultPageSize,
		})
		if err != nil {
			return err
		}
		currentResponse = nextResponse
		root := envelope.Data.EmbeddedDatasourcesConnection
		if root == nil || root.Nodes == nil || root.TotalCount == nil || *root.TotalCount != 1 || len(*root.Nodes) != 1 || strings.TrimSpace((*root.Nodes)[0].ID) != embeddedID {
			return protocolError(currentResponse, "expected exactly one embedded datasource with Metadata API ID %q while paging parents", embeddedID)
		}
		connection = (*root.Nodes)[0].ParentPublishedDatasourcesConnection
		if connection == nil {
			return protocolError(currentResponse, "embedded datasource %q omitted parentPublishedDatasourcesConnection while paging", embeddedID)
		}
		if connection.TotalCount == nil || *connection.TotalCount != total {
			return protocolError(currentResponse, "embedded datasource %q changed parent published datasource total while paging", embeddedID)
		}
		if connection.PageInfo == nil || connection.Nodes == nil {
			return protocolError(currentResponse, "embedded datasource %q returned an incomplete parent published datasource connection while paging", embeddedID)
		}
	}
}

func (c *Client) query(ctx context.Context, query string, variables map[string]any) (tableau.Response, graphqlEnvelope, error) {
	body, err := json.Marshal(graphqlRequest{Query: query, Variables: variables})
	if err != nil {
		return tableau.Response{}, graphqlEnvelope{}, fmt.Errorf("encode Tableau metadata query: %w", err)
	}
	response, err := c.transport.Do(ctx, c.session, tableau.Request{
		Method: http.MethodPost, ServerURL: c.serverURL, Path: "/api/metadata/graphql",
		Operation: queryOperation, Body: body, ContentType: "application/json", Accept: "application/json",
		MaxResponseBytes: maxResponseBytes,
	})
	if err != nil {
		return tableau.Response{}, graphqlEnvelope{}, err
	}
	var envelope graphqlEnvelope
	if err := json.Unmarshal(response.Body, &envelope); err != nil {
		return response, graphqlEnvelope{}, tableau.NewProtocolError(queryOperation, response, fmt.Errorf("decode Metadata API GraphQL response: %w", err), false)
	}
	warnings, err := decodeWarnings(envelope.Warnings)
	if err != nil {
		return response, graphqlEnvelope{}, tableau.NewProtocolError(queryOperation, response, fmt.Errorf("decode Metadata API GraphQL warnings: %w", err), false)
	}
	issues := append(append([]graphqlIssue(nil), envelope.Errors...), warnings...)
	if len(issues) > 0 {
		return response, graphqlEnvelope{}, newGraphQLIssuesError(response, issues)
	}
	return response, envelope, nil
}

func protocolError(response tableau.Response, format string, args ...any) error {
	return tableau.NewProtocolError(queryOperation, response, fmt.Errorf(format, args...), false)
}

func decodeWarnings(raw json.RawMessage) ([]graphqlIssue, error) {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" || value == "[]" || value == "{}" {
		return nil, nil
	}
	var warnings []graphqlIssue
	if err := json.Unmarshal(raw, &warnings); err != nil {
		return nil, err
	}
	return warnings, nil
}

type graphQLIssuesError struct {
	cause            error
	retryable        bool
	correctiveAction string
}

func newGraphQLIssuesError(response tableau.Response, issues []graphqlIssue) error {
	retryable := true
	hasPermissionIssue := false
	hasConfigurationIssue := false
	hasQueryIssue := false
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		code := issue.code()
		message := strings.TrimSpace(issue.Message)
		if message == "" {
			message = "Metadata API returned an error or warning"
		}
		parts = append(parts, fmt.Sprintf("%s: %s", code, message))
		switch code {
		case "BACKFILL_RUNNING", "INHERITANCE_INCOMPLETE", "LINKED_RESULTS_INCOMPLETE", "MAX_PAGE_SIZE_EXCEEDED", "NODE_LIMIT_EXCEEDED", "RATE_EXCEEDED", "TIME_LIMIT_EXCEEDED":
			// Retrying after indexing, throttling, or query limits recover can return complete results.
		case "ACCESS_DENIED", "FILTER_REQUIRED", "PERMISSIONS_MODE_SWITCHED", "USER_VISIBILITY_IS_LIMITED":
			retryable = false
			hasPermissionIssue = true
		case "FEATURE_DISABLED", "SITE_DISABLED":
			retryable = false
			hasConfigurationIssue = true
		case "INVALID_ARGUMENT":
			retryable = false
			hasQueryIssue = true
		default:
			retryable = false
			hasQueryIssue = true
		}
	}
	correctiveAction := "Retry after Tableau Metadata API indexing, throttling, or query limits recover."
	if !retryable {
		switch {
		case hasPermissionIssue:
			correctiveAction = "Verify Metadata API access and content permissions before retrying."
		case hasConfigurationIssue:
			correctiveAction = "Enable the Metadata API for the site before retrying."
		case hasQueryIssue:
			correctiveAction = "Correct the Metadata API query before retrying."
		default:
			correctiveAction = "Inspect the Metadata API response before retrying."
		}
	}
	cause := fmt.Errorf("Metadata API GraphQL issues: %s", strings.Join(parts, "; "))
	return &graphQLIssuesError{
		cause:            tableau.NewProtocolError(queryOperation, response, cause, retryable),
		retryable:        retryable,
		correctiveAction: correctiveAction,
	}
}

func (e *graphQLIssuesError) Error() string            { return e.cause.Error() }
func (e *graphQLIssuesError) Unwrap() error            { return e.cause }
func (e *graphQLIssuesError) Retryable() bool          { return e.retryable }
func (e *graphQLIssuesError) CorrectiveAction() string { return e.correctiveAction }

type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type graphqlEnvelope struct {
	Data     graphqlData     `json:"data"`
	Errors   []graphqlIssue  `json:"errors"`
	Warnings json.RawMessage `json:"warnings"`
}

type graphqlIssue struct {
	Message    string `json:"message"`
	Code       string `json:"code"`
	Extensions struct {
		Code     string `json:"code"`
		Severity string `json:"severity"`
	} `json:"extensions"`
}

func (i graphqlIssue) code() string {
	code := strings.TrimSpace(i.Extensions.Code)
	if code == "" {
		code = strings.TrimSpace(i.Code)
	}
	if code == "" {
		return "UNKNOWN"
	}
	return strings.ToUpper(code)
}

type graphqlData struct {
	WorkbooksConnection           *workbookConnection           `json:"workbooksConnection"`
	EmbeddedDatasourcesConnection *embeddedDatasourceConnection `json:"embeddedDatasourcesConnection"`
}

type workbookConnection struct {
	TotalCount *int            `json:"totalCount"`
	Nodes      *[]workbookNode `json:"nodes"`
}

type workbookNode struct {
	LUID                          string                        `json:"luid"`
	EmbeddedDatasourcesConnection *embeddedDatasourceConnection `json:"embeddedDatasourcesConnection"`
}

type embeddedDatasourceConnection struct {
	TotalCount *int                      `json:"totalCount"`
	PageInfo   *pageInfo                 `json:"pageInfo"`
	Nodes      *[]embeddedDatasourceNode `json:"nodes"`
}

type embeddedDatasourceNode struct {
	ID                                   string                         `json:"id"`
	Name                                 string                         `json:"name"`
	ParentPublishedDatasourcesConnection *publishedDatasourceConnection `json:"parentPublishedDatasourcesConnection"`
}

type publishedDatasourceConnection struct {
	TotalCount *int                       `json:"totalCount"`
	PageInfo   *pageInfo                  `json:"pageInfo"`
	Nodes      *[]publishedDatasourceNode `json:"nodes"`
}

type publishedDatasourceNode struct {
	LUID string `json:"luid"`
	Name string `json:"name"`
}

type pageInfo struct {
	HasNextPage *bool  `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}
