package managedpolicy

import (
	"encoding/json"
	"testing"
)

func TestTemplateExactBytes(t *testing.T) {
	for _, test := range []struct{ name, want string }{
		{"read-only", "{\n  \"version\": 1,\n  \"allowed_capabilities\": [\n    \"admin\",\n    \"admin.write\",\n    \"read\",\n    \"write\"\n  ],\n  \"remote_mutations\": false\n}\n"},
		{"read-write-no-admin", "{\n  \"version\": 1,\n  \"allowed_capabilities\": [\n    \"admin\",\n    \"read\",\n    \"write\"\n  ],\n  \"remote_mutations\": true\n}\n"},
		{"superuser", "{\n  \"version\": 1,\n  \"allowed_capabilities\": [\n    \"admin\",\n    \"admin.write\",\n    \"read\",\n    \"write\"\n  ],\n  \"remote_mutations\": true\n}\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			doc, err := Template(test.name, testCatalog())
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.MarshalIndent(doc, "", "  ")
			if err != nil || string(append(data, '\n')) != test.want {
				t.Fatalf("template=%s error=%v", data, err)
			}
			empty, err := Template(test.name, nil)
			if err != nil || empty.AllowedCapabilities == nil || len(empty.AllowedCapabilities) != 0 {
				t.Fatalf("empty template=%+v error=%v", empty, err)
			}
		})
	}
}
