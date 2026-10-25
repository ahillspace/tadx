package labelvalue

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/tableau"
	"github.com/ahillspace/tadx/internal/tableau/metadataassets"
	"github.com/ahillspace/tadx/internal/value"
)

type updateDefinitionStub struct {
	item        value.LabelValue
	writes      int
	readbackErr bool
	oldName     string
}

func (s *updateDefinitionStub) ListLabelValues(context.Context) ([]value.LabelValue, error) {
	if s.item.Name == "" {
		return nil, nil
	}
	return []value.LabelValue{s.item}, nil
}
func (s *updateDefinitionStub) GetLabelValue(context.Context, string) (value.LabelValue, error) {
	if s.writes > 0 && s.readbackErr {
		return value.LabelValue{}, errors.New("readback unavailable")
	}
	return s.item, nil
}
func (s *updateDefinitionStub) SetLabelValue(_ context.Context, oldName string, v value.LabelValue) (value.LabelValue, error) {
	s.writes++
	s.oldName = oldName
	s.item = v
	return v, nil
}

func TestUpdatePreservesExistenceSensitiveVocabularyRules(t *testing.T) {
	for _, tc := range []struct {
		name   string
		before value.LabelValue
		input  UpdateInput
	}{
		{"rename absent", value.LabelValue{}, UpdateInput{Name: "Definition", NewName: new("Renamed")}},
		{"creation incomplete", value.LabelValue{}, UpdateInput{Name: "Definition", Description: new("New")}},
		{"category immutable", value.LabelValue{Name: "Definition", Category: "Original", Description: "Old"}, UpdateInput{Name: "Definition", Category: new("Other")}},
		{"internal immutable", value.LabelValue{Name: "Definition", Category: "Original", Description: "Old", Internal: true}, UpdateInput{Name: "Definition", Description: new("New")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &updateDefinitionStub{item: tc.before}
			if err := ValidateUpdateInput(tc.input); err != nil {
				t.Fatalf("invalid test input: %v", err)
			}
			if _, err := Update(t.Context(), s, s, tc.input, false); err == nil || s.writes != 0 {
				t.Fatalf("error=%v writes=%d", err, s.writes)
			}
		})
	}
}

func TestUpdateRenameKeepsOldIdentityForSubmission(t *testing.T) {
	s := &updateDefinitionStub{item: value.LabelValue{Name: "Definition", Category: "Category", Description: "Old", BuiltIn: true, ElevatedDefault: true}}
	in := UpdateInput{Name: "Definition", NewName: new("Renamed")}
	out, err := Update(t.Context(), s, s, in, false)
	if err != nil || s.oldName != "Definition" || s.writes != 1 || out.Result == nil || out.Result.Item.Name != "Renamed" || !out.Result.Item.BuiltIn || !out.Result.Item.ElevatedDefault {
		t.Fatalf("output=%+v error=%v writer=%+v", out, err, s)
	}
}
func TestUpdatePreviewAndConfirmedReceipt(t *testing.T) {
	for _, preview := range []bool{true, false} {
		s := &updateDefinitionStub{item: value.LabelValue{Name: "Definition", Description: "old", Category: "Category"}, readbackErr: true}
		description := "new"
		in := UpdateInput{Environment: "test", Name: "Definition", Description: &description}
		out, e := Update(t.Context(), s, s, in, preview)
		if preview {
			if e != nil || s.writes != 0 || out.Result != nil {
				t.Fatalf("preview: %+v %v", out, e)
			}
		} else {
			var classified *errs.Error
			if !errors.As(e, &classified) || classified.Outcome != errs.OutcomeConfirmed || out.Result == nil || out.Result.Item.Name != "Definition" || s.writes != 1 {
				t.Fatalf("receipt: %+v %v", out, e)
			}
		}
	}
}
func TestUpdateValidation(t *testing.T) {
	s := &updateDefinitionStub{}
	if e := ValidateUpdateInput(UpdateInput{}); e == nil || s.writes != 0 {
		t.Fatal("invalid input accepted")
	}
}

