package labelvalue_test

import (
	"context"
	"testing"

	labelvalue "github.com/ahillspace/tadx/actions/admin/labelvalue"
)

type providerSpy struct{ opens int }

func (p *providerSpy) Open(context.Context, string, string, bool) (labelvalue.LiveSession, error) {
	p.opens++
	return labelvalue.LiveSession{}, nil
}

func TestServiceRejectsInvalidValueInputBeforeOpeningProvider(t *testing.T) {
	for name, invoke := range map[string]func(*labelvalue.Service) error{
		"list": func(s *labelvalue.Service) error {
			_, err := s.ListLabelValue(t.Context(), labelvalue.ListInput{Limit: -1})
			return err
		},
		"inspect": func(s *labelvalue.Service) error {
			_, err := s.InspectLabelValue(t.Context(), labelvalue.InspectInput{})
			return err
		},
		"update": func(s *labelvalue.Service) error {
			_, err := s.UpdateLabelValue(t.Context(), labelvalue.UpdateInput{}, true)
			return err
		},
		"delete": func(s *labelvalue.Service) error {
			_, err := s.DeleteLabelValue(t.Context(), labelvalue.DeleteInput{}, true)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			provider := &providerSpy{}
			if err := invoke(labelvalue.New(provider)); err == nil || provider.opens != 0 {
				t.Fatalf("error=%v opens=%d", err, provider.opens)
			}
		})
	}
}
