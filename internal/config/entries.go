package config

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// EntryViolation names a field and its rule, never the rejected value.
type EntryViolation struct {
	Field      string `json:"field"`
	Rule       string `json:"rule"`
	OtherEntry string `json:"other_entry,omitempty"`
	SecretRisk bool   `json:"secret_risk,omitzero"`
}

// InvalidEntry retains an unusable entry without exposing its original values.
type InvalidEntry struct {
	Violations    []EntryViolation `json:"violations"`
	CredentialRef string           `json:"-" yaml:"-"`
	node          *yaml.Node
	environment   Environment
	workspace     WorkspaceRegistration
}

// EntryWarning supplies compact and expanded configuration diagnostics.
type EntryWarning struct {
	Commands    [][]string       `json:"-"`
	Explanation string           `json:"-"`
	Kind        string           `json:"kind"`
	Name        string           `json:"name"`
	Fields      []string         `json:"fields"`
	Summary     string           `json:"summary"`
	Violations  []EntryViolation `json:"violations,omitempty"`
}

func violationFields(violations []EntryViolation) []string {
	fields := make(map[string]bool)
	for _, violation := range violations {
		fields[violation.Field] = true
	}
	return slices.Sorted(maps.Keys(fields))
}

func violationSummary(kind, name string, violations []EntryViolation) string {
	parts := make([]string, 0, len(violations))
	for _, violation := range violations {
		text := violation.Field + " " + violation.Rule
		if violation.OtherEntry != "" {
			text += fmt.Sprintf(" (also involves %q)", violation.OtherEntry)
		}
		parts = append(parts, text)
	}
	return fmt.Sprintf("%s %q is invalid: %s", kind, name, strings.Join(parts, "; "))
}

// InvalidEnvironmentError preserves entry-specific recovery across action boundaries.
type InvalidEnvironmentError struct {
	Alias      string
	Violations []EntryViolation
}

func (e *InvalidEnvironmentError) Error() string {
	return violationSummary("environment", e.Alias, e.Violations)
}
func (*InvalidEnvironmentError) Retryable() bool { return false }
func (e *InvalidEnvironmentError) CorrectiveAction() string {
	return "Run tadx env list to inspect configured entries, then correct or remove the invalid environment."
}

// CorrectiveCommands keeps logical identities separate from shell formatting.
func (e *InvalidEnvironmentError) CorrectiveCommands() [][]string {
	commands := [][]string{positionalRecovery([]string{"env", "update"}, e.Alias), positionalRecovery([]string{"env", "remove"}, e.Alias)}
	for _, v := range e.Violations {
		if strings.Contains(v.Field, "credential") || strings.HasPrefix(v.Field, "pat_") || v.Field == "auth" || v.Field == "auth.type" {
			commands = append(commands, []string{"auth", "logout", "--environment", e.Alias})
			break
		}
	}
	return commands
}

// CorrectiveExplanation supplies recovery context without executable interpolation.
func (e *InvalidEnvironmentError) CorrectiveExplanation() string {
	explanation := "Supply corrected fields when updating; put update flags before any -- positional delimiter."
	for _, v := range e.Violations {
		if strings.Contains(v.Field, "credential") || strings.HasPrefix(v.Field, "pat_") || v.Field == "auth" || v.Field == "auth.type" {
			explanation += " Remove a stored credential reference with logout before removing an entry with a stored PAT."
			break
		}
	}
	for _, v := range e.Violations {
		if v.SecretRisk {
			explanation += " A secret may be stored in plain text; a PAT saved there should be revoked."
			break
		}
	}
	return explanation
}

func positionalRecovery(args []string, name string) []string {
	if strings.HasPrefix(name, "-") {
		args = append(args, "--")
	}
	return append(args, name)
}

func (*InvalidEnvironmentError) PrerequisiteKind() string       { return "environment" }
func (e *InvalidEnvironmentError) PrerequisiteResource() string { return e.Alias }
func (e *InvalidEnvironmentError) PrerequisiteSummary() string  { return e.Error() }

// InvalidWorkspaceError describes an unusable logical registration.
type InvalidWorkspaceError struct {
	Name       string
	Violations []EntryViolation
}

