package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"

	keyring "github.com/zalando/go-keyring"
)

const (
	credentialService = "io.github.ahillspace.tadx"
	maxPATRecordBytes = 2048
	patRecordVersion  = 1
)

var (
	credentialReferencePattern = regexp.MustCompile(`\Acred_[0-9a-f]{32}\z`)
	targetFingerprintPattern   = regexp.MustCompile(`\Asha256:[0-9a-f]{64}\z`)
)

// CredentialReference identifies one opaque credential-store entry.
type CredentialReference string

// CredentialTarget binds stored credentials to one Tableau server origin and exact site.
type CredentialTarget struct {
	ServerURL      string
	SiteContentURL string
}

// CredentialSource identifies where a complete PAT pair originated.
type CredentialSource string

const (
	// CredentialSourceEnvironment identifies a complete process-environment PAT pair.
	CredentialSourceEnvironment CredentialSource = "environment"
	// CredentialSourceOSKeyring identifies a PAT pair loaded from the native OS credential store.
	CredentialSourceOSKeyring CredentialSource = "os_keyring"
)

// PATCredentials contains a resolved PAT pair inside the authentication boundary.
// Its formatting methods never expose credential values.
type PATCredentials struct {
	Name   string
	Secret string
	Source CredentialSource
}

func (c PATCredentials) String() string {
	return fmt.Sprintf("resolved PAT credentials from %s ([REDACTED])", c.Source)
}

func (c PATCredentials) GoString() string { return c.String() }

// MarshalJSON emits only the credential source.
func (c PATCredentials) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Source CredentialSource `json:"source"`
	}{Source: c.Source})
}

// PATStore persists complete PAT pairs in a native credential store.
type PATStore interface {
	StorePAT(context.Context, CredentialTarget, string, string) (CredentialReference, error)
	ReplacePAT(context.Context, CredentialReference, CredentialTarget, string, string) error
	LoadPAT(context.Context, CredentialReference, CredentialTarget) (PATCredentials, error)
	DeletePAT(context.Context, CredentialReference) error
}

// CredentialStoreErrorKind classifies credential-store failures without exposing backend details.
type CredentialStoreErrorKind string

const (
	CredentialStoreInvalid         CredentialStoreErrorKind = "invalid"
	CredentialStoreNotFound        CredentialStoreErrorKind = "not_found"
	CredentialStoreUnavailable     CredentialStoreErrorKind = "unavailable"
	CredentialStoreLocked          CredentialStoreErrorKind = "locked"
	CredentialStoreDenied          CredentialStoreErrorKind = "denied"
	CredentialStoreUnsupported     CredentialStoreErrorKind = "unsupported"
	CredentialStoreCorrupt         CredentialStoreErrorKind = "corrupt"
	CredentialStoreTargetMismatch  CredentialStoreErrorKind = "target_mismatch"
	CredentialStoreTooLarge        CredentialStoreErrorKind = "too_large"
	CredentialStoreCanceled        CredentialStoreErrorKind = "canceled"
	CredentialStoreOperationFailed CredentialStoreErrorKind = "operation_failed"
)

// CredentialStoreError reports a sanitized credential-store failure.
// It intentionally does not unwrap the backend error.
type CredentialStoreError struct {
	Kind      CredentialStoreErrorKind
	Operation string
}

func (e *CredentialStoreError) Error() string {
	operation := e.Operation
	if operation == "" {
		operation = "access"
	}
	return fmt.Sprintf("credential store %s failed: %s", operation, credentialStoreSummary(e.Kind))
}

// Retryable reports whether retrying can succeed after transient local state changes.
func (e *CredentialStoreError) Retryable() bool {
	return e.Kind == CredentialStoreUnavailable || e.Kind == CredentialStoreOperationFailed
}

// CorrectiveAction returns bounded guidance without backend error text.
func (e *CredentialStoreError) CorrectiveAction() string {
	switch e.Kind {
	case CredentialStoreNotFound:
		return "Store a PAT for the selected environment, or provide both configured PAT environment variables."
	case CredentialStoreUnavailable:
		return "Start the native credential service, or provide both configured PAT environment variables."
	case CredentialStoreLocked:
		return "Unlock the native credential store, then retry."
	case CredentialStoreDenied:
		return "Grant TADX access to the native credential store, then retry."
	case CredentialStoreUnsupported:
		return "Provide both configured PAT environment variables on this system."
	case CredentialStoreCorrupt, CredentialStoreTargetMismatch:
		return "Remove the stored PAT and authenticate the selected environment again."
	case CredentialStoreTooLarge:
		return "Use a PAT name and secret that fit within the native credential-store limit."
	case CredentialStoreCanceled:
		return "Retry when credential-store access can complete."
	case CredentialStoreInvalid:
		return "Review the credential reference and Tableau target configuration, then retry."
	default:
		return "Retry credential-store access, or provide both configured PAT environment variables."
	}
}

