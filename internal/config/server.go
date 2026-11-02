package config

import (
	"net/url"
	"strings"
)

// IsTableauCloud recognizes Tableau's documented Cloud REST API hosts.
// Cloud REST authentication uses the pod URL, including when browser content
// is exposed through a custom domain.
func IsTableauCloud(server string) bool {
	u, err := url.Parse(server)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.User != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "online.tableau.com" || strings.HasSuffix(host, ".online.tableau.com")
}
