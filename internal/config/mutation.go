package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// SiteMutation records consent for one server and exact site content URL.
type SiteMutation struct {
	ServerURL      string `yaml:"server_url" json:"server_url"`
	SiteContentURL string `yaml:"site_content_url" json:"site_content_url"`
	Enabled        bool   `yaml:"enabled" json:"enabled"`
}

// CanonicalMutationServer normalizes equivalent Tableau server spellings.
func CanonicalMutationServer(server string) (string, error) {
	if err := validateServerURL(server); err != nil {
		return "", err
	}
	u, err := url.Parse(server)
	if err != nil {
		return "", err
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", fmt.Errorf("server hostname must not be empty")
	}
	if port := u.Port(); port != "" && port != "443" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	u.Host = host
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = strings.TrimRight(u.RawPath, "/")
	return u.String(), nil
}

func validateSiteMutations(settings []SiteMutation) error {
	_, err := validatedSiteMutationIndex(settings)
	return err
}

func validatedSiteMutationIndex(settings []SiteMutation) (map[[2]string]int, error) {
	index := make(map[[2]string]int, len(settings))
	for i, setting := range settings {
		server, err := CanonicalMutationServer(setting.ServerURL)
		if err != nil {
			return nil, fmt.Errorf("site mutation server: %w", err)
		}
		key := [2]string{server, setting.SiteContentURL}
		if _, exists := index[key]; exists {
			return nil, fmt.Errorf("duplicate site mutation setting for server %q and site %q", server, setting.SiteContentURL)
		}
		index[key] = i
	}
	return index, nil
}

// MutationSetting returns only consent matching the selected server and site.
func (c Config) MutationSetting(environment Environment) (*bool, error) {
	server, err := CanonicalMutationServer(environment.URL)
	if err != nil {
		return nil, err
	}
	index, err := validatedSiteMutationIndex(c.SiteMutations)
	if err != nil {
		return nil, err
	}
	if i, ok := index[[2]string{server, environment.SiteContentURL}]; ok {
		return new(c.SiteMutations[i].Enabled), nil
	}
	return nil, nil
}

// SetMutationSetting saves consent for one resolved site without changing other sites.
func (c *Config) SetMutationSetting(environment Environment, enabled bool) error {
	server, err := CanonicalMutationServer(environment.URL)
	if err != nil {
		return err
	}
	index, err := validatedSiteMutationIndex(c.SiteMutations)
	if err != nil {
		return err
	}
	if i, ok := index[[2]string{server, environment.SiteContentURL}]; ok {
		c.SiteMutations[i].Enabled = enabled
		c.SiteMutations[i].ServerURL = server
		return nil
	}
	c.SiteMutations = append(c.SiteMutations, SiteMutation{ServerURL: server, SiteContentURL: environment.SiteContentURL, Enabled: enabled})
	return nil
}
