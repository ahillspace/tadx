package datasource_test

import (
	"io"
	"net/http"
	"slices"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

func TestPublishedDatasourceTagRoutes(t *testing.T) {
	const label = "  A/B &?%  "
	for _, tc := range []struct {
		method, escaped, body string
		status                int
		wantErr               bool
	}{
		{"GET", "", `<tsResponse><datasource id="ds-1"><tags><tag label="  A/B &amp;?%  "/></tags></datasource></tsResponse>`, 200, false},
		{"GET", "", `<tsResponse><datasource id="ds-1"><tags/></datasource></tsResponse>`, 200, false},
		{"GET", "", `<tsResponse><datasource id="ds-1"/></tsResponse>`, 200, true},
		{"GET", "", `<tsResponse><datasource id="other"><tags/></datasource></tsResponse>`, 200, true},
		{"PUT", "/tags", `<tsResponse><tags><tag label="  A/B &amp;?%  "/></tags></tsResponse>`, 200, false},
		{"PUT", "/tags", `<tsResponse><tags/></tsResponse>`, 200, true},
		{"DELETE", "/tags/%20%20A%2FB%20&%3F%25%20%20", "", 204, false},
		{"DELETE", "/tags/%20%20A%2FB%20&%3F%25%20%20", "", 200, true},
	} {
		c := tdsFixture(t, "3.12", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != tc.method || r.URL.EscapedPath() != "/api/3.12/sites/site-1/datasources/ds-1"+tc.escaped || r.URL.RawQuery != "" {
				t.Errorf("unexpected request %s %s", r.Method, r.URL)
			}
			if r.Method == "PUT" {
				body, _ := io.ReadAll(r.Body)
				if string(body) != `<tsRequest><tags><tag label="  A/B &amp;?%  "></tag></tags></tsRequest>` {
					t.Errorf("add body %s", body)
				}
			}
			w.Header().Set("X-Request-Id", "tags-1")
			w.WriteHeader(tc.status)
			io.WriteString(w, tc.body)
		})
		if tc.method == "GET" {
			tags, err := c.ReadDatasourceTags(t.Context(), "ds-1")
			if (err != nil) != tc.wantErr {
				t.Fatal(tags, err)
			}
			if tc.body == `<tsResponse><datasource id="ds-1"><tags><tag label="  A/B &amp;?%  "/></tags></datasource></tsResponse>` && !slices.Equal(tags, []string{label}) {
				t.Fatal(tags)
			}
			continue
		}
		var got value.TDSReceipt
		var err error
		if tc.method == "PUT" {
			got, err = c.AddDatasourceTags(t.Context(), "ds-1", []string{label})
		} else {
			got, err = c.DeleteDatasourceTag(t.Context(), "ds-1", label)
		}
		if (err != nil) != tc.wantErr || got.TableauRequestID != "tags-1" || got.Status == "acknowledged" && tc.wantErr || got.Status != "acknowledged" && !tc.wantErr {
			t.Errorf("%+v %v", got, err)
		}
	}
}
