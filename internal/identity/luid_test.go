package identity

import "testing"

func TestValidateLUIDShape(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "opaque token", value: "owner-1", valid: true},
		{name: "trimmed token", value: " owner-1 ", valid: true},
		{name: "empty", value: "", valid: false},
		{name: "whitespace", value: "owner 1", valid: false},
		{name: "control", value: "owner\n1", valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateLUIDShape("owner", test.value)
			if (err == nil) != test.valid {
				t.Fatalf("error = %v, valid = %t", err, test.valid)
			}
		})
	}
}
