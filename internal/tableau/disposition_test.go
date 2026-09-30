package tableau

import "testing"

func TestDispositionFilename(t *testing.T) {
	tests := []struct {
		name        string
		disposition string
		want        string
	}{
		{name: "missing", disposition: "", want: ""},
		{name: "no filename", disposition: `name="tableau_workbook"`, want: ""},
		{name: "plain", disposition: `name="tableau_workbook"; filename="Finance.twbx"`, want: "Finance.twbx"},
		{name: "attachment type", disposition: `attachment; filename="Daily.tflx"`, want: "Daily.tflx"},
		{name: "form encoded spaces and ampersand", disposition: `name="tableau_datasource"; filename="Global+Sales+%26+Pipeline+Final.tdsx"`, want: "Global Sales & Pipeline Final.tdsx"},
		{name: "form encoded pipe", disposition: `name="tableau_datasource"; filename="Food+Service+-+Sales+%7C+Budget.tdsx"`, want: "Food Service - Sales | Budget.tdsx"},
		{name: "form encoded plus sign", disposition: `name="tableau_workbook"; filename="C%2B%2B+Metrics.twb"`, want: "C++ Metrics.twb"},
		{name: "form encoded non-ASCII", disposition: `name="tableau_workbook"; filename="Ventas+%C3%91o%C3%B1o.twbx"`, want: "Ventas Ñoño.twbx"},
		{name: "unquoted token", disposition: `attachment; filename=Sales.tds`, want: "Sales.tds"},
		{name: "malformed value falls back to the first filename parameter", disposition: `name="tableau_workbook"; filename=Bound Book.twbx`, want: "Bound Book.twbx"},
		{name: "extended filename is already decoded", disposition: `attachment; filename*=UTF-8''Ventas%20%C3%91o%C3%B1o+2.twbx`, want: "Ventas Ñoño+2.twbx"},
		{name: "extended filename wins over plain", disposition: `attachment; filename="fallback.twbx"; filename*=UTF-8''Real%20Name.twbx`, want: "Real Name.twbx"},
		{name: "invalid percent sequence is kept as sent", disposition: `name="tableau_workbook"; filename="Growth+100%.twbx"`, want: "Growth+100%.twbx"},
		{name: "non-UTF-8 decoding is kept as sent", disposition: `name="tableau_workbook"; filename="Caf%E9.twbx"`, want: "Caf%E9.twbx"},
		{name: "decoded slash is kept as sent", disposition: `name="tableau_datasource"; filename="Sales%2FBudget.tdsx"`, want: "Sales%2FBudget.tdsx"},
		{name: "decoded backslash is kept as sent", disposition: `name="tableau_datasource"; filename="Sales%5CBudget.tdsx"`, want: "Sales%5CBudget.tdsx"},
		{name: "raw traversal is returned for caller validation", disposition: `name="tableau_datasource"; filename="../Sales.tdsx"`, want: "../Sales.tdsx"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := DispositionFilename(test.disposition); got != test.want {
				t.Fatalf("DispositionFilename(%q) = %q, want %q", test.disposition, got, test.want)
			}
		})
	}
}

func TestValidFilename(t *testing.T) {
	for _, filename := range []string{"Sales.tdsx", "Global Sales & Pipeline.tdsx", "Ventas Ñoño.twbx"} {
		if !ValidFilename(filename) {
			t.Errorf("ValidFilename(%q) = false", filename)
		}
	}
	for _, filename := range []string{"", ".", "..", "../Sales.tdsx", "dir/Sales.tdsx", `dir\Sales.tdsx`, "/Sales.tdsx"} {
		if ValidFilename(filename) {
			t.Errorf("ValidFilename(%q) = true", filename)
		}
	}
}