func credentialStoreSummary(kind CredentialStoreErrorKind) string {
	switch kind {
	case CredentialStoreInvalid:
		return "invalid credential input"
	case CredentialStoreNotFound:
		return "credential not found"
	case CredentialStoreUnavailable:
		return "native credential service unavailable"
	case CredentialStoreLocked:
		return "native credential store locked"
	case CredentialStoreDenied:
		return "native credential store access denied"
	case CredentialStoreUnsupported:
		return "native credential store unsupported"
	case CredentialStoreCorrupt:
		return "stored credential is invalid"
	case CredentialStoreTargetMismatch:
		return "stored credential belongs to a different Tableau target"
	case CredentialStoreTooLarge:
		return "credential record exceeds the supported size"
	case CredentialStoreCanceled:
		return "operation canceled"
	default:
		return "native credential operation failed"
	}
}

type keyringBackend interface {
	Set(string, string, string) error
	Get(string, string) (string, error)
	Delete(string, string) error
}

type osKeyring struct{}

func (osKeyring) Set(service, account, value string) error {
	return keyring.Set(service, account, value)
}

func (osKeyring) Get(service, account string) (string, error) {
	return keyring.Get(service, account)
}

func (osKeyring) Delete(service, account string) error {
	return keyring.Delete(service, account)
}

type patStore struct {
	backend keyringBackend
	random  io.Reader
}

type patRecord struct {
	Version           int    `json:"version"`
	PATName           string `json:"pat_name"`
	PATSecret         string `json:"pat_secret"`
	TargetFingerprint string `json:"target_fingerprint"`
}

// NewOSPATStore creates a PAT store backed only by the native OS credential service.
func NewOSPATStore() PATStore {
	return newPATStore(osKeyring{}, rand.Reader)
}

func newPATStore(backend keyringBackend, random io.Reader) *patStore {
	return &patStore{backend: backend, random: random}
}

func (s *patStore) StorePAT(ctx context.Context, target CredentialTarget, name, secret string) (CredentialReference, error) {
	if err := credentialContextError(ctx, "store"); err != nil {
		return "", err
	}
	if s == nil || s.backend == nil || s.random == nil || strings.TrimSpace(name) == "" || strings.TrimSpace(secret) == "" {
		return "", &CredentialStoreError{Kind: CredentialStoreInvalid, Operation: "store"}
	}
	randomID := make([]byte, 16)
	if _, err := io.ReadFull(s.random, randomID); err != nil {
		return "", &CredentialStoreError{Kind: CredentialStoreOperationFailed, Operation: "store"}
	}
	reference := CredentialReference("cred_" + hex.EncodeToString(randomID))
	if err := s.writePAT(ctx, "store", reference, target, name, secret); err != nil {
		return "", err
	}
	return reference, nil
}

// ReplacePAT atomically overwrites one referenced native credential record.
func (s *patStore) ReplacePAT(ctx context.Context, reference CredentialReference, target CredentialTarget, name, secret string) error {
	return s.writePAT(ctx, "replace", reference, target, name, secret)
}

func (s *patStore) writePAT(ctx context.Context, operation string, reference CredentialReference, target CredentialTarget, name, secret string) error {
	if err := credentialContextError(ctx, operation); err != nil {
		return err
	}
	if s == nil || s.backend == nil || !validCredentialReference(reference) || strings.TrimSpace(name) == "" || strings.TrimSpace(secret) == "" {
		return &CredentialStoreError{Kind: CredentialStoreInvalid, Operation: operation}
	}
	fingerprint, err := credentialTargetFingerprint(target)
	if err != nil {
		return err
	}
	record, err := json.Marshal(patRecord{Version: patRecordVersion, PATName: name, PATSecret: secret, TargetFingerprint: fingerprint})
	if err != nil || len(record) > maxPATRecordBytes {
		return &CredentialStoreError{Kind: CredentialStoreTooLarge, Operation: operation}
	}
	if err := s.backend.Set(credentialService, credentialAccount(reference), string(record)); err != nil {
		return classifyCredentialBackendError(operation, err)
	}
	return nil
}