func (e *InvalidWorkspaceError) Error() string {
	return violationSummary("workspace", e.Name, e.Violations)
}
func (*InvalidWorkspaceError) Retryable() bool { return false }
func (e *InvalidWorkspaceError) CorrectiveAction() string {
	return "Run tadx workspace list to inspect registrations, then repair the invalid workspace registration."
}
func (e *InvalidWorkspaceError) CorrectiveCommands() [][]string {
	return [][]string{
		positionalRecovery([]string{"workspace", "unregister"}, e.Name),
		positionalRecovery([]string{"workspace", "register", "--path", "<path>"}, e.registrationRecoveryName()),
	}
}
func (e *InvalidWorkspaceError) CorrectiveExplanation() string {
	explanation := "Preserve workspace files. Unregister the invalid entry first, then register the existing valid root. Replace <path> with that root."
	if e.registrationRecoveryName() == "<name>" {
		explanation += " Replace <name> with a valid, unused workspace name."
	}
	return explanation
}
func (e *InvalidWorkspaceError) registrationRecoveryName() string {
	if ValidateWorkspaceName(e.Name) != nil {
		return "<name>"
	}
	for _, violation := range e.Violations {
		if violation.Field == "name" {
			return "<name>"
		}
	}
	return e.Name
}
func (*InvalidWorkspaceError) PrerequisiteKind() string       { return "workspace" }
func (e *InvalidWorkspaceError) PrerequisiteResource() string { return e.Name }
func (e *InvalidWorkspaceError) PrerequisiteSummary() string  { return e.Error() }

// EnvironmentAliases includes quarantined aliases, preserving target selection policy.
func (c Config) EnvironmentAliases() []string {
	names := make(map[string]bool, len(c.Environments)+len(c.InvalidEnvironments))
	for name := range c.Environments {
		names[name] = true
	}
	for name := range c.InvalidEnvironments {
		names[name] = true
	}
	return slices.Sorted(maps.Keys(names))
}

// WorkspaceNames includes unusable registrations without resolving their roots.
func (c Config) WorkspaceNames() []string {
	names := make(map[string]bool, len(c.Workspaces)+len(c.InvalidWorkspaces))
	for name := range c.Workspaces {
		names[name] = true
	}
	for name := range c.InvalidWorkspaces {
		names[name] = true
	}
	return slices.Sorted(maps.Keys(names))
}

// EnvironmentForRepair returns known fields without granting operational usability.
func (c Config) EnvironmentForRepair(alias string) (Environment, error) {
	if environment, ok := c.Environments[alias]; ok {
		environment.Alias = alias
		return environment, nil
	}
	if invalid, ok := c.InvalidEnvironments[alias]; ok {
		environment := invalid.environment
		environment.Alias = alias
		return environment, nil
	}
	return Environment{}, c.selectionError(alias)
}

// WorkspaceForRepair returns known registration fields for bounded registry repair.
func (c Config) WorkspaceForRepair(selector string) (string, WorkspaceRegistration, error) {
	if registration, ok := c.Workspaces[selector]; ok {
		return selector, registration, nil
	}
	if invalid, ok := c.InvalidWorkspaces[selector]; ok {
		return selector, invalid.workspace, nil
	}
	var matched string
	for _, name := range c.WorkspaceNames() {
		if !strings.EqualFold(name, selector) {
			continue
		}
		if matched != "" {
			return "", WorkspaceRegistration{}, fmt.Errorf("workspace selector %q is ambiguous; use an exact registered name", selector)
		}
		matched = name
	}
	if matched != "" {
		if registration, ok := c.Workspaces[matched]; ok {
			return matched, registration, nil
		}
		return matched, c.InvalidWorkspaces[matched].workspace, nil
	}
	return "", WorkspaceRegistration{}, fmt.Errorf("workspace %q does not exist", selector)
}

// WorkspaceRegistrations exposes every known root for filesystem protection.
// Consumers must not treat these registrations as operationally valid.
func (c Config) WorkspaceRegistrations() map[string]WorkspaceRegistration {
	registrations := maps.Clone(c.Workspaces)
	if registrations == nil {
		registrations = make(map[string]WorkspaceRegistration)
	}
	for name, invalid := range c.InvalidWorkspaces {
		registrations[name] = invalid.workspace
	}
	return registrations
}

