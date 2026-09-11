package metadataassets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestNestedDatasourceCoveragePreservesConfirmedRecords(t *testing.T) {
	for _, kind := range []string{"database", "table", "field", "column"} {
		for _, terminal := range []string{"short", "changed-total", "duplicate"} {
			t.Run(kind+"/"+terminal, func(t *testing.T) {
				calls := 0
				client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
					var request struct {
						Query     string
						Variables map[string]any
					}
					if e := json.NewDecoder(r.Body).Decode(&request); e != nil {
						t.Error(e)
					}
					if kind == "table" && strings.Contains(request.Query, "items:upstreamDatabasesConnection") {
						fmt.Fprint(w, `{"data":{"parents":{"nodes":[{"luid":"ds-1","items":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`)
						return
					}
					calls++
					row := `{"id":"asset-meta","luid":"asset-1","name":"Asset"}`
					if kind == "field" {
						row = `{"id":"field-meta","name":"Sales","fullyQualifiedName":"[Sales]","description":"Meaning","descriptionInherited":[],"upstreamColumnsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}`
					}
					nodes := "[" + row + "]"
					total, info := 2, `{"hasNextPage":true,"endCursor":"next"}`
					if calls == 2 {
						info = `{"hasNextPage":false}`
						switch terminal {
						case "short":
							nodes = "[]"
						case "changed-total":
							total = 1
							nodes = "[]"
						}
					}
					page := fmt.Sprintf(`{"totalCount":%d,"pageInfo":%s,"nodes":%s}`, total, info, nodes)
					if kind == "column" {
						page = `{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"field-meta","name":"Sales","fullyQualifiedName":"[Sales]","description":"Meaning","descriptionInherited":[],"upstreamColumnsConnection":` + page + `}]}`
					}
					fmt.Fprintf(w, `{"data":{"parents":{"nodes":[{"id":"ds-meta","luid":"ds-1","name":"Sales","items":%s}]}}}`, page)
				})
				if kind == "database" || kind == "table" {
					out, e := client.DatasourceUpstream(context.Background(), "ds-1")
					if e == nil || out.Complete || len(out.Databases)+len(out.Tables) != 1 || calls != 2 {
						t.Fatalf("%+v calls=%d error=%v", out, calls, e)
					}
				} else {
					out, e := client.DatasourceFieldDescriptions(context.Background(), "ds-1")
					if e == nil || out.Complete || len(out.Fields) != 1 || calls != 2 {
						t.Fatalf("%+v calls=%d error=%v", out, calls, e)
					}
					if kind == "column" && len(out.Fields[0].UpstreamColumns) != 1 {
						t.Fatalf("lost confirmed columns: %+v", out)
					}
				}
			})
		}
	}
}
