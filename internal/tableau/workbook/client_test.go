package workbook_test

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/tableau"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

type session struct{}

func (session) Authorize(request *http.Request) { request.Header.Set("X-Tableau-Auth", "token") }
func (session) SiteLUID() string                { return "site-1" }
func (session) UserLUID() string                { return "user-1" }
func (session) String() string                  { return "session" }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

type multipartPart struct {
	name     string
	filename string
	content  []byte
}

func readMultipart(request *http.Request) ([]multipartPart, error) {
	mediaType, parameters, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}
	if mediaType != "multipart/mixed" {
		return nil, errors.New("request content type is not multipart/mixed")
	}
	reader := multipart.NewReader(request.Body, parameters["boundary"])
	var parts []multipartPart
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			return parts, nil
		}
		if err != nil {
			return nil, err
		}
		_, disposition, err := mime.ParseMediaType("form-data; " + part.Header.Get("Content-Disposition"))
		if err != nil {
			return nil, err
		}
		content, err := io.ReadAll(part)
		if err != nil {
			return nil, err
		}
		parts = append(parts, multipartPart{name: disposition["name"], filename: disposition["filename"], content: content})
	}
}

func TestClientNormalizesClassicWorkbookPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("pageNumber") != "2" || request.URL.Query().Get("pageSize") != "2" {
			t.Fatalf("query = %s", request.URL.RawQuery)
		}
		writer.Header().Set("Content-Type", "application/xml")
		_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="2" pageSize="2" totalAvailable="3"/><workbooks><workbook id="wb-3" name="Finance"><project id="project-1" name="Ops"/><owner id="owner-1"/></workbook></workbooks></tsResponse>`)
	}))
	defer server.Close()

	client := tableauworkbook.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	page, err := client.List(context.Background(), 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if page.Page.Number != 2 || page.Page.Size != 2 || page.Page.Total != 3 || len(page.Items) != 1 || page.Items[0].LUID != "wb-3" {
		t.Fatalf("page = %#v", page)
	}
}

func TestClientDownloadsNativeWorkbookAndFilename(t *testing.T) {
	include := func(value bool) *bool { return &value }
	tests := []struct {
		name           string
		includeExtract *bool
		wantQuery      string
	}{
		{name: "unspecified", wantQuery: ""},
		{name: "included", includeExtract: include(true), wantQuery: "includeExtract=true"},
		{name: "excluded", includeExtract: include(false), wantQuery: "includeExtract=false"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/api/3.29/sites/site-1/workbooks/wb-1/content" || request.URL.RawQuery != test.wantQuery {
					t.Fatalf("request URI = %s", request.URL.RequestURI())
				}
				writer.Header().Set("Content-Disposition", `name="tableau_workbook"; filename="Finance.twbx"`)
				writer.Header().Set("Content-Type", "application/octet-stream")
				_, _ = writer.Write([]byte("twbx-bytes"))
			}))
			defer server.Close()

			client := tableauworkbook.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
			download, err := client.Download(context.Background(), "wb-1", test.includeExtract)
			if err != nil {
				t.Fatal(err)
			}
			if download.Filename != "Finance.twbx" || !bytes.Equal(download.Content, []byte("twbx-bytes")) {
				t.Fatalf("download = %#v", download)
			}
		})
	}
}

func TestClientUsesUploadSessionAndBoundedJobPolling(t *testing.T) {
	var calls []string
	var appendParts [][]multipartPart
	var appendSequences []string
	var publishParts []multipartPart
	var handlerErr error
	jobPolls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls = append(calls, request.Method+" "+request.URL.RequestURI())
		writer.Header().Set("Content-Type", "application/xml")
		switch {
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/fileUploads"):
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><fileUpload uploadSessionId="upload-1" fileSize="0"/></tsResponse>`)
		case request.Method == http.MethodPut && strings.Contains(request.URL.Path, "/fileUploads/upload-1"):
			parts, err := readMultipart(request)
			if err != nil {
				handlerErr = err
				http.Error(writer, "invalid append body", http.StatusBadRequest)
				return
			}
			appendParts = append(appendParts, parts)
			appendSequences = append(appendSequences, request.URL.Query().Get("sequenceID"))
			_, _ = io.WriteString(writer, `<tsResponse><fileUpload uploadSessionId="upload-1" fileSize="1"/></tsResponse>`)
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/workbooks"):
			if request.URL.Query().Get("uploadSessionId") != "upload-1" || request.URL.Query().Get("asJob") != "true" || request.URL.Query().Get("overwrite") != "true" {
				t.Fatalf("publish query = %s", request.URL.RawQuery)
			}
			var err error
			publishParts, err = readMultipart(request)
			if err != nil {
				handlerErr = err
				http.Error(writer, "invalid publish body", http.StatusBadRequest)
				return
			}
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><job id="job-1" mode="Asynchronous" type="PublishWorkbook" progress="0" finishCode="1"/></tsResponse>`)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/jobs/job-1"):
			jobPolls++
			progress, finish := "0", "1"
			if jobPolls == 2 {
				progress, finish = "100", "0"
			}
			_, _ = io.WriteString(writer, `<tsResponse><job id="job-1" progress="`+progress+`" finishCode="`+finish+`"/></tsResponse>`)
		default:
			http.Error(writer, "unexpected", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := tableauworkbook.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	client.SetUploadThreshold(1)
	client.SetUploadChunkSize(3)
	client.SetPollPolicy(5*time.Millisecond, 100*time.Millisecond)
	result, err := client.Publish(context.Background(), tableauworkbook.PublishRequest{
		Name: "Finance", ProjectLUID: "project-1", Filename: "Finance.twbx", Content: []byte("abcdefg"), Overwrite: true, AsJob: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if handlerErr != nil {
		t.Fatal(handlerErr)
	}
	if result.JobID != "job-1" || result.Status != "succeeded" || jobPolls != 2 || len(calls) != 7 {
		t.Fatalf("result = %#v, polls = %d, calls = %v", result, jobPolls, calls)
	}
	if strings.Join(appendSequences, ",") != "1,2,3" {
		t.Fatalf("append sequence IDs = %v", appendSequences)
	}
	var uploaded []byte
	for index, parts := range appendParts {
		if len(parts) != 2 || parts[0].name != "request_payload" || len(parts[0].content) != 0 || parts[1].name != "tableau_file" || parts[1].filename != "Finance.twbx" {
			t.Fatalf("append %d parts = %#v", index+1, parts)
		}
		uploaded = append(uploaded, parts[1].content...)
	}
	if !bytes.Equal(uploaded, []byte("abcdefg")) {
		t.Fatalf("uploaded content = %q", uploaded)
	}
	if len(publishParts) != 1 || publishParts[0].name != "request_payload" {
		t.Fatalf("publish parts = %#v", publishParts)
	}
	var payload struct {
		Workbook struct {
			Name    string `xml:"name,attr"`
			Project struct {
				ID string `xml:"id,attr"`
			} `xml:"project"`
		} `xml:"workbook"`
	}
	if err := xml.Unmarshal(publishParts[0].content, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Workbook.Name != "Finance" || payload.Workbook.Project.ID != "project-1" {
		t.Fatalf("publish payload = %#v", payload)
	}
}

func TestClientPublishesSmallWorkbookInMultipartBody(t *testing.T) {
	var parts []multipartPart
	var handlerErr error
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("overwrite") != "false" || request.URL.Query().Has("asJob") {
			t.Fatalf("publish query = %s", request.URL.RawQuery)
		}
		parts, handlerErr = readMultipart(request)
		if handlerErr != nil {
			http.Error(writer, "invalid publish body", http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/xml")
		writer.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(writer, `<tsResponse><workbook id="wb-1" name="Finance"><project id="project-1"/></workbook></tsResponse>`)
	}))
	defer server.Close()

	client := tableauworkbook.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	result, err := client.Publish(context.Background(), tableauworkbook.PublishRequest{
		Name: "Finance", ProjectLUID: "project-1", Filename: "Finance.twb", Content: []byte("workbook-content"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if handlerErr != nil {
		t.Fatal(handlerErr)
	}
	if result.Status != "succeeded" || result.WorkbookLUID != "wb-1" {
		t.Fatalf("result = %#v", result)
	}
	if len(parts) != 2 || parts[0].name != "request_payload" || parts[1].name != "tableau_workbook" || parts[1].filename != "Finance.twb" || !bytes.Equal(parts[1].content, []byte("workbook-content")) {
		t.Fatalf("publish parts = %#v", parts)
	}
}

func TestClientEscapesMultipartFilenames(t *testing.T) {
	t.Run("direct publish", func(t *testing.T) {
		filename := `Finance "Q1".twb`
		var parts []multipartPart
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			var err error
			parts, err = readMultipart(request)
			if err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			writer.Header().Set("Content-Type", "application/xml")
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><workbook id="wb-1" name="Finance"><project id="project-1"/></workbook></tsResponse>`)
		}))
		defer server.Close()

		client := tableauworkbook.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
		if _, err := client.Publish(context.Background(), tableauworkbook.PublishRequest{
			Name: "Finance", ProjectLUID: "project-1", Filename: filename, Content: []byte("workbook-content"),
		}); err != nil {
			t.Fatal(err)
		}
		if len(parts) != 2 || parts[1].filename != filename {
			t.Fatalf("publish parts = %#v", parts)
		}
	})

	t.Run("upload append", func(t *testing.T) {
		filename := "Finance\nQ1.twbx"
		var appendParts []multipartPart
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "application/xml")
			switch {
			case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/fileUploads"):
				writer.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(writer, `<tsResponse><fileUpload uploadSessionId="upload-1" fileSize="0"/></tsResponse>`)
			case request.Method == http.MethodPut:
				var err error
				appendParts, err = readMultipart(request)
				if err != nil {
					http.Error(writer, err.Error(), http.StatusBadRequest)
					return
				}
				_, _ = io.WriteString(writer, `<tsResponse><fileUpload uploadSessionId="upload-1" fileSize="1"/></tsResponse>`)
			case request.Method == http.MethodPost:
				writer.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(writer, `<tsResponse><workbook id="wb-1" name="Finance"><project id="project-1"/></workbook></tsResponse>`)
			default:
				http.Error(writer, "unexpected request", http.StatusNotFound)
			}
		}))
		defer server.Close()

		client := tableauworkbook.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
		client.SetUploadThreshold(1)
		if _, err := client.Publish(context.Background(), tableauworkbook.PublishRequest{
			Name: "Finance", ProjectLUID: "project-1", Filename: filename, Content: []byte("workbook-content"),
		}); err != nil {
			t.Fatal(err)
		}
		if len(appendParts) != 2 || appendParts[1].filename != filename {
			t.Fatalf("append parts = %#v", appendParts)
		}
	})
}

func TestClientStopsPollingAtTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/xml")
		if request.Method == http.MethodPost {
			writer.Header().Set("X-Tableau-Request-Id", "publish-request-1")
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><job id="job-timeout" progress="0" finishCode="1"/></tsResponse>`)
			return
		}
		_, _ = io.WriteString(writer, `<tsResponse><job id="job-timeout" progress="0" finishCode="1"/></tsResponse>`)
	}))
	defer server.Close()

	client := tableauworkbook.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	client.SetPollPolicy(2*time.Millisecond, 8*time.Millisecond)
	result, err := client.Publish(context.Background(), tableauworkbook.PublishRequest{
		Name: "Finance", ProjectLUID: "project-1", Filename: "Finance.twb", Content: []byte("small"), AsJob: true,
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %v", err)
	}
	if result.JobID != "job-timeout" || result.TableauRequestID != "publish-request-1" {
		t.Fatalf("timeout result omitted identifiers: %#v", result)
	}
}

func TestClientReturnsFailingPollRequestID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/xml")
		if request.Method == http.MethodPost {
			writer.Header().Set("X-Tableau-Request-Id", "publish-request")
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><job id="job-failed-poll" progress="0" finishCode="1"/></tsResponse>`)
			return
		}
		writer.Header().Set("X-Tableau-Request-Id", "poll-request")
		writer.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(writer, `<tsResponse><error code="403007"><summary>Forbidden</summary><detail>Missing permission</detail></error></tsResponse>`)
	}))
	defer server.Close()

	client := tableauworkbook.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	result, err := client.Publish(context.Background(), tableauworkbook.PublishRequest{
		Name: "Finance", ProjectLUID: "project-1", Filename: "Finance.twb", Content: []byte("small"), AsJob: true,
	})
	if err == nil {
		t.Fatal("Publish() succeeded after a failed poll")
	}
	if result.Status != "unknown" || result.JobID != "job-failed-poll" || result.TableauRequestID != "poll-request" {
		t.Fatalf("poll failure result = %#v", result)
	}
}

