//go:build live

package metadata_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/tableau"
	tableauauth "github.com/ahillspace/tadx/internal/tableau/auth"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	tableaumetadata "github.com/ahillspace/tadx/internal/tableau/metadata"
)

const maxLiveEvidenceBytes = 4 * 1024 * 1024

func TestLiveWorkbookPublishedDatasourceContract(t *testing.T) {
	workbookLUID := strings.TrimSpace(os.Getenv("TADX_LIVE_PDS_WORKBOOK_LUID"))
	if workbookLUID == "" {
		t.Skip("set TADX_LIVE_PDS_WORKBOOK_LUID to run the live published-datasource contract test")
	}

	configPath := strings.TrimSpace(os.Getenv("TADX_LIVE_CONFIG"))
	if configPath == "" {
		var err error
		configPath, err = config.UserConfigPath()
		if err != nil {
			t.Fatal(err)
		}
	}
	configuration, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := configuration.ResolveEnvironment(strings.TrimSpace(os.Getenv("TADX_LIVE_ENVIRONMENT")))
	if err != nil {
		t.Fatal(err)
	}

	capture := &metadataEvidenceTransport{base: http.DefaultTransport}
	httpClient := &http.Client{Transport: capture, Timeout: 2 * time.Minute}
	transport := tableau.NewTransport(httpClient, environment.APIVersion, func() string { return "pds-live-contract" })
	provider := coreauth.NewPATProvider(coreauth.LookupEnvFunc(os.LookupEnv), tableauauth.NewClient(transport))
	session, err := provider.Authenticate(context.Background(), coreauth.Target{
		Environment:       environment.Alias,
		ServerURL:         environment.URL,
		SiteContentURL:    environment.SiteContentURL,
		PATNameVariable:   environment.Auth.PATNameEnv,
		PATSecretVariable: environment.Auth.PATSecretEnv,
	})
	if err != nil {
		t.Fatal(err)
	}

	references, err := tableaumetadata.NewClient(transport, session, environment.URL).DirectPublishedDatasources(context.Background(), workbookLUID)
	if err != nil {
		t.Fatal(err)
	}
	if len(references) == 0 {
		t.Fatalf("workbook %q returned no direct published datasource references", workbookLUID)
	}

	datasources := tableaudatasource.NewClient(transport, session, environment.URL)
	luids := make([]string, 0, len(references))
	for _, reference := range references {
		item, getErr := datasources.Get(context.Background(), reference.LUID)
		if getErr != nil {
			t.Fatalf("Metadata LUID %q was not accepted by the REST datasource endpoint: %v", reference.LUID, getErr)
		}
		if item.LUID != reference.LUID {
			t.Fatalf("REST datasource LUID = %q, Metadata LUID = %q", item.LUID, reference.LUID)
		}
		luids = append(luids, evidenceIdentifier(reference.LUID))
	}
	sort.Strings(luids)

	evidence, err := capture.JSON()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("verified Metadata published datasource LUIDs through REST: %s", strings.Join(luids, ", "))
	t.Logf("redacted Metadata API request and response capture:\n%s", evidence)
}

type metadataEvidenceTransport struct {
	base      http.RoundTripper
	mu        sync.Mutex
	exchanges []metadataExchange
}

type metadataExchange struct {
	Method           string          `json:"method"`
	Path             string          `json:"path"`
	Request          json.RawMessage `json:"request"`
	Status           int             `json:"status"`
	TableauRequestID string          `json:"tableau_request_id,omitempty"`
	Response         json.RawMessage `json:"response"`
}

