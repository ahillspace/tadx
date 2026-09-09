package list

import (
	"context"
	"testing"
)

func TestRequestedLimitUsesBoundedProviderPages(t *testing.T) {
	for _, limit := range []int{101, 1000, 10000} {
		reader := &allInventoryReader{total: 12000}
		out, err := New(reader).Execute(context.Background(), Input{Limit: limit})
		if err != nil || len(out.Users) != limit || out.Page.Limit != limit || !out.Page.MoreAvailable || reader.calls != (limit+99)/100 {
			t.Fatalf("limit=%d output=%+v calls=%d err=%v", limit, out.Page, reader.calls, err)
		}
	}
	reader := &allInventoryReader{total: 12000}
	if _, err := New(reader).Execute(context.Background(), Input{Limit: 10001}); err == nil || reader.calls != 0 {
		t.Fatal("invalid limit read provider")
	}
}
