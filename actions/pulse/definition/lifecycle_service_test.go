package definition

import (
	"strings"
	"testing"
)

func TestDefinitionLifecycleServiceRejectsInvalidInputsBeforeProviderSetup(t *testing.T) {
	service := New(Ports{})
	tests := []struct {
		name string
		run  func() error
	}{
		{"create", func() error { _, err := service.CreatePulseDefinition(t.Context(), CreateInput{}, true); return err }},
		{"delete", func() error { _, err := service.DeletePulseDefinition(t.Context(), DeleteInput{}); return err }},
		{"publish", func() error { _, err := service.PublishPulseDefinition(t.Context(), PublishInput{}); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.run()
			if err == nil || strings.Contains(err.Error(), "not configured") {
				t.Fatalf("invalid input reached unconfigured provider: %v", err)
			}
		})
	}
}