func TestUpdateSelectsCollectionOnlyForAbsentBaseline(t *testing.T) {
	for _, tc := range []struct {
		name        string
		before      value.LabelValue
		input       UpdateInput
		wantOldName string
	}{
		{"existing description", value.LabelValue{Name: "Definition", Category: "Custom", Description: "Before"}, UpdateInput{Name: "Definition", Description: new("After")}, "Definition"},
		{"absent creation", value.LabelValue{}, UpdateInput{Name: "Definition", Category: new("Custom"), Description: new("After")}, ""},
		{"existing rename", value.LabelValue{Name: "Definition", Category: "Custom", Description: "Before"}, UpdateInput{Name: "Definition", NewName: new("Renamed")}, "Definition"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &updateDefinitionStub{item: tc.before}
			out, err := Update(t.Context(), s, s, tc.input, false)
			if err != nil {
				t.Fatal(err)
			}
			if s.oldName != tc.wantOldName || s.writes != 1 || out.Result == nil || out.Result.Status != "updated" {
				t.Fatalf("route oldName=%q want=%q writes=%d output=%+v", s.oldName, tc.wantOldName, s.writes, out)
			}
		})
	}
}

type labelValueHTTPSession struct{}

func (labelValueHTTPSession) Authorize(r *http.Request) {
	r.Header.Set("X-Tableau-Auth", "test-session")
}
func (labelValueHTTPSession) SiteLUID() string { return "site" }
func (labelValueHTTPSession) UserLUID() string { return "user" }
func (labelValueHTTPSession) String() string   { return "label value HTTP fixture" }

func TestUpdateExistingDescriptionUsesExactRESTResource(t *testing.T) {
	current := value.LabelValue{Name: "Definition", Category: "Custom", Description: "Before"}
	writes, collectionWrites := 0, 0
	body := func() string {
		return fmt.Sprintf(`<labelValue name="%s" category="%s" description="%s" internal="false" elevatedDefault="false" builtIn="false"/>`, current.Name, current.Category, current.Description)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site/labelValues":
			io.WriteString(w, "<tsResponse><labelValueList>"+body()+"</labelValueList></tsResponse>")
		case r.Method == http.MethodPut && r.URL.Path == "/api/3.29/sites/site/labelValues":
			collectionWrites++
			io.WriteString(w, "<tsResponse>"+body()+"</tsResponse>")
		case r.Method == http.MethodPut && r.URL.Path == "/api/3.29/sites/site/labelValues/Definition":
			writes++
			var request struct {
				Value struct {
					Name        string `xml:"name,attr"`
					Category    string `xml:"category,attr"`
					Description string `xml:"description,attr"`
				} `xml:"labelValue"`
			}
			if err := xml.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			if request.Value.Name != "Definition" || request.Value.Category != "Custom" || request.Value.Description != "After" {
				t.Errorf("unexpected exact update payload: %+v", request.Value)
				w.WriteHeader(400)
				return
			}
			current.Description = request.Value.Description
			io.WriteString(w, "<tsResponse>"+body()+"</tsResponse>")
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site/labelValues/Definition":
			io.WriteString(w, "<tsResponse>"+body()+"</tsResponse>")
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	client := metadataassets.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), labelValueHTTPSession{}, server.URL)
	out, err := Update(t.Context(), client, client, UpdateInput{Name: "Definition", Description: new("After")}, false)
	if err != nil || out.Result == nil || out.Result.Status != "updated" || out.Result.Item.Description != "After" || writes != 1 || collectionWrites != 0 {
		t.Fatalf("exact resource update: error=%v output=%+v writes=%d collection=%d", err, out, writes, collectionWrites)
	}
	if !strings.Contains(body(), `description="After"`) {
		t.Fatal("HTTP fixture saved state did not change")
	}
}