// PatchEnvironment corrects explicit fields while preserving untouched YAML.
// The selected entry must become fully valid before the patch takes effect.
func (c *Config) PatchEnvironment(alias string, fields map[string]any) error {
	if _, err := c.EnvironmentForRepair(alias); err != nil {
		return err
	}
	candidate := cloneConfig(*c)
	node, err := candidate.environmentNode(alias)
	if err != nil {
		return err
	}
	delete(candidate.Environments, alias)
	if candidate.InvalidEnvironments == nil {
		candidate.InvalidEnvironments = make(map[string]InvalidEntry)
	}
	candidate.InvalidEnvironments[alias] = InvalidEntry{node: node}
	for _, path := range slices.Sorted(maps.Keys(fields)) {
		if !slices.Contains(environmentFieldPaths, path) {
			return fmt.Errorf("unsupported environment field %q", path)
		}
		if err := setNodeField(node, path, fields[path]); err != nil {
			if classified, classifyErr := classifyConfiguration(candidate); classifyErr == nil {
				if invalid, exists := classified.InvalidEnvironments[alias]; exists {
					return &InvalidEnvironmentError{Alias: alias, Violations: invalid.Violations}
				}
			}
			return err
		}
	}
	classified, err := classifyConfiguration(candidate)
	if err != nil {
		return err
	}
	if invalid, ok := classified.InvalidEnvironments[alias]; ok {
		return &InvalidEnvironmentError{Alias: alias, Violations: invalid.Violations}
	}
	*c = classified
	return nil
}

// ClearCredentialReference removes only a usable stored credential identity.
// Other invalid fields remain quarantined and unchanged.
func (c *Config) ClearCredentialReference(alias string) error {
	environment, err := c.EnvironmentForRepair(alias)
	if err != nil {
		return err
	}
	if !credentialRefPattern.MatchString(environment.Auth.CredentialRef) {
		return nil
	}
	if invalid, ok := c.InvalidEnvironments[alias]; ok {
		invalid.node = cloneNode(invalid.node)
		removeNodeField(invalid.node, "auth.credential_ref")
		invalid.environment.Auth.CredentialRef = ""
		invalid.CredentialRef = ""
		c.InvalidEnvironments[alias] = invalid
		if c.clearedCredentials == nil {
			c.clearedCredentials = make(map[string]bool)
		}
		c.clearedCredentials[alias] = true
	} else {
		environment.Auth.CredentialRef = ""
		c.Environments[alias] = environment
	}
	return nil
}

// RemoveEnvironment removes either representation after the caller checks its guards.
func (c *Config) RemoveEnvironment(alias string) error {
	if _, err := c.EnvironmentForRepair(alias); err != nil {
		return err
	}
	delete(c.Environments, alias)
	delete(c.InvalidEnvironments, alias)
	return nil
}

// RemoveWorkspace removes only registry state, never filesystem content.
func (c *Config) RemoveWorkspace(selector string) error {
	name, _, err := c.WorkspaceForRepair(selector)
	if err != nil {
		return err
	}
	delete(c.Workspaces, name)
	delete(c.InvalidWorkspaces, name)
	return nil
}

// ConfigurationWarnings supplies one deterministic warning per invalid entry.
func (c Config) ConfigurationWarnings() []EntryWarning {
	var warnings []EntryWarning
	for _, name := range slices.Sorted(maps.Keys(c.InvalidEnvironments)) {
		invalid := c.InvalidEnvironments[name]
		summary := fmt.Sprintf("environment %q is invalid (%s)", name, strings.Join(violationFields(invalid.Violations), ", "))
		for _, v := range invalid.Violations {
			if v.SecretRisk {
				summary += "; a secret may be stored in plain text and should be revoked"
				break
			}
		}
		recovery := &InvalidEnvironmentError{Alias: name, Violations: invalid.Violations}
		warnings = append(warnings, EntryWarning{Kind: "environment", Name: name, Fields: violationFields(invalid.Violations), Summary: summary, Violations: slices.Clone(invalid.Violations), Commands: recovery.CorrectiveCommands(), Explanation: recovery.CorrectiveExplanation()})
	}
	for _, name := range slices.Sorted(maps.Keys(c.InvalidWorkspaces)) {
		invalid := c.InvalidWorkspaces[name]
		recovery := &InvalidWorkspaceError{Name: name, Violations: invalid.Violations}
		warnings = append(warnings, EntryWarning{Kind: "workspace", Name: name, Fields: violationFields(invalid.Violations), Summary: fmt.Sprintf("workspace %q is invalid (%s)", name, strings.Join(violationFields(invalid.Violations), ", ")), Violations: slices.Clone(invalid.Violations), Commands: recovery.CorrectiveCommands(), Explanation: recovery.CorrectiveExplanation()})
	}
	for _, name := range slices.Sorted(maps.Keys(c.unknownFields)) {
		warnings = append(warnings, EntryWarning{Kind: "configuration", Name: name, Fields: []string{name}, Summary: fmt.Sprintf("configuration field %q is unknown and ignored by this TADX build", name)})
	}
	return warnings
}