func TestClientReportsUnknownWhenAcceptedPublishResponseCannotBeDecoded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/xml")
		writer.Header().Set("X-Tableau-Request-Id", "publish-request")
		writer.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(writer, `<tsResponse><job id="job-visible"/></broken>`)
	}))
	defer server.Close()

	client := tableauworkbook.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	result, err := client.Publish(context.Background(), tableauworkbook.PublishRequest{
		Name: "Finance", ProjectLUID: "project-1", Filename: "Finance.twb", Content: []byte("small"),
	})
	if err == nil {
		t.Fatal("Publish() succeeded with an invalid accepted response")
	}
	if result.Status != "unknown" || result.JobID != "job-visible" || result.TableauRequestID != "publish-request" {
		t.Fatalf("accepted response failure = %#v", result)
	}
}

func TestClientReportsUnknownWhenAcceptedPublishResponseCannotBeRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/xml")
		writer.Header().Set("Content-Length", "1024")
		writer.Header().Set("X-Tableau-Request-Id", "accepted-request")
		writer.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(writer, `<tsResponse><job id="job-partial"`)
	}))
	defer server.Close()

	client := tableauworkbook.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	result, err := client.Publish(context.Background(), tableauworkbook.PublishRequest{
		Name: "Finance", ProjectLUID: "project-1", Filename: "Finance.twb", Content: []byte("small"), AsJob: true,
	})
	if err == nil {
		t.Fatal("Publish() succeeded with a truncated accepted response")
	}
	if result.Status != "unknown" || result.TableauRequestID != "accepted-request" {
		t.Fatalf("accepted read failure result = %#v", result)
	}
}

func TestClientReportsUnknownWhenFinalPublishTransportFails(t *testing.T) {
	requests := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		_, _ = io.Copy(io.Discard, request.Body)
		return nil, errors.New("connection closed after request write")
	})}
	client := tableauworkbook.NewClient(tableau.NewTransport(httpClient, "3.29", nil), session{}, "https://tableau.example")
	result, err := client.Publish(context.Background(), tableauworkbook.PublishRequest{
		Name: "Finance", ProjectLUID: "project-1", Filename: "Finance.twb", Content: []byte("small"),
	})
	if err == nil {
		t.Fatal("Publish() succeeded after an indeterminate final request")
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	if result.Status != "unknown" {
		t.Fatalf("transport failure result = %#v", result)
	}
}

func TestClientPollingTimeoutBoundsInFlightRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/xml")
		if request.Method == http.MethodPost {
			writer.Header().Set("X-Tableau-Request-Id", "publish-request")
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><job id="job-blocked" progress="0" finishCode="1"/></tsResponse>`)
			return
		}
		writer.Header().Set("X-Tableau-Request-Id", "poll-request")
		writer.WriteHeader(http.StatusOK)
		writer.(http.Flusher).Flush()
		<-request.Context().Done()
	}))
	defer server.Close()

	client := tableauworkbook.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	client.SetPollPolicy(time.Millisecond, 20*time.Millisecond)
	started := time.Now()
	result, err := client.Publish(context.Background(), tableauworkbook.PublishRequest{
		Name: "Finance", ProjectLUID: "project-1", Filename: "Finance.twb", Content: []byte("small"), AsJob: true,
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatalf("polling exceeded bound: %s", time.Since(started))
	}
	if result.Status != "timed_out" || result.JobID != "job-blocked" || result.TableauRequestID != "poll-request" {
		t.Fatalf("poll timeout result = %#v", result)
	}
}

func TestClientRejectsIncompleteOrMismatchedTerminalJobResponses(t *testing.T) {
	tests := []struct {
		name      string
		response  string
		errorText string
	}{
		{name: "mismatched identity", response: `<tsResponse><job id="job-other" progress="100" finishCode="0"/></tsResponse>`, errorText: "expected"},
		{name: "missing progress", response: `<tsResponse><job id="job-1" finishCode="0"/></tsResponse>`, errorText: "progress"},
		{name: "missing terminal finish code", response: `<tsResponse><job id="job-1" progress="100"/></tsResponse>`, errorText: "finish code"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/xml")
				if request.Method == http.MethodPost {
					writer.Header().Set("X-Tableau-Request-Id", "publish-request")
					writer.WriteHeader(http.StatusCreated)
					_, _ = io.WriteString(writer, `<tsResponse><job id="job-1" progress="0" finishCode="1"/></tsResponse>`)
					return
				}
				writer.Header().Set("X-Tableau-Request-Id", "poll-request")
				_, _ = io.WriteString(writer, test.response)
			}))
			defer server.Close()

			client := tableauworkbook.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
			result, err := client.Publish(context.Background(), tableauworkbook.PublishRequest{
				Name: "Finance", ProjectLUID: "project-1", Filename: "Finance.twb", Content: []byte("small"), AsJob: true,
			})
			if err == nil || !strings.Contains(err.Error(), test.errorText) {
				t.Fatalf("error = %v", err)
			}
			if result.Status != "unknown" || result.JobID != "job-1" || result.TableauRequestID != "poll-request" {
				t.Fatalf("protocol failure result = %#v", result)
			}
		})
	}
}

func TestClientRejectsMismatchedUploadAppendIdentity(t *testing.T) {
	publishCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/xml")
		switch {
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/fileUploads"):
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><fileUpload uploadSessionId="upload-1" fileSize="0"/></tsResponse>`)
		case request.Method == http.MethodPut && strings.HasSuffix(request.URL.Path, "/fileUploads/upload-1"):
			_, _ = io.WriteString(writer, `<tsResponse><fileUpload uploadSessionId="upload-other" fileSize="5"/></tsResponse>`)
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/workbooks"):
			publishCalls++
		default:
			http.Error(writer, "unexpected", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := tableauworkbook.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	client.SetUploadThreshold(1)
	_, err := client.Publish(context.Background(), tableauworkbook.PublishRequest{
		Name: "Finance", ProjectLUID: "project-1", Filename: "Finance.twbx", Content: []byte("large"),
	})
	if err == nil || !strings.Contains(err.Error(), "upload session ID") {
		t.Fatalf("error = %v", err)
	}
	if publishCalls != 0 {
		t.Fatalf("publish calls = %d", publishCalls)
	}
}
