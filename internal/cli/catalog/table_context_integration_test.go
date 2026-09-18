package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	tablelist "github.com/ahillspace/tadx/actions/catalog/table/list"
	catalogresource "github.com/ahillspace/tadx/internal/resources/catalog"
	"github.com/ahillspace/tadx/internal/tableau"
	"github.com/ahillspace/tadx/internal/tableau/metadataassets"
)

type tableListService struct {
	action *tablelist.Action
	last   tablelist.Output
}

func (s *tableListService) ListCatalogTables(ctx context.Context, in tablelist.Input) (tablelist.Output, error) {
	in.Environment, in.Site = "fixture", "site"
	out, err := s.action.Execute(ctx, in)
	s.last = out
	return out, err
}

type tableListRenderer struct{ value any }

func (r *tableListRenderer) Render(v any) error {
	r.value = v
	return nil
}

func TestCLITableListRetainsSchemaContextInCompactReceipt(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/metadata/graphql" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("X-Tableau-Request-Id", "table-list-request")
		_, _ = w.Write([]byte(`{"data":{"items":{"totalCount":1,"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[{"__typename":"DatabaseTable","id":"meta-table","luid":"table","name":"Orders","description":"Order facts","fullName":"warehouse.orders","schema":"sales","database":{"__typename":"Database","id":"meta-db","luid":"db","name":"Warehouse"},"tagsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[]}}]}}}`))
	}))
	defer server.Close()

	adapter := catalogresource.New(metadataassets.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL))
	service := &tableListService{action: tablelist.New(adapter)}
	renderer := &tableListRenderer{}
	command := New(Dependencies{TableLister: service, Renderer: renderer})
	command.SetContext(t.Context())
	command.SetArgs([]string{"table", "list"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}

	encoded, err := json.Marshal(service.last.CompactOutput())
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	items, ok := document["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("compact items = %#v", document["items"])
	}
	item, ok := items[0].(map[string]any)
	if !ok || item["schema"] != "sales" || item["full_name"] != "warehouse.orders" {
		t.Fatalf("compact table context = %#v", item)
	}
	if renderer.value == nil || service.last.RequestID != "table-list-request" {
		t.Fatalf("CLI receipt lost renderer or request context: renderer=%#v output=%+v", renderer.value, service.last)
	}
}
