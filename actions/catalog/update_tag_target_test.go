package catalog

import (
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

func TestCatalogTagUpdatesPassExactValueTargets(t *testing.T) {
	t.Run("database", func(t *testing.T) {
		fixture := &databaseFixture{}
		input := databaseValid()
		input.AddTags = []string{"test"}
		if _, err := newDatabaseRunner(fixture, fixture).Execute(t.Context(), input, false); err != nil {
			t.Fatal(err)
		}
		if got, want := fixture.lastTagTarget, (value.LabelTarget{Type: "database", LUID: input.ID}); got != want {
			t.Fatalf("tag target = %#v, want %#v", got, want)
		}
	})
	t.Run("table", func(t *testing.T) {
		fixture := &tableFixture{}
		input := tableValid()
		input.AddTags = []string{"test"}
		if _, err := newTableRunner(fixture, fixture).Execute(t.Context(), input, false); err != nil {
			t.Fatal(err)
		}
		if got, want := fixture.lastTagTarget, (value.LabelTarget{Type: "table", LUID: input.ID}); got != want {
			t.Fatalf("tag target = %#v, want %#v", got, want)
		}
	})
	t.Run("column", func(t *testing.T) {
		fixture := &columnFixture{}
		input := columnValid()
		input.AddTags = []string{"test"}
		if _, err := newColumnRunner(fixture, fixture).Execute(t.Context(), input, false); err != nil {
			t.Fatal(err)
		}
		if got, want := fixture.lastTagTarget, (value.LabelTarget{Type: "column", LUID: input.ID}); got != want {
			t.Fatalf("tag target = %#v, want %#v", got, want)
		}
	})
}
