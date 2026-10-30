package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ahillspace/tadx/internal/fsreplace"
	"github.com/ahillspace/tadx/internal/lock"
)

// CommandSessions owns one command's sessions. Signing in with a PAT ends the
// PAT's earlier sessions, so each TADX sign-in holds the PAT's lock and
// advances the PAT's shared sign-in epoch.
//
// A stored PAT's session is saved beside it, so separate commands share it:
// a command adopts the saved session of the PAT's latest sign-in and holds the
// PAT's lock only while signing in. Any other PAT's session cannot be shared,
// so the command leases that PAT's lock until Suspend or Close; another
// command signing in with it would end this command's session.
// Close must run after every request using these sessions has finished.
type CommandSessions struct {
	gate        chan struct{}
	resolver    *PATSourceResolver
	directory   string
	closed      bool
	credentials map[credentialSourceKey]credentialResult
	leases      map[string]*lock.Handle
	sessions    map[sessionKey]sessionEntry
}

type credentialSourceKey struct{ server, site, nameVariable, secretVariable, reference string }
type credentialResult struct {
	value PATCredentials
	err   error
}
type sessionKey struct{ credential, site string }

// sessionEntry is the command's one session for a PAT and site and the
// sign-in epoch its current token was issued at.
type sessionEntry struct {
	session *session
	epoch   uint64
}

// NewCommandSessions scopes credentials to a single command. An empty directory
// selects the user's shared cache directory so different configs coordinate.
func NewCommandSessions(lookup LookupEnv, store PATStore, directory string) *CommandSessions {
	if lookup != nil {
		original := lookup
		type environmentValue struct {
			value   string
			present bool
		}
		values := make(map[string]environmentValue)
		// Resolution is serialized by the command gate. Capture even missing
		// variables once across targets; stored credentials remain target-bound.
		lookup = LookupEnvFunc(func(name string) (string, bool) {
			result, exists := values[name]
			if !exists {
				result.value, result.present = original.LookupEnv(name)
				values[name] = result
			}
			return result.value, result.present
		})
	}
	return &CommandSessions{gate: make(chan struct{}, 1), resolver: NewPATSourceResolver(lookup, store), directory: directory, credentials: make(map[credentialSourceKey]credentialResult), leases: make(map[string]*lock.Handle), sessions: make(map[sessionKey]sessionEntry)}
}

func (c *CommandSessions) enter(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case c.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *CommandSessions) leave() { <-c.gate }

// Authenticate resolves each source once and reuses only the same actual PAT,
// normalized server, and exact site. Authentication calls acquire this gate;
// callers may use returned sessions after the call completes.
func (c *CommandSessions) Authenticate(ctx context.Context, target Target, signer Signer) (Session, error) {
	session, _, err := c.authenticateWithSource(ctx, target, signer, authenticateReuse)
	return session, err
}

// SignIn always signs in, so the PAT itself is validated, and saves the new
// session for later commands like any other sign-in. It retains the source
// selected for this exact attempt, including an upstream sign-in failure.
func (c *CommandSessions) SignIn(ctx context.Context, target Target, signer Signer) (Session, CredentialSource, error) {
	return c.authenticateWithSource(ctx, target, signer, authenticateSignIn)
}

// CoordinationKey returns an opaque identity for the resolved PAT, normalized
// server, and exact site. It contains no credential or session material.
func (c *CommandSessions) CoordinationKey(ctx context.Context, target Target) (string, error) {
	if err := c.enter(ctx); err != nil {
		return "", err
	}
	defer c.leave()
	if c.closed {
		return "", errors.New("command authentication is closed")
	}
	_, server, credentials, err := c.resolveCredentials(ctx, target)
	if err != nil {
		return "", err
	}
	if err := validateAuthenticationInputs(target, credentials); err != nil {
		return "", err
	}
	return coordinationIdentity(server, target.SiteContentURL, credentials), nil
}

// Suspend releases the command's PAT leases while retaining command-local
// credentials and sessions, so a long job wait does not keep other commands
// from a PAT whose sessions cannot be shared. Later authentication checks the
// PAT's sign-in epoch before reusing those sessions.
func (c *CommandSessions) Suspend(ctx context.Context) error {
	if err := c.enter(ctx); err != nil {
		return err
	}
	defer c.leave()
	if c.closed {
		return errors.New("command authentication is closed")
	}
	return c.releaseLeases()
}

