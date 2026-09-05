package flow_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/tableau"
	tableauflow "github.com/ahillspace/tadx/internal/tableau/flow"
)

type session struct{}

func (session) String() string          { return "test flow session" }
func (session) Authorize(*http.Request) {}
func (session) SiteLUID() string        { return "site-1" }
func (session) UserLUID() string        { return "user-1" }

var _ coreauth.Session = session{}

func TestClientUpdatesOnlyExplicitFlowOwner(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut || request.URL.EscapedPath() != "/api/3.29/sites/site-1/flows/flow-1/owner/owner-2" || request.URL.RawQuery != "" {
			t.Fatalf("request = %s %s?%s", request.Method, request.URL.EscapedPath(), request.URL.RawQuery)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if len(body) != 0 {
			t.Fatalf("body = %q, want empty", body)
		}
		writer.Header().Set("X-Tableau-Request-Id", "request-update")
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	owner := "owner-2"
	client := tableauflow.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	result, err := client.Update(context.Background(), tableauflow.UpdateRequest{LUID: "flow-1", OwnerLUID: &owner})
	if err != nil || result.Status != "succeeded" || result.OwnerLUID != owner || result.TableauRequestID != "request-update" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestClientRejectsFlowUpdateWithoutExactOwner(t *testing.T) {
	client := tableauflow.NewClient(nil, nil, "")
	empty := ""
	for _, input := range []tableauflow.UpdateRequest{{LUID: "flow-1"}, {OwnerLUID: &empty}, {LUID: "flow-1", OwnerLUID: &empty}} {
		if _, err := client.Update(context.Background(), input); err == nil {
			t.Fatalf("Update(%#v) succeeded", input)
		}
	}
}

type emptySiteSession struct{ session }

func (emptySiteSession) SiteLUID() string { return "" }

func TestClientRejectsSessionWithoutSiteIdentityBeforeRequest(t *testing.T) {
	requests := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("unexpected request")
	})}
	client := tableauflow.NewClient(tableau.NewTransport(httpClient, "3.29", nil), emptySiteSession{}, "https://tableau.example")
	_, err := client.Get(context.Background(), "flow-1")
	if err == nil || !strings.Contains(err.Error(), "site LUID") || requests != 0 {
		t.Fatalf("Get() error = %v, requests = %d", err, requests)
	}
}

