package datasource_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/tableau"
	ds "github.com/ahillspace/tadx/internal/tableau/datasource"
	"github.com/ahillspace/tadx/internal/value"
)

const sourceID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
const copyID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
const tdsBase = "/api/3.30/tds/sites/site-1/"

func tdsFixture(t *testing.T, version string, handle http.HandlerFunc) *ds.Client {
	t.Helper()
	server := httptest.NewTLSServer(handle)
	t.Cleanup(server.Close)
	return ds.NewClient(tableau.NewTransport(server.Client(), version, nil), session{}, server.URL)
}

func TestTDSCheckoutAndPublishWire(t *testing.T) {
	calls := 0
	client := tdsFixture(t, "3.30", func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.Header.Get("X-Tableau-Auth") == "" || r.URL.RawQuery != "" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		switch r.URL.Path {
		case tdsBase + "datasource/" + sourceID + "/workcopy":
			if r.ContentLength != 0 {
				t.Error("checkout has a body")
			}
			w.WriteHeader(201)
			io.WriteString(w, `{"tdsWorkCopy":{"workCopyLuid":"`+copyID+`"}}`)
		case tdsBase + "workcopy/" + copyID + "/publish":
			body, _ := io.ReadAll(r.Body)
			if !sameJSON(string(body), `{"datasource":{"name":"New sales","description":""}}`) {
				t.Errorf("publish body %s", body)
			}
			w.WriteHeader(201)
			io.WriteString(w, `{"tdsWorkCopy":{"datasourceLuid":"`+sourceID+`"}}`)
		default:
			t.Errorf("path %s", r.URL.Path)
		}
	})
	got, err := client.CreateWorkCopy(t.Context(), sourceID)
	if err != nil || got.Status != "acknowledged" || got.WorkCopyLUID != copyID {
		t.Fatalf("%#v %v", got, err)
	}
	desc := ""
	got, err = client.PublishWorkCopy(t.Context(), copyID, "New sales", &desc)
	if err != nil || got.Status != "acknowledged" || got.DatasourceLUID != sourceID || calls != 2 {
		t.Fatalf("%#v %v", got, err)
	}
}

func sameJSON(left, right string) bool {
	var l, r any
	return json.Unmarshal([]byte(left), &l) == nil && json.Unmarshal([]byte(right), &r) == nil && reflect.DeepEqual(l, r)
}

func TestTDSExactBulkReadsAndAbsence(t *testing.T) {
	for _, tc := range []struct {
		kind, id, name, body string
		status               int
		absent, wantErr      bool
		rows                 int
	}{
		{"workcopy", copyID, "A /?#", `{"tdsElementList":[{"name":"A /?#","description":""}]}`, 200, false, false, 1},
		{"datasource", sourceID, "", `{"tdsElementList":[{"name":"A","description":"known"},{"name":"B","error":{"code":5,"message":"NOT_FOUND"}}]}`, 200, false, true, 2},
		{"workcopy", copyID, "missing", `{"error":{"code":"404065","summary":"missing"}}`, 404, true, false, 0},
		{"workcopy", copyID, "missing", `{"error":{"code":"404004","summary":"missing datasource"}}`, 404, false, true, 0},
		{"workcopy", copyID, "missing", `{"tdsElementList":[]}`, 200, false, true, 0},
		{"workcopy", copyID, "A", `{"tdsElementList":[{"name":"a","description":"wrong"}]}`, 200, false, true, 0},
		{"workcopy", copyID, "", `{}`, 200, false, true, 0},
	} {
		client := tdsFixture(t, "3.30", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" || r.URL.Path != tdsBase+tc.kind+"/"+tc.id || r.URL.Query().Get("elementType") != "FieldDescription" || r.URL.Query().Get("name") != tc.name || r.URL.Query().Has("name") != (tc.name != "") {
				t.Errorf("unexpected %s %s", r.Method, r.URL)
			}
			w.Header().Set("X-Request-Id", "read-1")
			w.WriteHeader(tc.status)
			io.WriteString(w, tc.body)
		})
		read, err := client.ReadTDSElements(t.Context(), tc.kind, tc.id, "FieldDescription", tc.name)
		if (err != nil) != tc.wantErr || read.Absent != tc.absent || len(read.Elements) != tc.rows || read.TableauRequestID != "read-1" {
			t.Errorf("body %s = %#v %v", tc.body, read, err)
		}
		if tc.rows == 2 && (read.Elements[0].Description == nil || *read.Elements[0].Description != "known" || read.Elements[1].Error.Code != 5) {
			t.Errorf("lost partial rows: %#v", read)
		}
	}
}

