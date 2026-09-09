// Package config defines TADX's non-secret configuration contract.
package config

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ahillspace/tadx/internal/lock"
	"gopkg.in/yaml.v3"
)

const (
	// CurrentVersion is the only supported configuration schema version.
	CurrentVersion = 1
	// WorkspaceConfigName is the visible, versionable workspace configuration file.
	WorkspaceConfigName = "tadx.yaml"
	// AuthTypePAT is the only authentication type supported in V1.
	AuthTypePAT           = "pat"
	maxConfigBytes        = 1024 * 1024
	maxWorkspaces         = 1000
	migrationBackupSuffix = ".pre-workspace-migration-v1.bak"
)

// Config is the user-global, non-secret TADX configuration model.
type Config struct {
	MutationsEnabled   *bool                            `yaml:"mutations_enabled,omitempty" json:"mutations_enabled,omitempty"`
	Version            int                              `yaml:"version" json:"version"`
	DefaultEnvironment string                           `yaml:"default_environment,omitempty" json:"default_environment,omitempty"`
	DefaultWorkspace   string                           `yaml:"default_workspace,omitempty" json:"default_workspace,omitempty"`
	Environments       map[string]Environment           `yaml:"environments,omitempty" json:"environments,omitempty"`
	Workspaces         map[string]WorkspaceRegistration `yaml:"workspaces,omitempty" json:"workspaces,omitempty"`
}

// WorkspaceRegistration maps one logical workspace name to a stable identity
// and its machine-local root.
type WorkspaceRegistration struct {
	ID   string `yaml:"id" json:"id"`
	Path string `yaml:"path" json:"path"`
}

// Environment describes one named Tableau target without storing credentials.
type Environment struct {
	Alias                 string `yaml:"-" json:"alias,omitempty"`
	URL                   string `yaml:"url" json:"url"`
	SiteContentURL        string `yaml:"site_content_url,omitempty" json:"site_content_url,omitempty"`
	APIVersion            string `yaml:"api_version,omitempty" json:"api_version,omitempty"`
	Auth                  Auth   `yaml:"auth" json:"auth"`
	DefaultWorkspace      string `yaml:"default_workspace,omitempty" json:"default_workspace,omitempty"`
	CatalogMaxConcurrency int    `yaml:"catalog_max_concurrency,omitempty" json:"catalog_max_concurrency,omitempty"`
}

// Auth contains credential references, never PAT values.
type Auth struct {
	Type          string `yaml:"type" json:"type"`
	PATNameEnv    string `yaml:"pat_name_env,omitempty" json:"pat_name_env,omitempty"`
	PATSecretEnv  string `yaml:"pat_secret_env,omitempty" json:"pat_secret_env,omitempty"`
	CredentialRef string `yaml:"credential_ref,omitempty" json:"credential_ref,omitempty"`
}

// ValidationError reports deterministic configuration violations.
type ValidationError struct {
	Violations []string
}

var (
	workspaceIDPattern   = regexp.MustCompile(`^ws_[0-9a-f]{32}$`)
	credentialRefPattern = regexp.MustCompile(`^cred_[0-9a-f]{32}$`)
)

func (e *ValidationError) Error() string {
	return "invalid configuration: " + strings.Join(e.Violations, "; ")
}

