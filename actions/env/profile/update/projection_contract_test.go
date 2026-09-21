package update_test

import (
	"encoding/json"
	"testing"

	profileupdate "github.com/ahillspace/tadx/actions/env/profile/update"
)

func TestCompactProfileRetainsTargetAndChangedValues(t *testing.T) {
	out := profileupdate.Output{Status: "updated", Profile: profileupdate.Profile{Alias: "dev", ServerURL: "https://example.invalid", SiteContentURL: "site", PATNameEnv: "UNRELATED_NAME", CacheMaxConcurrency: 0}, ChangedFields: []string{"site_content_url", "cache_max_concurrency"}}
	for _, full := range []bool{false, true} {
		projection := out.CompactOutput()
		if full {
			projection = out.FullOutput()
		}
		encoded, err := json.Marshal(projection)
		if err != nil {
			t.Fatal(err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		profile, ok := decoded["environment"].(map[string]any)
		if !ok {
			t.Fatalf("environment must remain an object: %s", encoded)
		}
		if profile["alias"] != "dev" || profile["site_content_url"] != "site" || profile["cache_max_concurrency"] != float64(0) {
			t.Errorf("changed values or target lost: %s", encoded)
		}
		if !full {
			if _, exists := profile["pat_name_env"]; exists {
				t.Errorf("compact includes unrelated profile: %s", encoded)
			}
		}
	}
}
