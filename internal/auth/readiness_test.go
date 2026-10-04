package auth

import "testing"

func TestInspectLocalPATReadinessKeepsPartialOverrideAndNeverOpensStore(t *testing.T) {
	for _, tc := range []struct {
		name, secret string
		stored       bool
		want         LocalPATReadiness
	}{
		{name: "name", secret: "secret", stored: true, want: LocalPATReadiness{true, true, "environment", true}},
		{name: "name", stored: true, want: LocalPATReadiness{true, false, "environment", false}},
		{name: "  ", stored: true, want: LocalPATReadiness{false, false, "os_credential_store", true}},
		{want: LocalPATReadiness{Source: "none"}},
	} {
		lookup := LookupEnvFunc(func(key string) (string, bool) {
			if key == "NAME" {
				return tc.name, tc.name != ""
			}
			return tc.secret, tc.secret != ""
		})
		got := InspectLocalPATReadiness("NAME", "SECRET", tc.stored, lookup)
		if got != tc.want {
			t.Fatalf("name=%q secret=%q stored=%v got=%+v want=%+v", tc.name, tc.secret, tc.stored, got, tc.want)
		}
	}
}
