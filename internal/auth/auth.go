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
	Environment       string
	ServerURL         string
	SiteContentURL    string
	PATNameVariable   string
	PATSecretVariable string
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
	}{ServerURL: r.ServerURL, SiteContentURL: r.SiteContentURL})
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
	lookup LookupEnv
	signer Signer
}

// NewPATProvider creates a PAT-backed Provider. PAT values remain inside auth.
func NewPATProvider(lookup LookupEnv, signer Signer) Provider {
	return &patProvider{lookup: lookup, signer: signer}
}

func (p *patProvider) Authenticate(ctx context.Context, target Target) (Session, error) {
	if p.lookup == nil {
		return nil, errors.New("PAT environment lookup is not configured")
	}
	if p.signer == nil {
		return nil, errors.New("PAT sign-in client is not configured")
	}
	if target.PATNameVariable != "" && strings.EqualFold(target.PATNameVariable, target.PATSecretVariable) {
		return nil, errors.New("PAT name and secret must use different environment variables")
	}
	values := make(map[string]string, 2)
	missing := make([]string, 0, 2)
	for _, variable := range []string{target.PATNameVariable, target.PATSecretVariable} {
		if variable == "" {
			missing = append(missing, "<unset reference>")
			continue
		}
		value, ok := p.lookup.LookupEnv(variable)
		if !ok || value == "" {
			missing = append(missing, variable)
			continue
		}
		values[variable] = value
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, &MissingVariablesError{Environment: target.Environment, Variables: missing}
	}

	name := values[target.PATNameVariable]
	secret := values[target.PATSecretVariable]
	response, err := p.signer.SignIn(ctx, SignInRequest{
		ServerURL:      target.ServerURL,
		SiteContentURL: target.SiteContentURL,
		PATName:        name,
		PATSecret:      secret,
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
	message := err.Error()
	type interval struct {
		start int
		end   int
	}
	var intervals []interval
	seen := make(map[string]struct{}, len(secrets))
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		if _, exists := seen[secret]; exists {
			continue
		}
		seen[secret] = struct{}{}
		for offset := 0; offset <= len(message)-len(secret); {
			index := strings.Index(message[offset:], secret)
			if index < 0 {
				break
			}
			start := offset + index
			intervals = append(intervals, interval{start: start, end: start + len(secret)})
			offset = start + 1
		}
	}
	if len(intervals) == 0 {
		return errors.New(message)
	}
	sort.Slice(intervals, func(i, j int) bool {
		if intervals[i].start != intervals[j].start {
			return intervals[i].start < intervals[j].start
		}
		return intervals[i].end > intervals[j].end
	})
	merged := intervals[:1]
	for _, current := range intervals[1:] {
		last := &merged[len(merged)-1]
		if current.start <= last.end {
			last.end = max(last.end, current.end)
			continue
		}
		merged = append(merged, current)
	}
	var redacted strings.Builder
	position := 0
	for _, current := range merged {
		redacted.WriteString(message[position:current.start])
		redacted.WriteString("[REDACTED]")
		position = current.end
	}
	redacted.WriteString(message[position:])
	return errors.New(redacted.String())
}