func (s *patStore) LoadPAT(ctx context.Context, reference CredentialReference, target CredentialTarget) (PATCredentials, error) {
	if err := credentialContextError(ctx, "load"); err != nil {
		return PATCredentials{}, err
	}
	if s == nil || s.backend == nil || !validCredentialReference(reference) {
		return PATCredentials{}, &CredentialStoreError{Kind: CredentialStoreInvalid, Operation: "load"}
	}
	fingerprint, err := credentialTargetFingerprint(target)
	if err != nil {
		return PATCredentials{}, err
	}
	encoded, err := s.backend.Get(credentialService, credentialAccount(reference))
	if err != nil {
		return PATCredentials{}, classifyCredentialBackendError("load", err)
	}
	record, err := decodePATRecord(encoded)
	if err != nil {
		return PATCredentials{}, err
	}
	if subtle.ConstantTimeCompare([]byte(record.TargetFingerprint), []byte(fingerprint)) != 1 {
		return PATCredentials{}, &CredentialStoreError{Kind: CredentialStoreTargetMismatch, Operation: "load"}
	}
	return PATCredentials{Name: record.PATName, Secret: record.PATSecret, Source: CredentialSourceOSKeyring}, nil
}

func (s *patStore) DeletePAT(ctx context.Context, reference CredentialReference) error {
	if err := credentialContextError(ctx, "delete"); err != nil {
		return err
	}
	if s == nil || s.backend == nil || !validCredentialReference(reference) {
		return &CredentialStoreError{Kind: CredentialStoreInvalid, Operation: "delete"}
	}
	if err := s.backend.Delete(credentialService, credentialAccount(reference)); err != nil {
		return classifyCredentialBackendError("delete", err)
	}
	return nil
}

func decodePATRecord(encoded string) (patRecord, error) {
	if len(encoded) == 0 || len(encoded) > maxPATRecordBytes || !hasUniqueTopLevelJSONFields(encoded) {
		return patRecord{}, &CredentialStoreError{Kind: CredentialStoreCorrupt, Operation: "load"}
	}
	decoder := json.NewDecoder(strings.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var record patRecord
	if err := decoder.Decode(&record); err != nil {
		return patRecord{}, &CredentialStoreError{Kind: CredentialStoreCorrupt, Operation: "load"}
	}
	if err := ensureJSONEOF(decoder); err != nil || record.Version != patRecordVersion || record.PATName == "" || record.PATSecret == "" || !targetFingerprintPattern.MatchString(record.TargetFingerprint) {
		return patRecord{}, &CredentialStoreError{Kind: CredentialStoreCorrupt, Operation: "load"}
	}
	return record, nil
}

func hasUniqueTopLevelJSONFields(encoded string) bool {
	decoder := json.NewDecoder(strings.NewReader(encoded))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return false
	}
	seen := make(map[string]struct{}, 4)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		name, ok := token.(string)
		if !ok {
			return false
		}
		if _, duplicate := seen[name]; duplicate {
			return false
		}
		seen[name] = struct{}{}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return false
		}
	}
	token, err = decoder.Token()
	if err != nil || token != json.Delim('}') {
		return false
	}
	return ensureJSONEOF(decoder) == nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func validCredentialReference(reference CredentialReference) bool {
	return credentialReferencePattern.MatchString(string(reference))
}

func credentialAccount(reference CredentialReference) string {
	return "pat:v1:" + strings.TrimPrefix(string(reference), "cred_")
}

func credentialTargetFingerprint(target CredentialTarget) (string, error) {
	serverTarget, err := normalizeCredentialServerTarget(target.ServerURL)
	if err != nil || target.SiteContentURL != strings.TrimSpace(target.SiteContentURL) {
		return "", &CredentialStoreError{Kind: CredentialStoreInvalid, Operation: "target"}
	}
	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "%d:%s%d:%s", len(serverTarget), serverTarget, len(target.SiteContentURL), target.SiteContentURL)
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func normalizeCredentialServerTarget(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return "", errors.New("invalid HTTPS server origin")
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return "", errors.New("server URL host is empty")
	}
	port := parsed.Port()
	if port == "443" {
		port = ""
	}
	host := hostname
	if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	if port != "" {
		host = net.JoinHostPort(hostname, port)
	}
	basePath := strings.TrimRight(parsed.EscapedPath(), "/")
	return "https://" + host + basePath, nil
}

func credentialContextError(ctx context.Context, operation string) error {
	if ctx == nil {
		return &CredentialStoreError{Kind: CredentialStoreInvalid, Operation: operation}
	}
	if err := ctx.Err(); err != nil {
		return &CredentialStoreError{Kind: CredentialStoreCanceled, Operation: operation}
	}
	return nil
}

