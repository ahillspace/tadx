// Package config defines TADX's non-secret configuration contract.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	// CurrentVersion is the only supported configuration schema version.
	CurrentVersion = 1
	// WorkspaceConfigName is the visible, versionable workspace configuration file.
	WorkspaceConfigName = "tadx.yaml"
	// AuthTypePAT is the only authentication type supported in V1.
	AuthTypePAT = "pat"
)

// Config is the user-global, non-secret TADX configuration model.
type Config struct {
	Version            int                    `yaml:"version" json:"version"`
	DefaultEnvironment string                 `yaml:"default_environment,omitempty" json:"default_environment,omitempty"`
	DefaultWorkspace   string                 `yaml:"default_workspace,omitempty" json:"default_workspace,omitempty"`
	Environments       map[string]Environment `yaml:"environments,omitempty" json:"environments,omitempty"`
}

// Environment describes one named Tableau target without storing credentials.
type Environment struct {
	Alias            string `yaml:"-" json:"alias,omitempty"`
	URL              string `yaml:"url" json:"url"`
	SiteContentURL   string `yaml:"site_content_url,omitempty" json:"site_content_url,omitempty"`
	Auth             Auth   `yaml:"auth" json:"auth"`
	DefaultWorkspace string `yaml:"default_workspace,omitempty" json:"default_workspace,omitempty"`
}

// Auth contains environment-variable references, never PAT values.
type Auth struct {
	Type         string `yaml:"type" json:"type"`
	PATNameEnv   string `yaml:"pat_name_env,omitempty" json:"pat_name_env,omitempty"`
	PATSecretEnv string `yaml:"pat_secret_env,omitempty" json:"pat_secret_env,omitempty"`
}

// ValidationError reports deterministic configuration violations.
type ValidationError struct {
	Violations []string
}

func (e *ValidationError) Error() string {
	return "invalid configuration: " + strings.Join(e.Violations, "; ")
}

// Validate checks the complete configuration model without reading secrets.
func (c Config) Validate() error {
	var violations []string
	if c.Version != CurrentVersion {
		violations = append(violations, fmt.Sprintf("version must be %d", CurrentVersion))
	}
	if c.DefaultEnvironment != "" {
		if _, ok := c.Environments[c.DefaultEnvironment]; !ok {
			violations = append(violations, fmt.Sprintf("default environment %q does not exist", c.DefaultEnvironment))
		}
	}

	aliases := make([]string, 0, len(c.Environments))
	for alias := range c.Environments {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		environment := c.Environments[alias]
		if strings.TrimSpace(alias) == "" {
			violations = append(violations, "environment alias must not be empty")
		}
		if err := validateServerURL(environment.URL); err != nil {
			violations = append(violations, fmt.Sprintf("environment %q URL: %v", alias, err))
		}
		if environment.Auth.Type != AuthTypePAT {
			violations = append(violations, fmt.Sprintf("environment %q auth type must be %q", alias, AuthTypePAT))
		}
		defaultName, defaultSecret := DefaultPATVariableNames(alias)
		nameVariable := environment.Auth.PATNameEnv
		if nameVariable == "" {
			nameVariable = defaultName
		}
		secretVariable := environment.Auth.PATSecretEnv
		if secretVariable == "" {
			secretVariable = defaultSecret
		}
		if nameVariable == secretVariable {
			violations = append(violations, fmt.Sprintf("environment %q PAT name and secret must use different variables", alias))
		}
	}
	if len(violations) > 0 {
		return &ValidationError{Violations: violations}
	}
	return nil
}

// ResolveEnvironment returns an exact alias match with default PAT references applied.
// An empty alias selects DefaultEnvironment.
func (c Config) ResolveEnvironment(alias string) (Environment, error) {
	if alias == "" {
		alias = c.DefaultEnvironment
	}
	if alias == "" {
		return Environment{}, errors.New("no environment selected and no default environment is configured")
	}
	environment, ok := c.Environments[alias]
	if !ok {
		return Environment{}, fmt.Errorf("environment %q does not exist", alias)
	}
	environment.Alias = alias
	if environment.Auth.Type == "" {
		environment.Auth.Type = AuthTypePAT
	}
	defaultName, defaultSecret := DefaultPATVariableNames(alias)
	if environment.Auth.PATNameEnv == "" {
		environment.Auth.PATNameEnv = defaultName
	}
	if environment.Auth.PATSecretEnv == "" {
		environment.Auth.PATSecretEnv = defaultSecret
	}
	return environment, nil
}

// DefaultPATVariableNames returns the conventional PAT variable references for an alias.
func DefaultPATVariableNames(alias string) (name string, secret string) {
	normalized := normalizeAlias(alias)
	prefix := "TADX_" + normalized + "_PAT_"
	return prefix + "NAME", prefix + "SECRET"
}

func normalizeAlias(alias string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(alias) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func validateServerURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return errors.New("must be a valid absolute URL")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return errors.New("scheme must be http or https")
	}
	if parsed.Host == "" {
		return errors.New("host must not be empty")
	}
	if parsed.User != nil {
		return errors.New("must not contain credentials")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("must not contain a query or fragment")
	}
	return nil
}

// UserConfigPath returns the standard user-global TADX configuration path.
func UserConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user configuration directory: %w", err)
	}
	return UserConfigPathFrom(dir), nil
}

// UserConfigPathFrom builds the TADX configuration path from a platform config directory.
func UserConfigPathFrom(userConfigDir string) string {
	return filepath.Join(userConfigDir, "tadx", "config.yaml")
}