// Validate checks the complete configuration model without reading secrets.
func (c Config) Validate() error {
	var violations []string
	type variableReference struct {
		alias     string
		defaulted bool
	}
	variableReferences := make(map[string]variableReference)
	credentialReferences := make(map[string]string)
	if c.Version != CurrentVersion {
		violations = append(violations, fmt.Sprintf("version must be %d", CurrentVersion))
	}
	if c.DefaultEnvironment != "" {
		if _, ok := c.Environments[c.DefaultEnvironment]; !ok {
			violations = append(violations, fmt.Sprintf("default environment %q does not exist", c.DefaultEnvironment))
		}
	}
	workspaceNames := make(map[string]string, len(c.Workspaces))
	workspaceIDs := make(map[string]string, len(c.Workspaces))
	workspaceRoots := make(map[string]string, len(c.Workspaces))
	if len(c.Workspaces) > maxWorkspaces {
		violations = append(violations, fmt.Sprintf("workspace registry must not exceed %d entries", maxWorkspaces))
	}
	for name, registration := range c.Workspaces {
		if err := ValidateWorkspaceName(name); err != nil {
			violations = append(violations, fmt.Sprintf("workspace %q name: %v", name, err))
		}
		foldedName := strings.ToLower(name)
		for _, previous := range workspaceNames {
			if strings.EqualFold(previous, name) {
				violations = append(violations, fmt.Sprintf("workspace names %q and %q collide under case-insensitive matching", previous, name))
				break
			}
		}
		workspaceNames[foldedName] = name
		if !workspaceIDPattern.MatchString(registration.ID) {
			violations = append(violations, fmt.Sprintf("workspace %q ID must match ws_<32 lowercase hex>", name))
		} else if previous, exists := workspaceIDs[registration.ID]; exists {
			violations = append(violations, fmt.Sprintf("workspace ID %q is shared by %q and %q", registration.ID, previous, name))
		} else {
			workspaceIDs[registration.ID] = name
		}
		root, err := canonicalWorkspaceRoot(registration.Path)
		if err != nil {
			violations = append(violations, fmt.Sprintf("workspace %q path: %v", name, err))
		} else {
			key := workspaceRootKey(root)
			if previous, exists := workspaceRoots[key]; exists {
				violations = append(violations, fmt.Sprintf("workspace canonical root %q is shared by %q and %q", root, previous, name))
			} else {
				workspaceRoots[key] = name
			}
		}
	}
	if c.DefaultWorkspace != "" {
		if looksLikePath(c.DefaultWorkspace) {
			violations = append(violations, "default workspace must be a logical name, not a path")
		} else if _, _, ok := resolveWorkspaceRegistration(c.Workspaces, c.DefaultWorkspace); !ok {
			violations = append(violations, fmt.Sprintf("default workspace %q does not exist", c.DefaultWorkspace))
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
		if environment.APIVersion != "" && !isAPIVersion(environment.APIVersion) {
			violations = append(violations, fmt.Sprintf("environment %q API version must use major.minor numeric format", alias))
		}
		if environment.CatalogMaxConcurrency < 0 || environment.CatalogMaxConcurrency > 256 {
			violations = append(violations, fmt.Sprintf("environment %q catalog maximum concurrency must be between 1 and 256, or omitted for the default", alias))
		}
		if environment.Auth.Type != AuthTypePAT {
			violations = append(violations, fmt.Sprintf("environment %q auth type must be %q", alias, AuthTypePAT))
		}
		if reference := environment.Auth.CredentialRef; reference != "" {
			if !credentialRefPattern.MatchString(reference) {
				violations = append(violations, fmt.Sprintf("environment %q credential reference must match cred_<32 lowercase hex>", alias))
			} else if previous, exists := credentialReferences[reference]; exists {
				violations = append(violations, fmt.Sprintf("credential reference %q is shared by environments %q and %q", reference, previous, alias))
			} else {
				credentialReferences[reference] = alias
			}
		}
		if environment.DefaultWorkspace != "" {
			if looksLikePath(environment.DefaultWorkspace) {
				violations = append(violations, fmt.Sprintf("environment %q default workspace must be a logical name, not a path", alias))
			} else if _, _, ok := resolveWorkspaceRegistration(c.Workspaces, environment.DefaultWorkspace); !ok {
				violations = append(violations, fmt.Sprintf("environment %q default workspace %q does not exist", alias, environment.DefaultWorkspace))
			}
		}
		defaultName, defaultSecret := DefaultPATVariableNames(alias)
		nameVariable := environment.Auth.PATNameEnv
		nameDefaulted := nameVariable == ""
		if nameVariable == "" {
			nameVariable = defaultName
		}
		secretVariable := environment.Auth.PATSecretEnv
		secretDefaulted := secretVariable == ""
		if secretVariable == "" {
			secretVariable = defaultSecret
		}
		if strings.EqualFold(nameVariable, secretVariable) {
			violations = append(violations, fmt.Sprintf("environment %q PAT name and secret must use different variables", alias))
		}
		for _, reference := range []struct {
			variable  string
			defaulted bool
		}{
			{variable: nameVariable, defaulted: nameDefaulted},
			{variable: secretVariable, defaulted: secretDefaulted},
		} {
			identity := strings.ToUpper(reference.variable)
			previous, exists := variableReferences[identity]
			if exists && previous.alias != alias && (previous.defaulted || reference.defaulted) {
				violations = append(violations, fmt.Sprintf(
					"environment %q PAT variable %q conflicts with environment %q because at least one reference uses the default",
					alias, reference.variable, previous.alias,
				))
				continue
			}
			if !exists {
				variableReferences[identity] = variableReference{alias: alias, defaulted: reference.defaulted}
			}
		}
	}
	if len(violations) > 0 {
		return &ValidationError{Violations: violations}
	}
	return nil
}

// ValidateWorkspaceName validates one user-facing logical workspace name.
func ValidateWorkspaceName(name string) error {
	if name == "" || strings.TrimSpace(name) != name {
		return errors.New("must not be empty or have surrounding whitespace")
	}
	if len(name) > 128 {
		return errors.New("must not exceed 128 bytes")
	}
	if name == "." || name == ".." || looksLikePath(name) {
		return errors.New("must be a logical name, not a path")
	}
	if strings.ContainsAny(name, `<>:"|?*`) {
		return errors.New("must not contain characters that are invalid in portable file names")
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return errors.New("must not end with a dot or space")
	}
	stem := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if isReservedWorkspaceStem(stem) {
		return errors.New("must not use a reserved device name")
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return errors.New("must not contain control characters")
		}
	}
	return nil
}