func TestTDSEditElementWire(t *testing.T) {
	desc := "new"
	caption := "Display"
	for _, tc := range []struct {
		verb, method string
		status       int
		element      value.TDSElement
		body         string
	}{
		{"add", "POST", 201, value.TDSElement{ElementType: "TableDescription", Name: "Orders", Description: &desc}, `{"elementType":"TableDescription","name":"Orders","description":"new"}`},
		{"update", "PUT", 200, value.TDSElement{ElementType: "Column", Name: "Sales", Column: &value.TDSColumn{Caption: &caption}}, `{"elementType":"Column","name":"Sales","column":{"caption":"Display"}}`},
		{"delete", "DELETE", 200, value.TDSElement{ElementType: "FieldDescription", Name: "Sales"}, `{"elementType":"FieldDescription","name":"Sales"}`},
	} {
		client := tdsFixture(t, "3.30", func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			if r.Method != tc.method || r.URL.Path != tdsBase+"workcopy/"+copyID || r.Header.Get("Content-Type") != "application/json" || !sameJSON(string(body), `{"tdsElementList":[`+tc.body+`]}`) {
				t.Errorf("unexpected %s %s %s", r.Method, r.URL, body)
			}
			w.WriteHeader(tc.status)
			io.WriteString(w, "{}")
		})
		receipt, err := client.EditTDSElement(t.Context(), copyID, tc.verb, tc.element)
		if err != nil || receipt.Status != "acknowledged" {
			t.Errorf("%#v %v", receipt, err)
		}
	}
}

func TestTDSEditFailureIsNotSessionRejection(t *testing.T) {
	client := tdsFixture(t, "3.30", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "denial")
		w.WriteHeader(401)
		io.WriteString(w, `{"tdsElementResult":[{"elementType":"Column","name":"Sales","status":{"code":401,"message":"denied token"}}]}`)
	})
	got, err := client.EditTDSElement(t.Context(), copyID, "delete", value.TDSElement{ElementType: "Column", Name: "Sales"})
	if err == nil || errors.Is(err, errTestSessionRenewal) || got.Status != "rejected" || got.Failure == nil || got.Failure.Code != 401 || strings.Contains(got.Failure.Message, "token") || got.TableauRequestID != "denial" {
		t.Errorf("%#v %v", got, err)
	}
}

func TestTDSPreventsInvalidRequestsAndRetainsUnknownReceipts(t *testing.T) {
	for _, version := range []string{"3.6", "3.29"} {
		client := tdsFixture(t, version, func(http.ResponseWriter, *http.Request) { t.Error("unsupported request sent") })
		if got, err := client.CreateWorkCopy(t.Context(), sourceID); err == nil || got.Status != "not_attempted" {
			t.Fatal(got, err)
		}
	}
	client := tdsFixture(t, "3.30", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		io.WriteString(w, `{"tdsWorkCopy":{"workCopyLuid":"`+copyID+`","datasourceLuid":"`+sourceID+`"}}`)
	})
	if got, err := client.CreateWorkCopy(t.Context(), "bad"); err == nil || got.Status != "not_attempted" {
		t.Fatal(got, err)
	}
	got, err := client.CreateWorkCopy(t.Context(), sourceID)
	if err == nil || got.Status != "unknown" || got.WorkCopyLUID != copyID || got.DatasourceLUID != sourceID {
		t.Fatal(got, err)
	}
}

func TestTDSRedirectAndLostResponseDoNotRepeatCheckout(t *testing.T) {
	for _, mode := range []string{"redirect", "disconnect"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			c := tdsFixture(t, "3.30", func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls != 1 {
					w.WriteHeader(500)
					return
				}
				if mode == "redirect" {
					http.Redirect(w, r, "/redirected", http.StatusTemporaryRedirect)
					return
				}
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				conn.Close()
			})
			got, err := c.CreateWorkCopy(t.Context(), sourceID)
			if err == nil || got.Status != "unknown" || calls != 1 {
				t.Errorf("calls %d, receipt %+v, err %v", calls, got, err)
			}
		})
	}
}

func TestTDSEditPreservesUncertainServerFailure(t *testing.T) {
	client := tdsFixture(t, "3.30", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		io.WriteString(w, `{"tdsElementResult":[{"elementType":"Column","name":"Sales","status":{"code":503,"message":"unavailable"}}]}`)
	})
	got, err := client.EditTDSElement(t.Context(), copyID, "delete", value.TDSElement{ElementType: "Column", Name: "Sales"})
	if err == nil || got.Status != "unknown" || got.Failure == nil || got.Failure.Code != 503 {
		t.Fatal(got, err)
	}
}

func TestTDSCredentialBearingMetadataCannotBecomeAnEditInput(t *testing.T) {
	for _, row := range []string{
		`{"name":"token","description":"other"}`, `{"name":"Sales","description":"the token"}`,
	} {
		client := tdsFixture(t, "3.30", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Request-Id", "id-token")
			io.WriteString(w, `{"tdsElementList":[`+row+`]}`)
		})
		got, err := client.ReadTDSElements(t.Context(), "workcopy", copyID, "FieldDescription", "")
		if err == nil || len(got.Elements) != 0 || strings.Contains(got.TableauRequestID, "token") || strings.Contains(err.Error(), "token") {
			t.Fatalf("sensitive TDS read was accepted or exposed")
		}
	}
}