func (c *CommandSessions) releaseLeases() error {
	var failure error
	for identity, held := range c.leases {
		if err := held.Release(); err != nil {
			failure = errors.New("credential coordination lock release failed")
		}
		delete(c.leases, identity)
	}
	return failure
}

// authenticateMode selects how an authentication call treats an existing
// session.
type authenticateMode int

const (
	// authenticateReuse reuses the PAT's current session.
	authenticateReuse authenticateMode = iota
	// authenticateSignIn always signs in.
	authenticateSignIn
	// authenticateRenew replaces a rejected session with the PAT's latest
	// sign-in, signing in only when none newer than the rejection is saved.
	authenticateRenew
)

func (c *CommandSessions) authenticateWithSource(ctx context.Context, target Target, signer Signer, mode authenticateMode) (Session, CredentialSource, error) {
	if err := c.enter(ctx); err != nil {
		return nil, "", err
	}
	defer c.leave()
	if c.closed {
		return nil, "", errors.New("command authentication is closed")
	}
	_, server, credentials, err := c.resolveCredentials(ctx, target)
	if err != nil {
		return nil, credentials.Source, err
	}
	session, err := c.authenticate(ctx, sessionOrigin{owner: c, target: target, credentials: credentials, signer: signer, server: server}, mode, "")
	if err != nil {
		return nil, credentials.Source, err
	}
	return session, credentials.Source, nil
}

func (c *CommandSessions) resolveCredentials(ctx context.Context, target Target) (credentialSourceKey, string, PATCredentials, error) {
	server, err := normalizeCredentialServerTarget(target.ServerURL)
	if err != nil {
		return credentialSourceKey{}, "", PATCredentials{}, err
	}
	key := credentialSourceKey{server, target.SiteContentURL, target.PATNameVariable, target.PATSecretVariable, target.CredentialReference}
	resolved, ok := c.credentials[key]
	if !ok {
		resolved.value, resolved.err = c.resolver.Resolve(ctx, target)
		c.credentials[key] = resolved
	}
	return key, server, resolved.value, resolved.err
}

// AuthenticateCredentials validates an explicitly provided login PAT through
// the same credential coordination boundary without persisting credentials.
func (c *CommandSessions) AuthenticateCredentials(ctx context.Context, target Target, credentials PATCredentials, signer Signer) (Session, error) {
	if err := c.enter(ctx); err != nil {
		return nil, err
	}
	defer c.leave()
	if c.closed {
		return nil, errors.New("command authentication is closed")
	}
	server, err := normalizeCredentialServerTarget(target.ServerURL)
	if err != nil {
		return nil, err
	}
	session, err := c.authenticate(ctx, sessionOrigin{owner: c, target: target, credentials: credentials, signer: signer, server: server}, authenticateReuse, "")
	if err != nil {
		return nil, err
	}
	return session, nil
}

// sessionOrigin is what a command session was signed in with, so a rejected
// session signs in again with the same PAT, site, and sign-in client.
type sessionOrigin struct {
	owner       *CommandSessions
	target      Target
	credentials PATCredentials
	signer      Signer
	server      string
}

// renew replaces a rejected command session under the command gate.
func (o sessionOrigin) renew(ctx context.Context, current *session, rejected string) error {
	c := o.owner
	if err := c.enter(ctx); err != nil {
		return err
	}
	defer c.leave()
	if c.closed {
		return errors.New("command authentication is closed")
	}
	if current.currentToken() != rejected {
		return nil
	}
	_, err := c.authenticate(ctx, o, authenticateRenew, rejected)
	return err
}

// authenticate returns the command's one session for a PAT and site, updated
// in place to the PAT's latest sign-in. A stored PAT adopts the session saved
// by the latest sign-in from any command; otherwise the PAT signs in.
func (c *CommandSessions) authenticate(ctx context.Context, origin sessionOrigin, mode authenticateMode, rejected string) (*session, error) {
	if err := validateAuthenticationInputs(origin.target, origin.credentials); err != nil {
		return nil, err
	}
	identity := credentialIdentity(origin.server, origin.credentials)
	key := sessionKey{identity, origin.target.SiteContentURL}
	directory, err := c.lockDirectory()
	if err != nil {
		return nil, err
	}
	if _, shared := c.sessionStore(origin); !shared {
		if err := c.lease(ctx, directory, identity); err != nil {
			return nil, err
		}
	}
	if mode == authenticateReuse {
		epoch, err := readSignInEpoch(directory, identity)
		if err != nil {
			return nil, err
		}
		if entry, exists := c.sessions[key]; exists && entry.epoch == epoch {
			return entry.session, nil
		}
		if session, ok := c.adoptSavedSession(ctx, origin, key, identity, epoch, ""); ok {
			return session, nil
		}
	}
	return c.signIn(ctx, directory, origin, key, identity, mode, rejected)
}

