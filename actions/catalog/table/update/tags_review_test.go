package update

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
)

func TestTagValidationBeforeAnyRemoteRead(t *testing.T) {
	for _, tc := range []struct {
		name, tag string
		valid     bool
	}{
		{"ascii128", strings.Repeat("a", 128), true},
		{"ascii129", strings.Repeat("a", 129), false},
		{"unicode128", strings.Repeat("界", 128), true},
		{"unicode129", strings.Repeat("界", 129), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, preview := range []bool{true, false} {
				f := &fixture{}
				in := valid()
				in.AddTags = []string{tc.tag}
				if tc.valid {
					if err := ValidateInput(in); err != nil {
						t.Fatalf("valid Unicode tag rejected: %v", err)
					}
					continue
				}
				_, err := New(f, f).Execute(context.Background(), in, preview)
				if err == nil || f.reads != 0 || f.writes != 0 {
					t.Fatalf("error=%v reads=%d writes=%d preview=%v", err, f.reads, f.writes, preview)
				}
			}
		})
	}
}

func TestTagRemovalUsesExactSelectorRatherThanAdditionLengthLimit(t *testing.T) {
	in := valid()
	in.RemoveTags = []string{strings.Repeat("界", 129)}
	if err := ValidateInput(in); err != nil {
		t.Fatalf("removal inherited addition limit: %v", err)
	}
	for _, tag := range []string{" sales", "sales ", "sales\n", "sales\x00"} {
		in.RemoveTags = []string{tag}
		if err := ValidateInput(in); err == nil {
			t.Fatalf("invalid removal selector accepted: %q", tag)
		}
	}
}

type incompleteTagWriter struct {
	*fixture
	acknowledged []string
}

func (f *incompleteTagWriter) AddTableTags(context.Context, string, []string) ([]string, error) {
	f.writes++
	return f.acknowledged, nil
}

func TestIncompleteTagAcknowledgmentPreservesPartialResult(t *testing.T) {
	for _, ack := range [][]string{nil, {"sales"}} {
		f := &incompleteTagWriter{fixture: &fixture{}, acknowledged: ack}
		in := valid()
		in.AddTags = []string{"sales", "retail"}
		out, err := New(f, f).Execute(context.Background(), in, false)
		var structured *errs.Error
		if !errors.As(err, &structured) || structured.Phase != errs.PhaseVerification || structured.Outcome != errs.OutcomeUnknown || structured.Resource != "item" || structured.Retryable == nil || *structured.Retryable {
			t.Fatalf("incorrect failure evidence: %#v %v", structured, err)
		}
		if out.Result == nil || out.Result.Identity.LUID != "item" || out.Result.Status != "partial" || out.Result.Failed != "add_tags" || !slices.Equal(out.Result.Completed, []string{"description"}) || !slices.Equal(structured.Completed, out.Result.Completed) {
			t.Fatalf("partial success lost or unverified tags marked completed: %#v", out)
		}
		if f.writes != 2 {
			t.Fatalf("unexpected retry: writes=%d", f.writes)
		}
	}
}