func isReservedWorkspaceStem(stem string) bool {
	switch stem {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$", "CLOCK$",
		"COM¹", "COM²", "COM³", "LPT¹", "LPT²", "LPT³":
		return true
	}
	if len(stem) == 4 && stem[3] >= '1' && stem[3] <= '9' {
		return strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")
	}
	return false
}

// ResolveWorkspace returns an exact case-insensitive logical workspace match.
// An empty selector uses DefaultWorkspace.
func (c Config) ResolveWorkspace(selector string) (string, WorkspaceRegistration, error) {
	if selector == "" {
		selector = c.DefaultWorkspace
	}
	if selector == "" {
		return "", WorkspaceRegistration{}, errors.New("no workspace selected and no default workspace is configured")
	}
	name, registration, ok := resolveWorkspaceRegistration(c.Workspaces, selector)
	if !ok {
		return "", WorkspaceRegistration{}, fmt.Errorf("workspace %q does not exist", selector)
	}
	return name, registration, nil
}

func resolveWorkspaceRegistration(workspaces map[string]WorkspaceRegistration, selector string) (string, WorkspaceRegistration, bool) {
	for name, registration := range workspaces {
		if strings.EqualFold(name, selector) {
			return name, registration, true
		}
	}
	return "", WorkspaceRegistration{}, false
}