func classifyCredentialBackendError(operation string, err error) error {
	if err == nil {
		return nil
	}
	kind := CredentialStoreOperationFailed
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		kind = CredentialStoreCanceled
	case errors.Is(err, keyring.ErrNotFound):
		kind = CredentialStoreNotFound
	case errors.Is(err, keyring.ErrSetDataTooBig):
		kind = CredentialStoreTooLarge
	case errors.Is(err, os.ErrPermission):
		kind = CredentialStoreDenied
	default:
		message := strings.ToLower(err.Error())
		switch {
		case containsAny(message, "not implemented", "not supported", "unsupported platform"):
			kind = CredentialStoreUnsupported
		case containsAny(message, "is locked", "islocked", "collection locked", "keyring locked", "interaction is not allowed"):
			kind = CredentialStoreLocked
		case containsAny(message, "access denied", "permission denied", "authorization denied", "not authorized"):
			kind = CredentialStoreDenied
		case containsAny(message, "session bus", "serviceunknown", "service unknown", "namehasnoowner", "dbus", "d-bus", "no such interface", "cannot autolaunch"):
			kind = CredentialStoreUnavailable
		}
	}
	return &CredentialStoreError{Kind: kind, Operation: operation}
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

// PartialEnvironmentCredentialsError reports a partially populated PAT environment override.
type PartialEnvironmentCredentialsError struct {
	Environment string
	Present     string
	Missing     string
}

func (e *PartialEnvironmentCredentialsError) Error() string {
	return fmt.Sprintf("environment %q has a partial PAT environment override: %s is set and %s is missing", e.Environment, e.Present, e.Missing)
}

// PATSourceResolver resolves one complete PAT pair without mixing credential sources.
type PATSourceResolver struct {
	lookup LookupEnv
	store  PATStore
}

// NewPATSourceResolver creates a resolver that prefers a complete environment pair.
func NewPATSourceResolver(lookup LookupEnv, store PATStore) *PATSourceResolver {
	return &PATSourceResolver{lookup: lookup, store: store}
}

// Resolve returns a complete PAT pair from the environment or native credential store.
func (r *PATSourceResolver) Resolve(ctx context.Context, target Target) (PATCredentials, error) {
	if ctx == nil {
		return PATCredentials{}, &CredentialStoreError{Kind: CredentialStoreInvalid, Operation: "resolve"}
	}
	if err := ctx.Err(); err != nil {
		return PATCredentials{}, &CredentialStoreError{Kind: CredentialStoreCanceled, Operation: "resolve"}
	}
	if target.PATNameVariable != "" && strings.EqualFold(target.PATNameVariable, target.PATSecretVariable) {
		return PATCredentials{}, errors.New("PAT name and secret must use different environment variables")
	}
	if (target.PATNameVariable == "") != (target.PATSecretVariable == "") {
		present, missing := target.PATNameVariable, target.PATSecretVariable
		if present == "" {
			present, missing = missing, "<unset reference>"
		}
		return PATCredentials{}, &PartialEnvironmentCredentialsError{Environment: target.Environment, Present: present, Missing: missing}
	}

	name, namePresent := "", false
	secret, secretPresent := "", false
	if target.PATNameVariable != "" {
		if r == nil || r.lookup == nil {
			return PATCredentials{}, errors.New("PAT environment lookup is not configured")
		}
		name, namePresent = r.lookup.LookupEnv(target.PATNameVariable)
		secret, secretPresent = r.lookup.LookupEnv(target.PATSecretVariable)
		namePresent = namePresent && strings.TrimSpace(name) != ""
		secretPresent = secretPresent && strings.TrimSpace(secret) != ""
	}
	if namePresent && secretPresent {
		return PATCredentials{Name: name, Secret: secret, Source: CredentialSourceEnvironment}, nil
	}
	if namePresent != secretPresent {
		present, missing := target.PATNameVariable, target.PATSecretVariable
		if secretPresent {
			present, missing = missing, present
		}
		return PATCredentials{}, &PartialEnvironmentCredentialsError{Environment: target.Environment, Present: present, Missing: missing}
	}
	if target.CredentialReference != "" {
		if r == nil || r.store == nil {
			return PATCredentials{}, &CredentialStoreError{Kind: CredentialStoreUnavailable, Operation: "load"}
		}
		credentials, err := r.store.LoadPAT(ctx, CredentialReference(target.CredentialReference), CredentialTarget{
			ServerURL: target.ServerURL, SiteContentURL: target.SiteContentURL,
		})
		credentials.Source = CredentialSourceOSKeyring
		return credentials, err
	}

	missing := []string{target.PATNameVariable, target.PATSecretVariable}
	for index, variable := range missing {
		if variable == "" {
			missing[index] = "<unset reference>"
		}
	}
	sortStrings(missing)
	return PATCredentials{}, &MissingVariablesError{Environment: target.Environment, Variables: missing}
}

func sortStrings(values []string) {
	if len(values) == 2 && values[1] < values[0] {
		values[0], values[1] = values[1], values[0]
	}
}
