package pulse

import (
	"encoding/json"
	"testing"

	pulsemetric "github.com/ahillspace/tadx/actions/pulse/metric"
)

func TestPulseProjectionRequiredEmptyFields(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"follower snapshot", followerSnapshot{Version: 1, MetricLUID: "metric", Subscriptions: []pulsemetric.Subscription{{LUID: "subscription"}}}, `{"version":1,"metric_luid":"metric","subscriptions":[{"luid":"subscription","metric_luid":"","follower_type":"","follower_luid":""}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.value)
			if err != nil || string(got) != tc.want {
				t.Fatalf("JSON = %s, %v; want %s", got, err, tc.want)
			}
		})
	}
}