// signIn holds the PAT's lock, so that sign-ins with the PAT and their epochs
// are serialized across commands, and signs in unless another command's
// sign-in, made while this one waited, can be adopted instead.
func (c *CommandSessions) signIn(ctx context.Context, directory string, origin sessionOrigin, key sessionKey, identity string, mode authenticateMode, rejected string) (*session, error) {
	if c.leases[identity] == nil {
		held, err := c.acquirePATLock(ctx, directory, identity)
		if err != nil {
			return nil, err
		}
		defer held.Release()
	}
	epoch, err := readSignInEpoch(directory, identity)
	if err != nil {
		return nil, err
	}
	if mode != authenticateSignIn {
		if session, ok := c.adoptSavedSession(ctx, origin, key, identity, epoch, rejected); ok {
			return session, nil
		}
	}
	// Signing in with the same PAT invalidates its sessions across sites.
	// Epochs share the PAT lock's scope, not the site-specific job pool.
	response, err := signIn(ctx, origin.target, origin.credentials, origin.signer)
	if err != nil {
		return nil, err
	}
	if epoch == ^uint64(0) {
		return nil, errors.New("credential sign-in epoch is exhausted")
	}
	epoch++
	if err := writeSignInEpoch(directory, identity, epoch); err != nil {
		return nil, err
	}
	session, err := c.install(key, epoch, response, origin)
	if err != nil {
		return nil, err
	}
	c.saveSession(ctx, origin, identity, epoch, response)
	return session, nil
}

// adoptSavedSession installs the stored PAT's saved session when it belongs to
// the PAT's current epoch and is not the token Tableau rejected.
func (c *CommandSessions) adoptSavedSession(ctx context.Context, origin sessionOrigin, key sessionKey, identity string, epoch uint64, rejected string) (*session, bool) {
	saved, ok := c.loadSavedSession(ctx, origin, identity, epoch)
	if !ok || saved.Token == rejected {
		return nil, false
	}
	session, err := c.install(key, epoch, saved, origin)
	return session, err == nil
}

// install makes response the command's session for key, replacing an existing
// session in place so every client holding it sends the new token.
func (c *CommandSessions) install(key sessionKey, epoch uint64, response SignInResponse, origin sessionOrigin) (*session, error) {
	entry, exists := c.sessions[key]
	if exists {
		if err := entry.session.replace(response); err != nil {
			return nil, err
		}
	} else {
		entry.session = newSession(response, origin)
	}
	c.sessions[key] = sessionEntry{session: entry.session, epoch: epoch}
	return entry.session, nil
}

// lease holds the PAT's lock for the rest of the command, or until Suspend,
// because its session cannot be shared with another command.
func (c *CommandSessions) lease(ctx context.Context, directory, identity string) error {
	if c.leases[identity] != nil {
		return nil
	}
	held, err := c.acquirePATLock(ctx, directory, identity)
	if err != nil {
		return err
	}
	c.leases[identity] = held
	return nil
}

func (c *CommandSessions) acquirePATLock(ctx context.Context, directory, identity string) (*lock.Handle, error) {
	path := filepath.Join(directory, identity+".lock")
	var (
		held *lock.Handle
		err  error
	)
	if len(c.leases) == 0 {
		held, err = lock.AcquireContext(ctx, path)
	} else {
		// Waiting for one PAT while leasing another permits lock-order cycles
		// between multi-target commands. Additional credentials fail promptly.
		held, err = lock.TryAcquire(path)
	}
	switch {
	case err == nil:
		return held, nil
	case errors.Is(err, lock.ErrLocked):
		return nil, errors.New("PAT is in use by another command; retry after that command finishes")
	case ctx.Err() != nil:
		return nil, fmt.Errorf("PAT is in use by another command; credential wait ended: %w", ctx.Err())
	default:
		return nil, errors.New("credential coordination lock is unavailable")
	}
}

// sessionStore returns where origin's sessions persist. Only sessions of a
// PAT loaded from the native credential store persist, beside that PAT.
func (c *CommandSessions) sessionStore(origin sessionOrigin) (PATStore, bool) {
	return c.resolver.store, origin.credentials.Source == CredentialSourceOSKeyring
}

