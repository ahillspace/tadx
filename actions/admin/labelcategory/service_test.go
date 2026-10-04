package labelcategory_test

import (
	"context"
	"testing"

	category "github.com/ahillspace/tadx/actions/admin/labelcategory"
)

type providerSpy struct{ opens int }

func (p *providerSpy) Open(context.Context, string, string, bool) (category.LiveSession, error) {
	p.opens++
	return category.LiveSession{}, nil
}

func TestServiceRejectsInvalidCategoryInputBeforeOpeningProvider(t *testing.T) {
	for name, invoke := range map[string]func(*category.Service) error{
		"list": func(s *category.Service) error {
			_, err := s.ListLabelCategory(t.Context(), category.ListInput{Limit: -1})
			return err
		},
		"inspect": func(s *category.Service) error {
			_, err := s.InspectLabelCategory(t.Context(), category.InspectInput{})
			return err
		},
		"create": func(s *category.Service) error {
			_, err := s.CreateLabelCategory(t.Context(), category.CreateInput{}, true)
			return err
		},
		"update": func(s *category.Service) error {
			_, err := s.UpdateLabelCategory(t.Context(), category.UpdateInput{}, true)
			return err
		},
		"delete": func(s *category.Service) error {
			_, err := s.DeleteLabelCategory(t.Context(), category.DeleteInput{}, true)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			provider := &providerSpy{}
			if err := invoke(category.New(provider)); err == nil || provider.opens != 0 {
				t.Fatalf("error=%v opens=%d", err, provider.opens)
			}
		})
	}
}
