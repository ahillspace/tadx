// Package metadata implements the Tableau Metadata API client family.
package metadata

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/tableau"
)

const (
	maxResponseBytes = 4 * 1024 * 1024
	queryOperation   = "metadata.query"
)

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
