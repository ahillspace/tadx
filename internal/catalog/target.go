package catalog

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"
)

// NewTargetStore binds every operation, including deferred publication, to one
// immutable server-origin/site namespace. Aliases remain presentation and query
// fields; they never select the physical cache. Legacy unbound stores are not read.
func NewTargetStore(root, serverURL, site string, now func() time.Time) *Store {
	s := NewStore(root, now)
	origin, err := normalizedOrigin(serverURL)
	if err != nil {
		s.targetErr = err
		return s
	}
	identity, _ := json.Marshal([]string{origin, strings.TrimSpace(site)})
	s.relativePath = "catalog/target-" + digestID(identity) + ".sqlite"
	return s
}

// RelativePath is a portable path below the configuration root, without target
// names, credentials, or machine-specific directories.
func (s *Store) RelativePath() string {
	if s.relativePath != "" {
		return s.relativePath
	}
	return databaseRelativePath
}

func normalizedOrigin(value string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || u == nil || u.User != nil || u.Hostname() == "" || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("catalog target requires a valid server origin")
	}
	scheme, host, port := strings.ToLower(u.Scheme), strings.ToLower(u.Hostname()), u.Port()
	if scheme != "https" && scheme != "http" {
		return "", errors.New("catalog target requires an HTTP or HTTPS server origin")
	}
	if scheme == "https" && port == "443" || scheme == "http" && port == "80" {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	// Tableau transport retains a configured reverse-proxy base path. Keep it
	// in the identity so separate mounts at one origin cannot share observations.
	return scheme + "://" + host + strings.TrimRight(u.EscapedPath(), "/"), nil
}