func looksLikePath(value string) bool {
	return strings.ContainsAny(value, `/\`) || filepath.IsAbs(value) || filepath.VolumeName(value) != ""
}

func canonicalWorkspaceRoot(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("must not be empty")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}

// ResolveEnvironment returns an exact alias match with default PAT references applied.
// An empty alias selects DefaultEnvironment.
func (c Config) ResolveEnvironment(alias string) (Environment, error) {
	if alias == "" {
		alias = c.DefaultEnvironment
		if alias == "" && len(c.Environments) == 1 {
			for name := range c.Environments {
				alias = name
			}
		}
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
	if environment.APIVersion == "" {
		environment.APIVersion = "3.29"
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

// Load reads and validates the non-secret user configuration.
// Load reads and validates the user configuration, applying the legacy
// workspace-defaults migration in memory. Load never writes: persisting a
// migration is exclusively Update's responsibility, performed under the
// interprocess lock, so a read on one code path cannot race a concurrent locked
// Update and clobber its write.
func Load(path string) (Config, error) {
	configuration, _, _, err := load(path)
	return configuration, err
}

// load is the pure read used by both Load and Update. It returns the decoded
// configuration, the exact bytes read from disk, and whether the in-memory
// legacy migration changed anything. It performs no writes.
func load(path string) (Config, []byte, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, nil, false, fmt.Errorf("read configuration: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return Config{}, nil, false, fmt.Errorf("inspect configuration: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxConfigBytes {
		file.Close()
		return Config{}, nil, false, fmt.Errorf("configuration must be a regular file no larger than %d bytes", maxConfigBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxConfigBytes+1))
	if err != nil {
		file.Close()
		return Config{}, nil, false, fmt.Errorf("read configuration: %w", err)
	}
	if err := file.Close(); err != nil {
		return Config{}, nil, false, fmt.Errorf("close configuration: %w", err)
	}
	if len(data) > maxConfigBytes {
		return Config{}, nil, false, fmt.Errorf("configuration exceeds %d bytes", maxConfigBytes)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var configuration Config
	if err := decoder.Decode(&configuration); err != nil {
		return Config{}, nil, false, fmt.Errorf("decode configuration: %w", err)
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return Config{}, nil, false, fmt.Errorf("decode configuration: %w", err)
		}
		return Config{}, nil, false, errors.New("decode configuration: multiple YAML documents are not supported")
	}
	configuration, migrated, err := migrateLegacyWorkspaceDefaults(configuration)
	if err != nil {
		return Config{}, nil, false, err
	}
	if err := configuration.Validate(); err != nil {
		return Config{}, nil, false, err
	}
	return configuration, data, migrated, nil
}

func preserveWorkspaceMigrationBackup(path string, original []byte) error {
	backupPath := path + migrationBackupSuffix
	backup, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		if _, writeErr := backup.Write(original); writeErr != nil {
			backup.Close()
			_ = os.Remove(backupPath)
			return fmt.Errorf("write workspace migration backup: %w", writeErr)
		}
		if syncErr := backup.Sync(); syncErr != nil {
			backup.Close()
			_ = os.Remove(backupPath)
			return fmt.Errorf("sync workspace migration backup: %w", syncErr)
		}
		if closeErr := backup.Close(); closeErr != nil {
			_ = os.Remove(backupPath)
			return fmt.Errorf("close workspace migration backup: %w", closeErr)
		}
		return nil
	}
	if !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create workspace migration backup: %w", err)
	}
	existing, readErr := readBoundedConfigurationFile(backupPath)
	if readErr != nil {
		return fmt.Errorf("inspect existing workspace migration backup: %w", readErr)
	}
	if !bytes.Equal(existing, original) {
		return errors.New("existing workspace migration backup differs from the current legacy configuration; preserve or remove that backup after reviewing both files")
	}
	return nil
}

func readBoundedConfigurationFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxConfigBytes {
		file.Close()
		return nil, fmt.Errorf("file must be a regular file no larger than %d bytes", maxConfigBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxConfigBytes+1))
	closeErr := file.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(data) > maxConfigBytes {
		return nil, fmt.Errorf("file exceeds %d bytes", maxConfigBytes)
	}
	return data, nil
}

func migrateLegacyWorkspaceDefaults(configuration Config) (Config, bool, error) {
	type workspaceReference struct {
		label string
		value string
		set   func(string)
	}
	references := make([]workspaceReference, 0, len(configuration.Environments)+1)
	if looksLikePath(configuration.DefaultWorkspace) {
		references = append(references, workspaceReference{
			label: "default workspace",
			value: configuration.DefaultWorkspace,
			set:   func(name string) { configuration.DefaultWorkspace = name },
		})
	}
	aliases := make([]string, 0, len(configuration.Environments))
	for alias := range configuration.Environments {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		environment := configuration.Environments[alias]
		if !looksLikePath(environment.DefaultWorkspace) {
			continue
		}
		currentAlias := alias
		references = append(references, workspaceReference{
			label: fmt.Sprintf("environment %q default workspace", alias),
			value: environment.DefaultWorkspace,
			set: func(name string) {
				updated := configuration.Environments[currentAlias]
				updated.DefaultWorkspace = name
				configuration.Environments[currentAlias] = updated
			},
		})
	}
	if len(references) == 0 {
		return configuration, false, nil
	}

	if configuration.Workspaces == nil {
		configuration.Workspaces = make(map[string]WorkspaceRegistration)
	}
	type registeredRoot struct {
		name string
		root string
	}
	registered := make(map[string]registeredRoot, len(configuration.Workspaces))
	ids := make(map[string]string, len(configuration.Workspaces))
	for name, registration := range configuration.Workspaces {
		root, err := canonicalWorkspaceRoot(registration.Path)
		if err != nil {
			continue
		}
		registered[workspaceRootKey(root)] = registeredRoot{name: name, root: root}
		ids[registration.ID] = name
	}

	for _, reference := range references {
		if !filepath.IsAbs(reference.value) {
			return Config{}, false, fmt.Errorf(
				"migrate legacy %s: path %q is not absolute; register it with workspace create, then set the default workspace to its logical name",
				reference.label,
				reference.value,
			)
		}
		root, err := canonicalWorkspaceRoot(reference.value)
		if err != nil {
			return Config{}, false, fmt.Errorf("migrate legacy %s: resolve path: %w", reference.label, err)
		}
		key := workspaceRootKey(root)
		if existing, ok := registered[key]; ok {
			reference.set(existing.name)
			continue
		}

		name, err := availableLegacyWorkspaceName(configuration.Workspaces, root)
		if err != nil {
			return Config{}, false, fmt.Errorf("migrate legacy %s: %w", reference.label, err)
		}
		id := legacyWorkspaceID(root)
		if previous, exists := ids[id]; exists {
			return Config{}, false, fmt.Errorf(
				"migrate legacy %s: generated workspace identity conflicts with registered workspace %q; register the path with workspace create",
				reference.label,
				previous,
			)
		}
		configuration.Workspaces[name] = WorkspaceRegistration{ID: id, Path: root}
		registered[key] = registeredRoot{name: name, root: root}
		ids[id] = name
		reference.set(name)
	}
	return configuration, true, nil
}

func availableLegacyWorkspaceName(workspaces map[string]WorkspaceRegistration, root string) (string, error) {
	name := filepath.Base(root)
	if err := ValidateWorkspaceName(name); err != nil {
		name = "workspace"
	}
	if _, _, exists := resolveWorkspaceRegistration(workspaces, name); !exists {
		return name, nil
	}
	hash := sha256.Sum256([]byte("tadx-legacy-workspace-name-v1\x00" + workspaceRootKey(root)))
	for hashBytes := 6; hashBytes <= len(hash); hashBytes++ {
		suffix := fmt.Sprintf("-%x", hash[:hashBytes])
		candidate := truncateUTF8(name, 128-len(suffix)) + suffix
		if _, _, exists := resolveWorkspaceRegistration(workspaces, candidate); !exists {
			return candidate, nil
		}
	}
	return "", errors.New("cannot derive a unique logical workspace name; register the path with workspace create")
}

func truncateUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	end := 0
	for index := range value {
		if index > limit {
			break
		}
		end = index
	}
	return value[:end]
}

func legacyWorkspaceID(root string) string {
	hash := sha256.Sum256([]byte("tadx-legacy-workspace-id-v1\x00" + workspaceRootKey(root)))
	return fmt.Sprintf("ws_%x", hash[:16])
}

func workspaceRootKey(root string) string {
	return strings.ToLower(filepath.Clean(root))
}

// Save validates and atomically replaces the non-secret user configuration.
func Save(path string, configuration Config) error {
	if err := configuration.Validate(); err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create configuration directory: %w", err)
	}
	data, err := yaml.Marshal(configuration)
	if err != nil {
		return fmt.Errorf("encode configuration: %w", err)
	}
	if len(data) > maxConfigBytes {
		return fmt.Errorf("configuration exceeds %d bytes", maxConfigBytes)
	}
	temporary, err := os.CreateTemp(directory, ".tadx-config-*.yaml")
	if err != nil {
		return fmt.Errorf("create configuration staging file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("protect configuration staging file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write configuration staging file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync configuration staging file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close configuration staging file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("install configuration: %w", err)
	}
	return nil
}

// ErrNoChange lets an Update mutator report that the configuration is already
// in the desired state so Update skips the write and leaves the file untouched.
var ErrNoChange = errors.New("configuration unchanged")

const configLockSuffix = ".lock"

// Update performs one serialized read-modify-write of the user configuration.
// It holds an advisory interprocess lock across the load, the mutation, and the
// atomic save so concurrent env or workspace updates in separate tadx processes
// cannot lose writes. When createIfMissing is true a missing configuration file
// is treated as an empty current-version configuration; otherwise a missing
// file is returned as an error. A mutator that returns ErrNoChange leaves the
// file untouched and Update returns the loaded configuration unchanged.
func Update(path string, createIfMissing bool, mutate func(Config) (Config, error)) (Config, error) {
	return UpdateWithRollback(path, createIfMissing, func(current Config) (Config, func() error, error) {
		next, err := mutate(current)
		return next, nil, err
	})
}

// UpdateWithRollback serializes a configuration mutation and its filesystem
// staging. The mutator can return a rollback function, including on error.
// If the mutation or save fails, rollback runs before releasing the configuration
// lock. A successful save commits the staging and does not invoke rollback.
func UpdateWithRollback(path string, createIfMissing bool, mutate func(Config) (Config, func() error, error)) (result Config, resultErr error) {
	return updateTransaction(path, createIfMissing, func(current Config) (Config, func() error, func() error, error) {
		next, rollback, err := mutate(current)
		return next, rollback, nil, err
	})
}

// UpdateWithPostSave serializes a configuration mutation and an irreversible
// external commit. The callback runs after the new configuration is durable
// while the configuration lock remains held. If the callback fails, the prior
// configuration is restored before the lock is released.
func UpdateWithPostSave(path string, createIfMissing bool, mutate func(Config) (Config, func() error, error)) (result Config, resultErr error) {
	return updateTransaction(path, createIfMissing, func(current Config) (Config, func() error, func() error, error) {
		next, postSave, err := mutate(current)
		return next, nil, postSave, err
	})
}

func updateTransaction(path string, createIfMissing bool, mutate func(Config) (Config, func() error, func() error, error)) (result Config, resultErr error) {
	if strings.TrimSpace(path) == "" {
		return Config{}, errors.New("configuration path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Config{}, fmt.Errorf("create configuration directory: %w", err)
	}
	handle, err := lock.Acquire(path + configLockSuffix)
	if err != nil {
		return Config{}, fmt.Errorf("acquire configuration lock: %w", err)
	}
	defer func() { _ = handle.Release() }()
	var rollback func() error
	committed := false
	defer func() {
		if !committed && rollback != nil {
			if err := rollback(); err != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("rollback configuration mutation: %w", err))
			}
		}
	}()
	current, original, migrated, err := load(path)
	if err != nil {
		if !(createIfMissing && errors.Is(err, os.ErrNotExist)) {
			return Config{}, err
		}
		current, original, migrated = Config{Version: CurrentVersion}, nil, false
	}
	next, rollback, postSave, err := mutate(cloneConfig(current))
	if err != nil {
		if errors.Is(err, ErrNoChange) {
			// The mutator reports no logical change, but an in-memory legacy
			// migration may still be pending on disk. Persist it now, under the
			// lock, rather than leaving the upgrade to a future write.
			if migrated {
				if err := preserveWorkspaceMigrationBackup(path, original); err != nil {
					return Config{}, err
				}
				if err := Save(path, current); err != nil {
					return Config{}, fmt.Errorf("persist migrated workspace configuration: %w", err)
				}
			}
			committed = true
			return current, nil
		}
		return Config{}, err
	}
	if migrated {
		if err := preserveWorkspaceMigrationBackup(path, original); err != nil {
			return Config{}, err
		}
	}
	if err := Save(path, next); err != nil {
		return Config{}, err
	}
	if postSave != nil {
		if err := postSave(); err != nil {
			restoreErr := Save(path, current)
			if restoreErr != nil {
				return Config{}, errors.Join(err, fmt.Errorf("restore configuration after external commit failure: %w", restoreErr))
			}
			return current, err
		}
	}
	committed = true
	return next, nil
}

func cloneConfig(configuration Config) Config {
	clone := configuration
	if configuration.Environments != nil {
		clone.Environments = make(map[string]Environment, len(configuration.Environments))
		for alias, environment := range configuration.Environments {
			clone.Environments[alias] = environment
		}
	}
	if configuration.Workspaces != nil {
		clone.Workspaces = make(map[string]WorkspaceRegistration, len(configuration.Workspaces))
		for name, registration := range configuration.Workspaces {
			clone.Workspaces[name] = registration
		}
	}
	return clone
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
	if !strings.EqualFold(parsed.Scheme, "https") {
		return errors.New("scheme must be https")
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

func isAPIVersion(value string) bool {
	major, minor, found := strings.Cut(value, ".")
	if !found || major == "" || minor == "" || strings.Contains(minor, ".") {
		return false
	}
	for _, part := range []string{major, minor} {
		for index := 0; index < len(part); index++ {
			if part[index] < '0' || part[index] > '9' {
				return false
			}
		}
	}
	return true
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
