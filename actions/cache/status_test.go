package cache_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	status "github.com/ahillspace/tadx/actions/cache"
	"github.com/ahillspace/tadx/internal/output"
)

type source struct{ result status.StatusObservation }

func (s source) Status(context.Context, status.StatusInput) (status.StatusObservation, error) {
	return s.result, nil
}

func TestStatusActionCompactAndFullOutput(t *testing.T) {
	reader := source{result: status.StatusObservation{ID: "generation-1", Environment: "production", Site: "marketing", GeneratedAt: "2026-09-01T00:00:00Z", Age: "13h0m0s", Complete: true, Stale: true, Source: "tableau-rest", Path: "cache/production.json", Records: 2, Warnings: []string{"cache generation is older than 12 hours"}}}
	for _, test := range []struct {
		name, golden string
		full         bool
	}{{"compact", "testdata/status/compact.toon", false}, {"full", "testdata/status/full.toon", true}} {
		t.Run(test.name, func(t *testing.T) {
			value, err := status.New(cacheTestProvider{status: reader}).ReadCacheStatus(t.Context(), status.StatusInput{Environment: "production", Site: "marketing"})
			if err != nil {
				t.Fatal(err)
			}
			assertStatusGolden(t, test.golden, value, test.full)
		})
	}
}

func assertStatusGolden(t *testing.T, path string, value any, full bool) {
	t.Helper()
	var actual bytes.Buffer
	if err := output.RenderWithOptions(&actual, value, output.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual.Bytes(), expected) {
		t.Fatalf("golden mismatch\nexpected:\n%s\nactual:\n%s", expected, actual.Bytes())
	}
}