// loadSavedSession returns a previous command's session for the same PAT and
// target when no TADX sign-in with that PAT has superseded it. Any store
// failure means signing in, as without a saved session.
func (c *CommandSessions) loadSavedSession(ctx context.Context, origin sessionOrigin, identity string, epoch uint64) (SignInResponse, bool) {
	store, ok := c.sessionStore(origin)
	if !ok {
		return SignInResponse{}, false
	}
	saved, err := store.LoadSession(ctx, CredentialReference(origin.target.CredentialReference), CredentialTarget{ServerURL: origin.target.ServerURL, SiteContentURL: origin.target.SiteContentURL})
	if err != nil || saved.Credential != identity || saved.Epoch != epoch {
		return SignInResponse{}, false
	}
	return SignInResponse{Token: saved.Token, SiteLUID: saved.SiteLUID, UserLUID: saved.UserLUID}, true
}

// saveSession keeps a new sign-in for later commands. A failed save only
// means the next command signs in.
func (c *CommandSessions) saveSession(ctx context.Context, origin sessionOrigin, identity string, epoch uint64, response SignInResponse) {
	store, ok := c.sessionStore(origin)
	if !ok {
		return
	}
	_ = store.SaveSession(ctx, CredentialReference(origin.target.CredentialReference), CredentialTarget{ServerURL: origin.target.ServerURL, SiteContentURL: origin.target.SiteContentURL}, SavedSession{Credential: identity, Epoch: epoch, Token: response.Token, SiteLUID: response.SiteLUID, UserLUID: response.UserLUID})
}

func validateAuthenticationInputs(target Target, credentials PATCredentials) error {
	if target.SiteContentURL != strings.TrimSpace(target.SiteContentURL) {
		return errors.New("authentication site must not contain surrounding whitespace")
	}
	if strings.TrimSpace(credentials.Name) == "" || strings.TrimSpace(credentials.Secret) == "" {
		return errors.New("a complete PAT pair is required")
	}
	return nil
}

func credentialIdentity(server string, credentials PATCredentials) string {
	return digestIdentity("tadx-pat-lock-v1", server, credentials.Name, credentials.Secret)
}

func coordinationIdentity(server, site string, credentials PATCredentials) string {
	return digestIdentity("tadx-pat-coordination-v1", server, site, credentials.Name, credentials.Secret)
}

func digestIdentity(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = fmt.Fprintf(hash, "%d:%s", len(part), part)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func (c *CommandSessions) lockDirectory() (string, error) {
	directory := c.directory
	if directory == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return "", errors.New("credential coordination directory is unavailable")
		}
		directory = filepath.Join(cache, "tadx", "credential-locks")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", errors.New("credential coordination directory is unavailable")
	}
	return directory, nil
}

func signInEpochPath(directory, key string) string {
	return filepath.Join(directory, "epoch-"+key+".txt")
}

func readSignInEpoch(directory, key string) (uint64, error) {
	data, err := os.ReadFile(signInEpochPath(directory, key))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil || len(data) > 20 {
		return 0, errors.New("credential sign-in epoch is unavailable")
	}
	epoch, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, errors.New("credential sign-in epoch is unavailable")
	}
	return epoch, nil
}

// writeSignInEpoch replaces the epoch file atomically, because other commands
// read it without the PAT's lock.
func writeSignInEpoch(directory, key string, epoch uint64) error {
	unavailable := errors.New("credential sign-in epoch is unavailable")
	staged, err := os.CreateTemp(directory, "epoch-*.tmp")
	if err != nil {
		return unavailable
	}
	defer os.Remove(staged.Name())
	_, err = staged.WriteString(strconv.FormatUint(epoch, 10) + "\n")
	err = errors.Join(err, staged.Sync(), staged.Close())
	if err != nil {
		return unavailable
	}
	// A durability failure leaves the new epoch visible now; losing it later
	// only makes a command sign in instead of reusing a saved session.
	if err := fsreplace.Replace(staged.Name(), signInEpochPath(directory, key)); err != nil && !errors.As(err, new(*fsreplace.DurabilityError)) {
		return unavailable
	}
	return nil
}

// Close releases all PAT leases and discards command-local cached state.
func (c *CommandSessions) Close() error {
	if c == nil {
		return nil
	}
	if err := c.enter(context.Background()); err != nil {
		return err
	}
	defer c.leave()
	if c.closed {
		return nil
	}
	c.closed = true
	failure := c.releaseLeases()
	c.credentials, c.sessions, c.leases = nil, nil, nil
	c.resolver = nil
	return failure
}
