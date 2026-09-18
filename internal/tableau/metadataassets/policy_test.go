package metadataassets

import (
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

func TestManagedVocabularyDenialPrecedesHTTP(t *testing.T) {
	calls := 0
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(http.StatusInternalServerError) })
	denied := errors.New("managed policy denies vocabulary")
	c.checkCapability = func(string) error { return denied }
	for _, call := range []func() error{
		func() error { _, err := c.GetLabelValue(t.Context(), "Warning"); return err },
		func() error { _, err := c.ListLabelValues(t.Context()); return err },
		func() error { _, err := c.GetLabelCategory(t.Context(), "Custom"); return err },
		func() error { _, err := c.ListLabelCategories(t.Context()); return err },
		func() error {
			_, err := c.SetLabelValue(t.Context(), "Warning", value.LabelValue{Name: "Warning", Category: "Custom"})
			return err
		},
		func() error { return c.DeleteLabelValue(t.Context(), "Warning") },
		func() error {
			_, err := c.CreateLabelCategory(t.Context(), value.LabelCategory{Name: "Custom"})
			return err
		},
		func() error {
			_, err := c.UpdateLabelCategory(t.Context(), "Custom", value.LabelCategory{Name: "Custom"})
			return err
		},
		func() error { return c.DeleteLabelCategory(t.Context(), "Custom") },
	} {
		if err := call(); !errors.Is(err, denied) {
			t.Fatalf("error=%v", err)
		}
	}
	if calls != 0 {
		t.Fatalf("forbidden requests=%d", calls)
	}
}

func TestManagedCategoryInspectionDoesNotRequireList(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/3.29/sites/site-1/labelCategories" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `<tsResponse><labelCategoryList><labelCategory name="Custom" description="Meaning"/></labelCategoryList></tsResponse>`)
	})
	c.checkCapability = func(id string) error {
		if id != "admin.label.category.inspect" {
			return errors.New("unexpected capability: " + id)
		}
		return nil
	}
	got, err := c.GetLabelCategory(t.Context(), "Custom")
	if err != nil || got.Name != "Custom" {
		t.Fatalf("got=%#v error=%v", got, err)
	}
}