func (t *metadataEvidenceTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Path != "/api/metadata/graphql" {
		return t.base.RoundTrip(request)
	}

	requestBody, err := readAndRestoreRequest(request, maxLiveEvidenceBytes)
	if err != nil {
		return nil, err
	}
	response, err := t.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	responseBody, err := readAndRestoreResponse(response, maxLiveEvidenceBytes)
	if err != nil {
		return nil, err
	}

	exchange := metadataExchange{
		Method:           request.Method,
		Path:             request.URL.Path,
		Request:          sanitizeMetadataJSON(requestBody),
		Status:           response.StatusCode,
		TableauRequestID: evidenceIdentifier(response.Header.Get("X-Tableau-Request-Id")),
		Response:         sanitizeMetadataJSON(responseBody),
	}
	t.mu.Lock()
	t.exchanges = append(t.exchanges, exchange)
	t.mu.Unlock()
	return response, nil
}

func (t *metadataEvidenceTransport) JSON() ([]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.exchanges) == 0 {
		return nil, fmt.Errorf("live test captured no Metadata API exchanges")
	}
	return json.MarshalIndent(t.exchanges, "", "  ")
}

func readAndRestoreRequest(request *http.Request, limit int64) ([]byte, error) {
	if request.Body == nil {
		return nil, nil
	}
	data, err := readBounded(request.Body, limit)
	if err != nil {
		return nil, err
	}
	request.Body = io.NopCloser(bytes.NewReader(data))
	return data, nil
}

func readAndRestoreResponse(response *http.Response, limit int64) ([]byte, error) {
	if response.Body == nil {
		return nil, nil
	}
	data, err := readBounded(response.Body, limit)
	if err != nil {
		_ = response.Body.Close()
		return nil, err
	}
	_ = response.Body.Close()
	response.Body = io.NopCloser(bytes.NewReader(data))
	return data, nil
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("live evidence body exceeded %d bytes", limit)
	}
	return data, nil
}

func sanitizeMetadataJSON(data []byte) json.RawMessage {
	var value any
	if len(data) == 0 || json.Unmarshal(data, &value) != nil {
		return json.RawMessage(`{"redacted":"non-JSON body omitted"}`)
	}
	redactMetadataValue(value)
	result, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{"redacted":"unencodable body omitted"}`)
	}
	return result
}

func redactMetadataValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			switch strings.ToLower(key) {
			case "query", "code":
				// The static query and stable Tableau issue codes carry no fixture identity.
			case "luid", "workbookluid":
				if identifier, ok := child.(string); ok {
					typed[key] = evidenceIdentifier(identifier)
				} else {
					redactMetadataValue(child)
				}
			default:
				if _, ok := child.(string); ok {
					typed[key] = "[REDACTED]"
				} else {
					redactMetadataValue(child)
				}
			}
		}
	case []any:
		for _, child := range typed {
			redactMetadataValue(child)
		}
	}
}

func evidenceIdentifier(value string) string {
	if value == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("sha256:%x", digest[:8])
}

func TestLiveEvidenceRedactsFixtureIdentifiers(t *testing.T) {
	input := []byte(`{"query":"query Fixture($workbookLuid: String!) { workbooksConnection { nodes { luid name } } }","variables":{"workbookLuid":"workbook-secret","embeddedAfter":"cursor-secret"},"data":{"workbooksConnection":{"nodes":[{"id":"metadata-secret","luid":"datasource-secret","name":"Sales secret"}]}},"errors":[{"message":"fixture secret","extensions":{"code":"ACCESS_DENIED"}}]}`)
	redacted := string(sanitizeMetadataJSON(input))
	for _, secret := range []string{"workbook-secret", "cursor-secret", "metadata-secret", "datasource-secret", "Sales secret", "fixture secret"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("redacted evidence contains %q: %s", secret, redacted)
		}
	}
	for _, identifier := range []string{"workbook-secret", "datasource-secret"} {
		if !strings.Contains(redacted, evidenceIdentifier(identifier)) {
			t.Fatalf("redacted evidence omitted stable identifier digest for %q: %s", identifier, redacted)
		}
	}
	if !strings.Contains(redacted, "ACCESS_DENIED") {
		t.Fatalf("redacted evidence omitted stable issue code: %s", redacted)
	}
}
