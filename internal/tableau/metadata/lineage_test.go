package metadata

import (
	"strings"
	"testing"
)

func TestStaticLineageRootQueryContracts(t *testing.T) {
	tests := []struct {
		kind       ResourceKind
		connection string
	}{
		{KindWorkbook, "workbooksConnection"},
		{KindPublishedDatasource, "publishedDatasourcesConnection"},
		{KindFlow, "flowsConnection"},
	}
	for _, test := range tests {
		t.Run(string(test.kind), func(t *testing.T) {
			query, err := StaticLineageRootQuery(test.kind)
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range []string{test.connection, "$rootLuid", "filter: {luid: $rootLuid}", "permissionMode: OBFUSCATE_RESULTS", "totalCount", "pageInfo", "hasNextPage", "endCursor", "id", "luid", "name"} {
				if !strings.Contains(query, required) {
					t.Fatalf("query for %q omitted %q:\n%s", test.kind, required, query)
				}
			}
		})
	}
}

func TestValidateLineageRequestAppliesDefaultsAndBounds(t *testing.T) {
	request, err := ValidateLineageRequest(CaptureRequest{Kind: KindFlow, RESTLUID: " flow-1 "})
	if err != nil {
		t.Fatal(err)
	}
	if request.RESTLUID != "flow-1" || request.Direction != DirectionBoth || request.Depth != DefaultLineageDepth || request.PageSize != LineagePageSize {
		t.Fatalf("request = %#v", request)
	}
	for _, request := range []CaptureRequest{
		{Kind: "sheet", RESTLUID: "x"},
		{Kind: KindWorkbook},
		{Kind: KindWorkbook, RESTLUID: "x", Direction: "sideways"},
		{Kind: KindWorkbook, RESTLUID: "x", Depth: MaxLineageDepth + 1},
		{Kind: KindWorkbook, RESTLUID: "x", PageSize: LineagePageSize + 1},
	} {
		if _, err := ValidateLineageRequest(request); err == nil {
			t.Fatalf("expected validation error for %#v", request)
		}
	}
}
