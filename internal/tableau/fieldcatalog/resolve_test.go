package fieldcatalog_test

import (
	"fmt"
	"testing"

	"github.com/ahillspace/tadx/internal/tableau/fieldcatalog"
)

func TestResolveFieldsPreservesOrderAndRepeatedSelectors(t *testing.T) {
	fields := []fieldcatalog.Field{
		{ID: "a", Caption: "Alpha", Label: "Alpha"},
		{ID: "b", Caption: "Beta", Label: "Beta (Table)"},
	}
	got, err := fieldcatalog.ResolveFields(fields, []string{"Beta (Table)", "Alpha", "b", "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"b", "a", "b", "a"}
	if len(got) != len(want) {
		t.Fatalf("got %d fields", len(got))
	}
	for i := range want {
		if got[i].ID != want[i] {
			t.Fatalf("field %d = %q; want %q", i, got[i].ID, want[i])
		}
	}
	got, err = fieldcatalog.ResolveFields(fields, []string{"Alpha", "missing"})
	if err == nil || got != nil {
		t.Fatalf("invalid batch returned %#v, %v", got, err)
	}
}

func TestResolveFieldsFullCatalog(t *testing.T) {
	fields := make([]fieldcatalog.Field, 10000)
	selectors := make([]string, len(fields))
	for i := range fields {
		fields[i] = fieldcatalog.Field{ID: fmt.Sprintf("raw_%d", i), Caption: fmt.Sprintf("Caption %d", i)}
		selectors[i] = fields[i].Caption
	}
	got, err := fieldcatalog.ResolveFields(fields, selectors)
	if err != nil || len(got) != len(fields) {
		t.Fatalf("resolved %d fields: %v", len(got), err)
	}
	for i := range fields {
		if got[i].ID != fields[i].ID {
			t.Fatalf("field %d = %q", i, got[i].ID)
		}
	}
}

func TestResolveField(t *testing.T) {
	fields := []fieldcatalog.Field{
		{ID: "revenue_raw", Name: "revenue_raw", Caption: "Revenue", Label: "Revenue"},
		{ID: "order_date", Caption: "Order Date", Label: "Order Date (Orders)"},
		{ID: "Revenue", Caption: "Excluded Revenue", Excluded: true},
		{ID: "amount_a", Caption: "Amount", Label: "Amount (A)"},
		{ID: "amount_b", Caption: "Amount", Label: "Amount (B)"},
		{ID: "duplicate", Caption: "First Duplicate"},
		{ID: "duplicate", Caption: "Second Duplicate"},
		{ID: "cost (net)", Caption: "Net Cost"},
	}
	for _, tt := range []struct {
		selector string
		wantID   string
	}{
		{"revenue_raw", "revenue_raw"},
		{"Revenue", "Revenue"}, // Raw identity wins even when excluded.
		{"Excluded Revenue", "Revenue"},
		{"Order Date", "order_date"},
		{"Order Date (Orders)", "order_date"},
		{"Amount (A)", "amount_a"},
		{"cost (net)", "cost (net)"},
		{"Amount", ""},
		{"duplicate", ""},
		{"First Duplicate", ""},
		{"order date", ""},
		{"[order_date]", ""},
		{" Order Date ", ""},
		{"", ""},
		{"missing", ""},
	} {
		t.Run(tt.selector, func(t *testing.T) {
			got, err := fieldcatalog.ResolveField(fields, tt.selector)
			if tt.wantID == "" {
				if err == nil {
					t.Fatalf("resolved ambiguous or missing selector to %#v", got)
				}
				return
			}
			if err != nil || got.ID != tt.wantID {
				t.Fatalf("got %#v, %v; want %q", got, err, tt.wantID)
			}
		})
	}
	t.Run("identical caption and label are one candidate", func(t *testing.T) {
		got, err := fieldcatalog.ResolveField(fields[:1], "Revenue")
		if err != nil || got.ID != "revenue_raw" {
			t.Fatalf("got %#v, %v", got, err)
		}
	})
}
