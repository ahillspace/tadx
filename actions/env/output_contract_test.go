package env_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/ahillspace/tadx/actions/env"
)

func TestFullOutputsMatchTheirCanonicalRecords(t *testing.T) {
	profileRecord := env.Profile{Alias: "dev", ServerURL: "https://example.test"}
	cases := []struct {
		name   string
		output interface{ FullOutput() any }
	}{
		{"list", env.ListOutput{Profiles: []env.Profile{profileRecord}, Help: []string{}}},
		{"get", env.GetOutput{Profile: profileRecord, Help: []string{}}},
		{"add", env.AddOutput{Status: "added", Profile: env.AddProfile{Alias: "dev", ServerURL: "https://example.test"}, Help: []string{}}},
		{"update", env.UpdateOutput{Status: "updated", Profile: env.UpdateProfile{Alias: "dev", ServerURL: "https://example.test"}, ChangedFields: []string{"server_url"}, Help: []string{}}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			full, err := json.Marshal(test.output.FullOutput())
			if err != nil {
				t.Fatal(err)
			}
			canonical, err := json.Marshal(test.output)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(full, canonical) {
				t.Fatalf("full output differs from canonical record: %s versus %s", full, canonical)
			}
		})
	}
}
