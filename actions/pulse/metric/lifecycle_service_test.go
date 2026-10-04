package metric

import (
	"strings"
	"testing"
)

func TestMetricLifecycleServiceRejectsInvalidInputsBeforeProviderSetup(t *testing.T) {
	service := New(Ports{})
	tests := []struct {
		name string
		run  func() error
	}{
		{"fork", func() error { _, err := service.ForkPulseMetric(t.Context(), ForkInput{}, true); return err }},
		{"delete", func() error { _, err := service.DeletePulseMetric(t.Context(), DeleteInput{}); return err }},
		{"followers", func() error { _, err := service.ListPulseMetricFollowers(t.Context(), FollowersInput{}); return err }},
		{"follow", func() error { _, err := service.FollowPulseMetric(t.Context(), FollowInput{}, true); return err }},
		{"unfollow", func() error { _, err := service.UnfollowPulseMetric(t.Context(), UnfollowInput{}, true); return err }},
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
