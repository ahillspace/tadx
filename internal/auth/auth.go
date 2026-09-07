// Package auth hides PAT resolution and sign-in behind an authenticated-session seam.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// TableauAuthHeader is the Tableau session authorization header.
const TableauAuthHeader = "X-Tableau-Auth"

// Target contains non-secret information needed to authenticate an environment.
type Target struct {
	Environment         string
	ServerURL           string
	SiteContentURL      string
	PATNameVariable     string
	PATSecretVariable   string
	CredentialReference string
	Operation           string
}

// Provider authenticates a target without exposing PAT credentials to consumers.
type Provider interface {
	Authenticate(context.Context, Target) (Session, error)
}

// Session exposes authenticated request behavior and authoritative identity.
// It never exposes the session token as a value.
type Session interface {
	fmt.Stringer
	Authorize(*http.Request)
	SiteLUID() string
	UserLUID() string
}

// LookupEnv reads one environment variable.
type LookupEnv interface {
	LookupEnv(string) (string, bool)
}

// LookupEnvFunc adapts a function to LookupEnv.
type LookupEnvFunc func(string) (string, bool)

func (f LookupEnvFunc) LookupEnv(key string) (string, bool) {
	return f(key)
}

// Signer performs PAT sign-in inside the authentication implementation boundary.
// Callers use Provider and never receive SignInRequest.
type Signer interface {
	SignIn(context.Context, SignInRequest) (SignInResponse, error)
}

// SignInRequest is the internal handoff to a Tableau PAT sign-in client.
// Its formatting methods always redact credential values.
type SignInRequest struct {
	ServerURL      string
	SiteContentURL string
	PATName        string
	PATSecret      string
	Operation      string
}

func (r SignInRequest) String() string {
	return fmt.Sprintf("PAT sign-in to %s site %q ([REDACTED])", r.ServerURL, r.SiteContentURL)
}

func (r SignInRequest) GoString() string {
	return r.String()
}

// MarshalJSON emits only non-secret sign-in target fields.
func (r SignInRequest) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ServerURL      string `json:"server_url"`
		SiteContentURL string `json:"site_content_url"`
		Operation      string `json:"operation,omitempty"`
	}{ServerURL: r.ServerURL, SiteContentURL: r.SiteContentURL, Operation: r.Operation})
}

// SignInResponse contains a short-lived session token and authoritative identities.
type SignInResponse struct {
	Token    string
	SiteLUID string
	UserLUID string
}

func (r SignInResponse) String() string {
	return fmt.Sprintf("authenticated Tableau identity for site %s", r.SiteLUID)
}

func (r SignInResponse) GoString() string {
	return r.String()
}

// MarshalJSON emits identities without the session token.
func (r SignInResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		SiteLUID string `json:"site_luid"`
		UserLUID string `json:"user_luid,omitempty"`
	}{SiteLUID: r.SiteLUID, UserLUID: r.UserLUID})
}

// MissingVariablesError reports missing PAT references without credential values.
type MissingVariablesError struct {
	Environment string
	Variables   []string
}

func (e *MissingVariablesError) Error() string {
	return fmt.Sprintf("environment %q is missing required PAT variables: %s", e.Environment, strings.Join(e.Variables, ", "))
}

type patProvider struct {
	resolver *PATSourceResolver
	signer   Signer
}

// NewPATProvider creates a PAT-backed Provider. PAT values remain inside auth.
func NewPATProvider(lookup LookupEnv, signer Signer) Provider {
	return &patProvider{resolver: NewPATSourceResolver(lookup, nil), signer: signer}
}

// NewPATProviderWithStore creates a PAT-backed Provider with optional native credential storage.
func NewPATProviderWithStore(lookup LookupEnv, signer Signer, store PATStore) Provider {
	return &patProvider{resolver: NewPATSourceResolver(lookup, store), signer: signer}
}

func (p *patProvider) Authenticate(ctx context.Context, target Target) (Session, error) {
	if p.signer == nil {
		return nil, errors.New("PAT sign-in client is not configured")
	}
	if p.resolver == nil {
		return nil, errors.New("PAT credential resolver is not configured")
	}
	credentials, err := p.resolver.Resolve(ctx, target)
	if err != nil {
		return nil, err
	}
	name := credentials.Name
	secret := credentials.Secret
	response, err := p.signer.SignIn(ctx, SignInRequest{
		ServerURL:      target.ServerURL,
		SiteContentURL: target.SiteContentURL,
		PATName:        name,
		PATSecret:      secret,
		Operation:      target.Operation,
	})
	if err != nil {
		return nil, fmt.Errorf("sign in: %w", redactError(err, name, secret))
	}
	if response.Token == "" {
		return nil, errors.New("sign in returned an empty session token")
	}
	if response.SiteLUID == "" {
		return nil, errors.New("sign in returned an empty site LUID")
	}
	if response.UserLUID == "" {
		return nil, errors.New("sign in returned an empty user LUID")
	}
	return &session{token: response.Token, siteLUID: response.SiteLUID, userLUID: response.UserLUID}, nil
}

type session struct {
	token    string
	siteLUID string
	userLUID string
}

func (s *session) Authorize(request *http.Request) {
	if request != nil {
		request.Header.Set(TableauAuthHeader, s.token)
	}
}

func (s *session) SiteLUID() string { return s.siteLUID }

