package datasource

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

type tagFixture struct {
	observations [][]string
	writes       []string
	receipt      value.TDSReceipt
	err          error
}

func (f *tagFixture) ReadDatasourceTags(context.Context, string) ([]string, error) {
	if len(f.observations) == 0 {
		return nil, errors.New("read unavailable")
	}
	v := f.observations[0]
	f.observations = f.observations[1:]
	return v, nil
}
func (f *tagFixture) AddDatasourceTags(_ context.Context, _ string, labels []string) (value.TDSReceipt, error) {
	f.writes = append(f.writes, labels...)
	return f.receipt, f.err
}
func (f *tagFixture) DeleteDatasourceTag(_ context.Context, _, label string) (value.TDSReceipt, error) {
	f.writes = append(f.writes, label)
	return f.receipt, f.err
}

func TestPublishedTagMembership(t *testing.T) {
	const id = "datasource-1"
	for _, tc := range []struct {
		name         string
		add, preview bool
		observations [][]string
		receipt      string
		want         string
		writes       int
		failed       bool
	}{
		{"preview", true, true, [][]string{{"old"}}, "", "preview", 0, false},
		{"already current", true, false, [][]string{{"sales"}}, "", "already_current", 0, false},
		{"add", true, false, [][]string{{"other"}, {"different"}, {"sales", "different"}}, "acknowledged", "verified", 1, false},
		{"remove", false, false, [][]string{{"sales"}, {"sales", "new"}, {"new"}}, "acknowledged", "verified", 1, false},
		{"concurrent membership", false, false, [][]string{{"sales"}, {}}, "", "stale", 0, true},
		{"lost response despite observed effect", true, false, [][]string{{}, {}, {"sales"}}, "unknown", "unknown", 1, true},
		{"refused", true, false, [][]string{{}, {}}, "rejected", "rejected", 1, true},
		{"lost readback", true, false, [][]string{{}, {}}, "acknowledged", "acknowledged_unverified", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &tagFixture{observations: tc.observations, receipt: value.TDSReceipt{Status: tc.receipt, TableauRequestID: "request"}}
			if tc.receipt == "unknown" || tc.receipt == "rejected" {
				f.err = errors.New("submit failed")
			}
			out, err := ChangeDatasourceTag(t.Context(), f, TagInput{DatasourceLUID: id, Environment: "dev", Tag: "sales", Add: tc.add}, tc.preview)
			if (err != nil) != tc.failed || out.Status != tc.want || len(f.writes) != tc.writes {
				t.Fatalf("out=%+v writes=%v err=%v", out, f.writes, err)
			}
			if out.PresentBefore == nil || *out.PresentBefore != slices.Contains(tc.observations[0], "sales") {
				t.Errorf("lost observed membership: %+v", out)
			}
			if tc.name == "lost response despite observed effect" && (out.Observed == nil || !*out.Observed || out.TableauRequestID != "request") {
				t.Fatal(out)
			}
		})
	}
}
