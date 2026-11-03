package config

import "testing"

func TestIsTableauCloudUsesDocumentedAPIHost(t *testing.T) {
	for _, tc := range []struct {
		server string
		cloud  bool
	}{
		{"https://prod-test.online.tableau.com", true},
		{"https://PROD-TEST.ONLINE.TABLEAU.COM:443/", true},
		{"https://online.tableau.com", true},
		{"https://tableau.example.test", false},
		{"https://online.tableau.com.example.test", false},
		{"https://example.test/online.tableau.com", false},
		{"https://online.tableau.com@example.test", false},
		{"http://prod-test.online.tableau.com", false},
		{":invalid", false},
	} {
		t.Run(tc.server, func(t *testing.T) {
			if got := IsTableauCloud(tc.server); got != tc.cloud {
				t.Fatalf("IsTableauCloud(%q)=%t, want %t", tc.server, got, tc.cloud)
			}
		})
	}
}