func (s *session) UserLUID() string { return s.userLUID }

func (s *session) String() string {
	return fmt.Sprintf("authenticated Tableau session for site %s", s.siteLUID)
}

func redactError(err error, secrets ...string) error {
	redacted := &redactedCarrierError{message: Redact(err.Error(), secrets...)}
	var upstream interface {
		HTTPStatus() int
		TableauCode() string
		TableauSummary() string
		TableauDetail() string
	}
	if errors.As(err, &upstream) {
		redacted.status = upstream.HTTPStatus()
		redacted.code = Redact(upstream.TableauCode(), secrets...)
		redacted.summary = Redact(upstream.TableauSummary(), secrets...)
		redacted.detail = Redact(upstream.TableauDetail(), secrets...)
	}
	var requestID interface{ RequestID() string }
	if errors.As(err, &requestID) {
		redacted.requestID = Redact(requestID.RequestID(), secrets...)
	}
	var retryAdvice interface {
		Retryable() bool
		CorrectiveAction() string
	}
	if errors.As(err, &retryAdvice) {
		return &redactedClassifiedCarrierError{
			redactedCarrierError: redacted,
			retryable:            retryAdvice.Retryable(),
			correctiveAction:     Redact(retryAdvice.CorrectiveAction(), secrets...),
		}
	}
	return redacted
}

type redactedCarrierError struct {
	message   string
	status    int
	code      string
	summary   string
	detail    string
	requestID string
}

func (e *redactedCarrierError) Error() string          { return e.message }
func (e *redactedCarrierError) HTTPStatus() int        { return e.status }
func (e *redactedCarrierError) TableauCode() string    { return e.code }
func (e *redactedCarrierError) TableauSummary() string { return e.summary }
func (e *redactedCarrierError) TableauDetail() string  { return e.detail }
func (e *redactedCarrierError) RequestID() string      { return e.requestID }

type redactedClassifiedCarrierError struct {
	*redactedCarrierError
	retryable        bool
	correctiveAction string
}

func (e *redactedClassifiedCarrierError) Retryable() bool          { return e.retryable }
func (e *redactedClassifiedCarrierError) CorrectiveAction() string { return e.correctiveAction }

// Redact replaces complete and overlapping secret intervals without exposing remainders.
func Redact(message string, secrets ...string) string {
	const marker = "[REDACTED]"

	type secretMatch struct {
		secret string
		start  int
	}
	matches := make([]secretMatch, 0, len(secrets))
	redactionSecrets := make([]string, 0, len(secrets))
	seen := make(map[string]struct{}, len(secrets))
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		if _, exists := seen[secret]; exists {
			continue
		}
		seen[secret] = struct{}{}
		redactionSecrets = append(redactionSecrets, secret)
		if start := strings.Index(message, secret); start >= 0 {
			matches = append(matches, secretMatch{secret: secret, start: start})
		}
	}
	if len(matches) == 0 {
		return message
	}

	var redacted strings.Builder
	position := 0
	intervalStart := -1
	intervalEnd := -1
	for len(matches) > 0 {
		next := 0
		for index := 1; index < len(matches); index++ {
			if matches[index].start < matches[next].start {
				next = index
			}
		}

		current := matches[next]
		currentEnd := current.start + len(current.secret)
		searchStart := current.start + 1
		following := strings.Index(message[searchStart:], current.secret)
		if following < 0 {
			matches = append(matches[:next], matches[next+1:]...)
		} else {
			matches[next].start = searchStart + following
		}

		if intervalStart < 0 {
			intervalStart = current.start
			intervalEnd = currentEnd
			continue
		}
		if current.start <= intervalEnd {
			intervalEnd = max(intervalEnd, currentEnd)
			continue
		}
		redacted.WriteString(message[position:intervalStart])
		writeRedaction(&redacted, marker, intervalEnd-intervalStart)
		position = intervalEnd
		intervalStart = current.start
		intervalEnd = currentEnd
	}
	redacted.WriteString(message[position:intervalStart])
	writeRedaction(&redacted, marker, intervalEnd-intervalStart)
	position = intervalEnd
	redacted.WriteString(message[position:])
	sort.Slice(redactionSecrets, func(left, right int) bool {
		if len(redactionSecrets[left]) != len(redactionSecrets[right]) {
			return len(redactionSecrets[left]) > len(redactionSecrets[right])
		}
		return redactionSecrets[left] < redactionSecrets[right]
	})
	return removeSecrets(redacted.String(), redactionSecrets)
}

func writeRedaction(output *strings.Builder, marker string, coveredBytes int) {
	if coveredBytes >= len(marker) {
		output.WriteString(marker)
		return
	}
	for range coveredBytes {
		output.WriteByte('*')
	}
}

func removeSecrets(value string, secrets []string) string {
	containsSecret := false
	for _, secret := range secrets {
		if strings.Contains(value, secret) {
			containsSecret = true
			break
		}
	}
	if !containsSecret {
		return value
	}

	output := make([]byte, 0, len(value))
	for index := 0; index < len(value); index++ {
		output = append(output, value[index])
		for {
			matched := 0
			for _, secret := range secrets {
				if len(secret) <= len(output) && string(output[len(output)-len(secret):]) == secret {
					matched = len(secret)
					break
				}
			}
			if matched == 0 {
				break
			}
			output = output[:len(output)-matched]
		}
	}
	return string(output)
}
