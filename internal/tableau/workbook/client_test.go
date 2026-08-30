package workbook_test

import (
	"bytes"
	"context"
	"io"
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
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/3.29/sites/site-1/workbooks/wb-1/content" {
			t.Fatalf("path = %s", request.URL.Path)
		}
		writer.Header().Set("Content-Disposition", `name="tableau_workbook"; filename="Finance.twbx"`)
		writer.Header().Set("Content-Type", "application/octet-stream")
		_, _ = writer.Write([]byte("twbx-bytes"))
	}))
	defer server.Close()

	client := tableauworkbook.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	download, err := client.Download(context.Background(), "wb-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if download.Filename != "Finance.twbx" || !bytes.Equal(download.Content, []byte("twbx-bytes")) {
		t.Fatalf("download = %#v", download)
	}
}

func TestClientUsesUploadSessionAndBoundedJobPolling(t *testing.T) {
	var calls []string
	jobPolls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls = append(calls, request.Method+" "+request.URL.RequestURI())
		writer.Header().Set("Content-Type", "application/xml")
		switch {
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/fileUploads"):
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><fileUpload uploadSessionId="upload-1" fileSize="0"/></tsResponse>`)
		case request.Method == http.MethodPut && strings.Contains(request.URL.Path, "/fileUploads/upload-1"):
			if request.URL.Query().Get("sequenceID") == "" {
				t.Fatal("append request omitted sequenceID")
			}
			_, _ = io.WriteString(writer, `<tsResponse><fileUpload uploadSessionId="upload-1" fileSize="1"/></tsResponse>`)
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/workbooks"):
			if request.URL.Query().Get("uploadSessionId") != "upload-1" || request.URL.Query().Get("asJob") != "true" {
				t.Fatalf("publish query = %s", request.URL.RawQuery)
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
	client.SetPollPolicy(5*time.Millisecond, 100*time.Millisecond)
	result, err := client.Publish(context.Background(), tableauworkbook.PublishRequest{
		Name: "Finance", ProjectLUID: "project-1", Filename: "Finance.twbx", Content: []byte("large"), Overwrite: true, AsJob: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.JobID != "job-1" || result.Status != "succeeded" || jobPolls != 2 || len(calls) < 5 {
		t.Fatalf("result = %#v, polls = %d, calls = %v", result, jobPolls, calls)
	}
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
