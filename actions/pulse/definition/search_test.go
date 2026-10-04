package definition

import "testing"

func TestListSearchRejectsMalformedContinuationBeforeReader(t *testing.T) {
	if _, err := ListSearch(t.Context(), nil, ListInput{Environment: "target", Site: "site", Cursor: "not-a-cursor", Limit: 1}); err == nil {
		t.Fatal("malformed cursor reached native reader")
	}
}
