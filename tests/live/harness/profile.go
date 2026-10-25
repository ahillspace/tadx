package harness

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
)

type Credential struct {
	NameEnv       string `json:"name_env,omitempty"`
	SecretEnv     string `json:"secret_env,omitempty"`
	CredentialRef string `json:"credential_ref,omitempty"`
}

type Budgets struct {
	CommandSeconds     int `json:"command_seconds"`
	ObservationSeconds int `json:"observation_seconds"`
	OperationSeconds   int `json:"operation_seconds"`
}

type Profile struct {
	Version          int                `json:"schema_version"`
	Alias            string             `json:"alias"`
	ServerURL        string             `json:"server_url"`
	SiteContentURL   string             `json:"site_content_url"`
	ExpectedSiteLUID string             `json:"expected_site_luid,omitempty"`
	ConfigPath       string             `json:"config_path"`
	Environment      string             `json:"environment"`
	ActorPAT         Credential         `json:"actor_pat"`
	FixturePAT       Credential         `json:"fixture_pat,omitzero"`
	Artifacts        map[string]string  `json:"artifacts,omitempty"`
	Bindings         map[string]Binding `json:"bindings,omitempty"`
	Features         map[string]string  `json:"features,omitempty"`
	Budgets          Budgets            `json:"budgets"`
	CAFile           string             `json:"ca_file,omitempty"`
	Python           string             `json:"python,omitempty"`
	OptionalPacks    []string           `json:"optional_packs,omitempty"`
}

func LoadProfile(path string) (Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, err
	}
	if len(data) > 1<<20 {
		return Profile{}, errors.New("target profile exceeds 1 MiB")
	}
	var keys map[string]json.RawMessage
	if err = json.Unmarshal(data, &keys); err != nil {
		return Profile{}, errors.New("invalid target profile JSON")
	}
	if _, ok := keys["site_content_url"]; !ok || bytes.Equal(keys["site_content_url"], []byte("null")) {
		return Profile{}, errors.New("site_content_url must be explicitly present, including an empty Server default-site value")
	}
	var p Profile
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(&p); err != nil {
		return p, fmt.Errorf("decode target profile: %w", err)
	}
	if err = d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return p, errors.New("target profile contains trailing JSON")
	}
	base, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return p, err
	}
	resolve := func(s string) string {
		if s != "" && !filepath.IsAbs(s) {
			return filepath.Join(base, s)
		}
		return s
	}
	p.ConfigPath = resolve(p.ConfigPath)
	p.CAFile = resolve(p.CAFile)
	if strings.ContainsAny(p.Python, "/\\") {
		p.Python = resolve(p.Python)
	}
	for k, v := range p.Artifacts {
		p.Artifacts[k] = resolve(v)
	}
	for k, v := range p.Bindings {
		v.Path = resolve(v.Path)
		p.Bindings[k] = v
	}
	if p.Budgets.CommandSeconds == 0 {
		p.Budgets.CommandSeconds = 120
	}
	if p.Budgets.ObservationSeconds == 0 {
		p.Budgets.ObservationSeconds = 300
	}
	if p.Budgets.OperationSeconds == 0 {
		p.Budgets.OperationSeconds = 1200
	}
	return p, p.Validate()
}

func (p Profile) Validate() error {
	if p.Version != 1 {
		return errors.New("unsupported target profile schema_version")
	}
	if p.Alias == "" || p.Environment == "" || p.ConfigPath == "" {
		return errors.New("target profile requires alias, environment, and config_path")
	}
	u, err := url.Parse(p.ServerURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("server_url must be an HTTPS URL without credentials, query, or fragment")
	}
	for _, v := range []string{p.Alias, p.Environment, p.SiteContentURL} {
		if strings.TrimSpace(v) != v || strings.ContainsAny(v, "\r\n\x00") {
			return errors.New("target selectors contain whitespace or control characters")
		}
	}
	for _, c := range []Credential{p.ActorPAT, p.FixturePAT} {
		if (c.NameEnv == "") != (c.SecretEnv == "") {
			return errors.New("both PAT environment-variable references are required")
		}
		if c.NameEnv != "" && c.NameEnv == c.SecretEnv {
			return errors.New("PAT name and secret references must differ")
		}
	}
	if p.ActorPAT.NameEnv == "" && p.ActorPAT.CredentialRef == "" {
		return errors.New("actor_pat requires environment references or an approved native credential reference")
	}
	for _, n := range []int{p.Budgets.CommandSeconds, p.Budgets.ObservationSeconds, p.Budgets.OperationSeconds} {
		if n < 1 || n > 86400 {
			return errors.New("budgets must be between 1 and 86400 seconds")
		}
	}
	return nil
}

func ProfileFromEnvironment(path, environment string) (Profile, error) {
	if path == "" {
		var err error
		path, err = config.UserConfigPath()
		if err != nil {
			return Profile{}, err
		}
	}
	c, err := config.Load(path)
	if err != nil {
		return Profile{}, err
	}
	env, err := c.ResolveEnvironment(environment)
	if err != nil {
		return Profile{}, err
	}
	p := Profile{Version: 1, Alias: env.Alias, ServerURL: env.URL, SiteContentURL: env.SiteContentURL, ConfigPath: path, Environment: env.Alias, ActorPAT: Credential{env.Auth.PATNameEnv, env.Auth.PATSecretEnv, env.Auth.CredentialRef}, Budgets: Budgets{120, 300, 1200}, Bindings: map[string]Binding{}, Artifacts: map[string]string{}}
	return p, p.Validate()
}

// ResolvePAT reuses only the native credential resolution boundary, never the
// product's HTTP transport, API clients, payload decoders, or result normalizers.
func ResolvePAT(ctx context.Context, p Profile) (string, string, error) {
	c := p.FixturePAT
	if c.NameEnv == "" && c.CredentialRef == "" {
		c = p.ActorPAT
	}
	r := auth.NewPATSourceResolver(auth.LookupEnvFunc(os.LookupEnv), auth.NewOSPATStore())
	v, err := r.Resolve(ctx, auth.Target{Environment: p.Environment, ServerURL: p.ServerURL, SiteContentURL: p.SiteContentURL, PATNameVariable: c.NameEnv, PATSecretVariable: c.SecretEnv, CredentialReference: c.CredentialRef})
	if err != nil {
		return "", "", Blocked("fixture PAT credentials are unavailable: " + fmt.Sprintf("%T", err))
	}
	return v.Name, v.Secret, nil
}

func Fingerprint(server, site, id string) string {
	canonical, err := config.CanonicalMutationServer(server)
	if err != nil {
		canonical = strings.TrimRight(server, "/")
	}
	h := sha256.Sum256([]byte(canonical + "\x00" + site + "\x00" + id))
	return hex.EncodeToString(h[:])
}

func (p Profile) VerifyConsent() error {
	c, err := config.Load(p.ConfigPath)
	if err != nil {
		return err
	}
	env, err := c.ResolveEnvironment(p.Environment)
	if err != nil {
		return err
	}
	server, err := config.CanonicalMutationServer(env.URL)
	if err != nil {
		return err
	}
	want, err := config.CanonicalMutationServer(p.ServerURL)
	if err != nil {
		return err
	}
	if server != want || env.SiteContentURL != p.SiteContentURL {
		return errors.New("profile and selected config resolve different targets")
	}
	consent, err := c.MutationSetting(env)
	if err != nil {
		return err
	}
	if consent == nil || !*consent {
		return Blocked("the selected canonical server and exact site lack persisted remote-mutation consent; setup never enables it")
	}
	return nil
}