func TestClientListsOneBoundedFilteredPage(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/3.29/sites/site-1/flows" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("pageNumber") != "2" || query.Get("pageSize") != "25" {
			t.Errorf("pagination query = %q", request.URL.RawQuery)
		}
		filter := query.Get("filter")
		for _, want := range []string{"name:eq:Daily", "ownerName:eq:Operator", "projectId:eq:project-1", "projectName:eq:Operations"} {
			if !strings.Contains(filter, want) {
				t.Errorf("filter = %q, missing %q", filter, want)
			}
		}
		writer.Header().Set("Content-Type", "application/xml")
		writer.Header().Set("X-Tableau-Request-Id", "list-request")
		_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="2" pageSize="25" totalAvailable="26"/><flows><flow id="flow-1" name="Daily" description="Daily prep" fileType="tflx" createdAt="2026-08-01T00:00:00Z" updatedAt="2026-08-02T00:00:00Z"><project id="project-1" name="Operations"/><owner id="owner-1"/><tags><tag label="certified"/></tags><parameters><parameter id="parameter-1" type="string" name="Region" description="Target region" value="West" isRequired="true"/></parameters></flow></flows></tsResponse>`)
	}))
	defer server.Close()

	client := tableauflow.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	page, err := client.List(context.Background(), tableauflow.ListRequest{PageNumber: 2, PageSize: 25, Name: "Daily", OwnerName: "Operator", ProjectLUID: "project-1", ProjectName: "Operations"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Number != 2 || page.Size != 25 || page.Total != 26 || len(page.Items) != 1 {
		t.Fatalf("page = %#v", page)
	}
	flow := page.Items[0]
	if flow.LUID != "flow-1" || flow.ProjectLUID != "project-1" || flow.OwnerLUID != "owner-1" || len(flow.Tags) != 1 || len(flow.Parameters) != 1 || flow.Parameters[0].Required == nil || !*flow.Parameters[0].Required {
		t.Fatalf("flow = %#v", flow)
	}
}

func TestClientRejectsUnsupportedFlowFilterSeparatorsBeforeRequest(t *testing.T) {
	requests := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("unexpected request")
	})}
	client := tableauflow.NewClient(tableau.NewTransport(httpClient, "3.29", nil), session{}, "https://tableau.example")
	for _, value := range []string{"One,Two", "Research & Development"} {
		_, err := client.List(context.Background(), tableauflow.ListRequest{Name: value})
		if err == nil || !strings.Contains(err.Error(), "cannot contain") {
			t.Fatalf("List(%q) error = %v", value, err)
		}
	}
	if requests != 0 {
		t.Fatalf("requests = %d", requests)
	}
}

func TestClientGetsExactFlowWithOutputSteps(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/xml")
		_, _ = io.WriteString(writer, `<tsResponse><flowOutputSteps><flowOutputStep id="output-1" name="Extract"/></flowOutputSteps><flow id="flow-1" name="Daily" fileType="tfl"><project id="project-1" name="Operations"/><owner id="owner-1"/><tags/><parameters/></flow></tsResponse>`)
	}))
	defer server.Close()

	client := tableauflow.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	flow, err := client.Get(context.Background(), "flow-1")
	if err != nil {
		t.Fatal(err)
	}
	if flow.LUID != "flow-1" || len(flow.OutputSteps) != 1 || flow.OutputSteps[0].LUID != "output-1" {
		t.Fatalf("flow = %#v", flow)
	}
}

func TestClientRejectsMismatchedExactFlowIdentity(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Tableau-Request-Id", "get-request")
		_, _ = io.WriteString(writer, `<tsResponse><flow id="flow-other" name="Daily"><project id="project-1"/></flow></tsResponse>`)
	}))
	defer server.Close()

	client := tableauflow.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	_, err := client.Get(context.Background(), "flow-1")
	if err == nil || !strings.Contains(err.Error(), "expected") || tableau.RequestID(err) != "get-request" {
		t.Fatalf("Get() error = %v, request ID = %q", err, tableau.RequestID(err))
	}
}

func TestClientDownloadsUnchangedNativeFlowWithinBound(t *testing.T) {
	want := []byte("native\x00flow\r\nbytes")
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/3.29/sites/site-1/flows/flow-1/content" {
			t.Errorf("path = %q", request.URL.Path)
		}
		writer.Header().Set("Content-Disposition", `attachment; filename="Daily.tflx"`)
		writer.Header().Set("X-Tableau-Request-Id", "download-request")
		_, _ = writer.Write(want)
	}))
	defer server.Close()

	client := tableauflow.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	client.SetMaxDownloadBytes(int64(len(want)))
	download, err := client.Download(context.Background(), "flow-1")
	if err != nil {
		t.Fatal(err)
	}
	if download.Filename != "Daily.tflx" || !bytes.Equal(download.Content, want) || download.TableauRequestID != "download-request" {
		t.Fatalf("download = %#v", download)
	}
}

func TestClientBoundsFlowDownload(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Disposition", `attachment; filename="Daily.tfl"`)
		writer.Header().Set("X-Tableau-Request-Id", "download-request")
		_, _ = io.WriteString(writer, "12345")
	}))
	defer server.Close()

	client := tableauflow.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	client.SetMaxDownloadBytes(4)
	_, err := client.Download(context.Background(), "flow-1")
	if err == nil || tableau.RequestID(err) != "download-request" {
		t.Fatalf("Download() error = %v, request ID = %q", err, tableau.RequestID(err))
	}
}

func TestClientPreparesDirectPublishThenCommitsExactlyOnce(t *testing.T) {
	var calls int
	var parts []multipartPart
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		if request.Method != http.MethodPost || request.URL.Path != "/api/3.29/sites/site-1/flows" || request.URL.Query().Get("overwrite") != "false" {
			t.Errorf("request = %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
		}
		var err error
		parts, err = readMultipart(request)
		if err != nil {
			t.Errorf("multipart: %v", err)
		}
		writer.Header().Set("X-Tableau-Request-Id", "publish-request")
		writer.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(writer, `<tsResponse><flow id="flow-new" name="Daily"><project id="project-1"/></flow></tsResponse>`)
	}))
	defer server.Close()

	client := tableauflow.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	prepared, err := client.Prepare(context.Background(), tableauflow.PublishRequest{Name: "Daily", ProjectLUID: "project-1", Filename: "Daily.tflx", ContentPath: writePayload(t, "Daily.tflx", []byte("native-flow")), ContentSize: 11, ExpectedFingerprint: fingerprint([]byte("native-flow"))})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("prepare made %d final publish requests", calls)
	}
	result, err := prepared.Commit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" || result.FlowLUID != "flow-new" || result.TableauRequestID != "publish-request" {
		t.Fatalf("result = %#v", result)
	}
	if len(parts) != 2 || parts[0].name != "request_payload" || parts[1].name != "tableau_flow" || parts[1].filename != "Daily.tflx" || !bytes.Equal(parts[1].content, []byte("native-flow")) || !strings.Contains(string(parts[0].content), `<flow name="Daily"><project id="project-1"></project></flow>`) {
		t.Fatalf("parts = %#v", parts)
	}
	if _, err := prepared.Commit(context.Background()); err == nil || calls != 1 {
		t.Fatalf("second Commit() error = %v, calls = %d", err, calls)
	}
}

func TestClientUsesOrderedUploadSessionAndVerifiesAppendIdentity(t *testing.T) {
	var mu sync.Mutex
	var sequences []string
	var finalQuery string
	var appendParts [][]multipartPart
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/xml")
		switch {
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/fileUploads"):
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><fileUpload uploadSessionId="upload-1" fileSize="0"/></tsResponse>`)
		case request.Method == http.MethodPut && strings.HasSuffix(request.URL.Path, "/fileUploads/upload-1"):
			parts, err := readMultipart(request)
			if err != nil {
				t.Errorf("append multipart: %v", err)
			}
			mu.Lock()
			sequences = append(sequences, request.URL.Query().Get("sequenceID"))
			appendParts = append(appendParts, parts)
			mu.Unlock()
			_, _ = io.WriteString(writer, `<tsResponse><fileUpload uploadSessionId="upload-1" fileSize="1"/></tsResponse>`)
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/flows"):
			finalQuery = request.URL.RawQuery
			parts, err := readMultipart(request)
			if err != nil || len(parts) != 1 || parts[0].name != "request_payload" {
				t.Errorf("final multipart = %#v, error = %v", parts, err)
			}
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><flow id="flow-new" name="Daily"><project id="project-1"/></flow></tsResponse>`)
		default:
			http.Error(writer, "unexpected", http.StatusNotFound)
		}
	}))
	defer server.Close()

	content := []byte("abcdef")
	client := tableauflow.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	client.SetUploadThreshold(1)
	client.SetUploadChunkSize(3)
	prepared, err := client.Prepare(context.Background(), tableauflow.PublishRequest{Name: "Daily", ProjectLUID: "project-1", Filename: "Daily.tfl", ContentPath: writePayload(t, "Daily.tfl", content), ContentSize: int64(len(content)), ExpectedFingerprint: fingerprint(content), Overwrite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Join(sequences, ",") != "1,2" || !strings.Contains(finalQuery, "uploadSessionId=upload-1") || !strings.Contains(finalQuery, "flowType=tfl") || !strings.Contains(finalQuery, "overwrite=true") {
		t.Fatalf("sequences = %v, final query = %q", sequences, finalQuery)
	}
	if len(appendParts) != 2 || len(appendParts[0]) != 2 || appendParts[0][1].name != "tableau_file" || !bytes.Equal(appendParts[0][1].content, []byte("abc")) || !bytes.Equal(appendParts[1][1].content, []byte("def")) {
		t.Fatalf("append parts = %#v", appendParts)
	}
}

func TestClientRejectsChangedPublishPayloadBeforeRemoteRequest(t *testing.T) {
	path := writePayload(t, "Daily.tflx", []byte("changed"))
	requests := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("unexpected request")
	})}
	client := tableauflow.NewClient(tableau.NewTransport(httpClient, "3.29", nil), session{}, "https://tableau.example")
	_, err := client.Prepare(context.Background(), tableauflow.PublishRequest{Name: "Daily", ProjectLUID: "project-1", Filename: "Daily.tflx", ContentPath: path, ContentSize: 7, ExpectedFingerprint: fingerprint([]byte("planned"))})
	if err == nil || !strings.Contains(err.Error(), "changed after planning") || requests != 0 {
		t.Fatalf("Prepare() error = %v, requests = %d", err, requests)
	}
}

func TestClientRejectsMalformedAPIVersionBeforeStartingChunkedUpload(t *testing.T) {
	requests := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("unexpected request")
	})}
	content := []byte("native-flow")
	client := tableauflow.NewClient(tableau.NewTransport(httpClient, "invalid", nil), session{}, "https://tableau.example")
	client.SetUploadThreshold(1)
	_, err := client.Prepare(context.Background(), tableauflow.PublishRequest{Name: "Daily", ProjectLUID: "project-1", Filename: "Daily.tflx", ContentPath: writePayload(t, "Daily.tflx", content), ContentSize: int64(len(content)), ExpectedFingerprint: fingerprint(content)})
	if err == nil || !strings.Contains(err.Error(), "version") || requests != 0 {
		t.Fatalf("Prepare() error = %v, requests = %d", err, requests)
	}
}

func TestClientMovesWithProjectOnlyAndDeletesWithNoBody(t *testing.T) {
	var moveBody []byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Tableau-Request-Id", "mutation-request")
		switch request.Method {
		case http.MethodPut:
			moveBody, _ = io.ReadAll(request.Body)
			_, _ = io.WriteString(writer, `<tsResponse><flow id="flow-1" name="Daily"><project id="project-2" name="Archive"/></flow></tsResponse>`)
		case http.MethodDelete:
			body, _ := io.ReadAll(request.Body)
			if len(body) != 0 {
				t.Errorf("delete body = %q", body)
			}
			writer.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("method = %s", request.Method)
		}
	}))
	defer server.Close()

	client := tableauflow.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	move, err := client.Move(context.Background(), "flow-1", "project-2")
	if err != nil {
		t.Fatal(err)
	}
	if move.Status != "succeeded" || move.FlowLUID != "flow-1" || move.ProjectLUID != "project-2" || strings.Contains(string(moveBody), "owner") || string(moveBody) != `<tsRequest><flow><project id="project-2"></project></flow></tsRequest>` {
		t.Fatalf("move = %#v, body = %q", move, moveBody)
	}
	deleted, err := client.Delete(context.Background(), "flow-1")
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Status != "succeeded" || deleted.FlowLUID != "flow-1" || deleted.TableauRequestID != "mutation-request" {
		t.Fatalf("delete = %#v", deleted)
	}
}

func TestClientDoesNotRetryUncertainFinalMutations(t *testing.T) {
	for _, test := range []struct {
		name string
		call func(*tableauflow.RESTClient) (tableauflow.MutationResult, error)
	}{
		{name: "move", call: func(client *tableauflow.RESTClient) (tableauflow.MutationResult, error) {
			return client.Move(context.Background(), "flow-1", "project-2")
		}},
		{name: "delete", call: func(client *tableauflow.RESTClient) (tableauflow.MutationResult, error) {
			return client.Delete(context.Background(), "flow-1")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				requests++
				_, _ = io.Copy(io.Discard, request.Body)
				return nil, errors.New("connection closed after request write")
			})}
			client := tableauflow.NewClient(tableau.NewTransport(httpClient, "3.29", nil), session{}, "https://tableau.example")
			result, err := test.call(client)
			if err == nil || requests != 1 || result.Status != "unknown" || result.FlowLUID != "flow-1" {
				t.Fatalf("result = %#v, error = %v, requests = %d", result, err, requests)
			}
			if test.name == "move" && result.ProjectLUID != "project-2" {
				t.Fatalf("move result omitted planned project: %#v", result)
			}
		})
	}
}

func TestPreparedPublishReportsUnknownOnIndeterminateCommit(t *testing.T) {
	requests := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		_, _ = io.Copy(io.Discard, request.Body)
		return nil, errors.New("connection closed after request write")
	})}
	client := tableauflow.NewClient(tableau.NewTransport(httpClient, "3.29", nil), session{}, "https://tableau.example")
	content := []byte("native-flow")
	prepared, err := client.Prepare(context.Background(), tableauflow.PublishRequest{Name: "Daily", ProjectLUID: "project-1", Filename: "Daily.tflx", ContentPath: writePayload(t, "Daily.tflx", content), ContentSize: int64(len(content)), ExpectedFingerprint: fingerprint(content)})
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Commit(context.Background())
	if err == nil || result.Status != "unknown" || requests != 1 {
		t.Fatalf("result = %#v, error = %v, requests = %d", result, err, requests)
	}
}

type multipartPart struct {
	name, filename string
	content        []byte
}

func readMultipart(request *http.Request) ([]multipartPart, error) {
	reader, err := request.MultipartReader()
	if err != nil {
		return nil, err
	}
	var result []multipartPart
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			return result, nil
		}
		if err != nil {
			return nil, err
		}
		content, err := io.ReadAll(part)
		if err != nil {
			return nil, err
		}
		name := part.FormName()
		filename := part.FileName()
		if name == "" {
			disposition := part.Header.Get("Content-Disposition")
			for _, field := range strings.Split(disposition, ";") {
				key, value, found := strings.Cut(strings.TrimSpace(field), "=")
				if !found {
					continue
				}
				switch strings.ToLower(key) {
				case "name":
					name = strings.Trim(value, `"`)
				case "filename":
					filename = strings.Trim(value, `"`)
				}
			}
		}
		result = append(result, multipartPart{name: name, filename: filename, content: content})
	}
}

func writePayload(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func fingerprint(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }
