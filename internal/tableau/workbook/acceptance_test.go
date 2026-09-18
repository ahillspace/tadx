package workbook_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ahillspace/tadx/internal/tableau"
	"github.com/ahillspace/tadx/internal/tableau/workbook"
)

func TestAcceptanceHookRunsBeforeAnyJobRead(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost {
			t.Error("job read started before durable acceptance")
		}
		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, `<tsResponse><job id="accepted-job" type="PublishWorkbook" progress="0" finishCode="1"/></tsResponse>`)
	}))
	defer server.Close()
	client := workbook.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	got, err := client.Publish(t.Context(), workbook.PublishRequest{Name: "Book", ProjectLUID: "project", Filename: "Book.twbx", Content: []byte("small"), AsJob: true, Accepted: func(_ context.Context, id, request string) (workbook.PublishResult, error) {
		return workbook.PublishResult{Status: "pending", JobID: id, ReceiptPath: "receipt.json"}, errors.New("receipt test interrupted")
	}})
	if err == nil || calls != 1 || got.JobID != "accepted-job" || got.ReceiptPath != "receipt.json" || got.Status != "pending" {
		t.Fatalf("calls=%d got=%+v err=%v", calls, got, err)
	}
}
