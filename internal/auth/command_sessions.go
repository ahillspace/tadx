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
	"time"

	"github.com/ahillspace/tadx/internal/fsreplace"
	"github.com/ahillspace/tadx/internal/lock"
)

// CommandSessions owns one command's sessions. Signing in with a PAT ends the
// PAT's earlier sessions at every site, so each TADX sign-in holds the PAT's
// sign-in lock and advances the PAT's shared sign-in epoch.
//
// A command also holds the PAT's use lock until Suspend or Close. A stored
// PAT's session is saved beside it, so commands using the same credential
// reference and site hold the use lock shared and adopt the saved session of
// the PAT's latest sign-in instead of ending each other's sessions. Any other
// use of the PAT, including after a session could not be saved, waits until
// no other command uses the PAT.
// Close must run after every request using these sessions has finished.
type CommandSessions struct {
	gate        chan struct{}
	resolver    *PATSourceResolver
	directory   string
	closed      bool
	credentials map[credentialSourceKey]credentialResult
	uses        map[string]patUse
	sessions    map[sessionKey]sessionEntry
}

// patUse is the command's hold on a PAT's use lock, shared with the commands
// whose use key matches. An empty key means no other command may join.
type patUse struct {
	handle *lock.Handle
	key    string
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
	return &CommandSessions{gate: make(chan struct{}, 1), resolver: NewPATSourceResolver(lookup, store), directory: directory, credentials: make(map[credentialSourceKey]credentialResult), uses: make(map[string]patUse), sessions: make(map[sessionKey]sessionEntry)}
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

// Suspend releases the command's PAT use locks while retaining command-local
// credentials and sessions, so a long job wait does not keep other commands
// from a PAT. Later authentication takes the use lock again and checks the
// PAT's sign-in epoch before reusing those sessions.
func (c *CommandSessions) Suspend(ctx context.Context) error {
	if err := c.enter(ctx); err != nil {
		return err
	}
	defer c.leave()
	if c.closed {
		return errors.New("command authentication is closed")
	}
	return c.releaseUses()
}

func (c *CommandSessions) releaseUses() error {
	var failure error
	for identity := range c.uses {
		if err := c.releaseUse(identity); err != nil {
			failure = err
		}
	}
	return failure
}

func (c *CommandSessions) releaseUse(identity string) error {
	held := c.uses[identity]
	delete(c.uses, identity)
	if err := held.handle.Release(); err != nil {
		return errors.New("credential coordination lock release failed")
	}
	return nil
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
	// A login signs in only to validate the PAT, so it keeps no use of it.
	if releaseErr := c.releaseUse(credentialIdentity(server, credentials)); err == nil {
		err = releaseErr
	}
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
// by the latest sign-in from any command; otherwise the PAT signs in. All of
// this runs under the PAT's sign-in lock, so that sign-ins with the PAT and
// their epochs are serialized across commands and a command that waited
// adopts the sign-in made meanwhile instead of signing in again.
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
	signing, err := c.holdUse(ctx, directory, identity, c.useKey(origin))
	if err != nil {
		return nil, err
	}
	defer signing.Release()
	epoch, err := readSignInEpoch(directory, identity)
	if err != nil {
		return nil, err
	}
	if entry, exists := c.sessions[key]; exists {
		// Another request may have renewed the session while this one waited.
		if mode == authenticateReuse && entry.epoch == epoch || mode == authenticateRenew && entry.session.currentToken() != rejected {
			return entry.session, nil
		}
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
	if !c.saveSession(ctx, origin, identity, epoch, response) {
		if err := c.withholdUse(directory, identity); err != nil {
			return nil, err
		}
	}
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

// useKey identifies the commands that can adopt each other's saved sessions:
// those using the same stored credential reference for the same site. Other
// PATs have no saved session, so their key is empty and asks for sole use.
func (c *CommandSessions) useKey(origin sessionOrigin) string {
	if _, shared := c.sessionStore(origin); !shared {
		return ""
	}
	return digestIdentity("tadx-pat-use-v1", origin.server, origin.target.SiteContentURL, origin.target.CredentialReference)
}

// holdUse returns holding the PAT's sign-in lock once the command may sign in
// with the PAT without ending a session another command cannot replace by
// adopting the saved one. Waiting for other commands releases the command
// gate, so the command's authentication with other PATs proceeds meanwhile.
func (c *CommandSessions) holdUse(ctx context.Context, directory, identity, key string) (*lock.Handle, error) {
	for {
		signing, err := lock.AcquireContext(ctx, filepath.Join(directory, identity+".lock"))
		if err != nil {
			return nil, patUnavailable(ctx, err)
		}
		held, err := c.useFor(directory, identity, key)
		if err != nil || held {
			if err != nil {
				_ = signing.Release()
				return nil, err
			}
			return signing, nil
		}
		_ = signing.Release()
		if len(c.uses) > 0 {
			// Waiting for one PAT while using another permits lock-order cycles
			// between multi-target commands. Additional credentials fail promptly.
			return nil, patUnavailable(ctx, lock.ErrLocked)
		}
		if err := c.waitForUse(ctx); err != nil {
			return nil, patUnavailable(ctx, err)
		}
		if c.closed {
			return nil, errors.New("command authentication is closed")
		}
	}
}

// useFor makes the command's hold on the PAT's use lock fit key, reporting
// false while other commands' use prevents that. It runs under the PAT's
// sign-in lock, as does every acquisition of the use lock and every change to
// its key, so a hold the command converts cannot be taken in between.
func (c *CommandSessions) useFor(directory, identity, key string) (bool, error) {
	if held, holds := c.uses[identity]; holds {
		if held.key == "" {
			return true, nil
		}
		current, err := readUseKey(directory, identity)
		if err != nil {
			return false, err
		}
		if held.key == key && current == key {
			return true, nil
		}
		if err := c.releaseUse(identity); err != nil {
			return false, err
		}
	}
	path := usePath(directory, identity)
	if key != "" {
		shared, err := lock.TryAcquireShared(path)
		if err != nil && !errors.Is(err, lock.ErrLocked) {
			return false, errors.New("credential coordination lock is unavailable")
		}
		if err == nil {
			current, err := readUseKey(directory, identity)
			if err == nil && current == key {
				c.uses[identity] = patUse{handle: shared, key: key}
				return true, nil
			}
			_ = shared.Release()
			if err != nil {
				return false, err
			}
		}
	}
	exclusive, err := lock.TryAcquire(path)
	if errors.Is(err, lock.ErrLocked) {
		return false, nil
	}
	if err != nil {
		return false, errors.New("credential coordination lock is unavailable")
	}
	if key == "" {
		c.uses[identity] = patUse{handle: exclusive}
		return true, nil
	}
	err = writeUseKey(directory, identity, key)
	_ = exclusive.Release()
	if err != nil {
		return false, err
	}
	shared, err := lock.TryAcquireShared(path)
	if err != nil {
		return false, errors.New("credential coordination lock is unavailable")
	}
	c.uses[identity] = patUse{handle: shared, key: key}
	return true, nil
}

// withholdUse follows a sign-in whose session could not be saved: no other
// command can adopt it, so none may join the PAT's use or sign in with it
// until this command stops using the PAT. Commands already sharing the PAT
// find the key changed and wait before their next sign-in.
func (c *CommandSessions) withholdUse(directory, identity string) error {
	if c.uses[identity].key == "" {
		return nil
	}
	if err := writeUseKey(directory, identity, ""); err != nil {
		return err
	}
	if err := c.releaseUse(identity); err != nil {
		return err
	}
	path := usePath(directory, identity)
	held, err := lock.TryAcquire(path)
	if errors.Is(err, lock.ErrLocked) {
		held, err = lock.TryAcquireShared(path)
	}
	if err != nil {
		return errors.New("credential coordination lock is unavailable")
	}
	c.uses[identity] = patUse{handle: held}
	return nil
}

// waitForUse pauses before the next attempt with the command gate released,
// then takes the gate back even after ctx ends, because the caller holds it.
func (c *CommandSessions) waitForUse(ctx context.Context) error {
	c.leave()
	defer func() { c.gate <- struct{}{} }()
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
	return ctx.Err()
}

func patUnavailable(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, lock.ErrLocked):
		return errors.New("PAT is in use by another command; retry after that command finishes")
	case ctx.Err() != nil:
		return fmt.Errorf("PAT is in use by another command; credential wait ended: %w", ctx.Err())
	default:
		return errors.New("credential coordination lock is unavailable")
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

// saveSession keeps a new sign-in for later commands and reports whether they
// can adopt it.
func (c *CommandSessions) saveSession(ctx context.Context, origin sessionOrigin, identity string, epoch uint64, response SignInResponse) bool {
	store, ok := c.sessionStore(origin)
	if !ok {
		return false
	}
	return store.SaveSession(ctx, CredentialReference(origin.target.CredentialReference), CredentialTarget{ServerURL: origin.target.ServerURL, SiteContentURL: origin.target.SiteContentURL}, SavedSession{Credential: identity, Epoch: epoch, Token: response.Token, SiteLUID: response.SiteLUID, UserLUID: response.UserLUID}) == nil
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

// writeSignInEpoch replaces the epoch file atomically, so a command that stops
// while writing never leaves a partial epoch.
func writeSignInEpoch(directory, key string, epoch uint64) error {
	if !replaceCoordinationFile(directory, signInEpochPath(directory, key), strconv.FormatUint(epoch, 10)+"\n") {
		return errors.New("credential sign-in epoch is unavailable")
	}
	return nil
}

func usePath(directory, identity string) string {
	return filepath.Join(directory, identity+".use")
}

func useKeyPath(directory, identity string) string {
	return filepath.Join(directory, "use-key-"+identity+".txt")
}

// readUseKey returns the key of the commands that may share the PAT's use
// lock, or an empty key when no command may share it.
func readUseKey(directory, identity string) (string, error) {
	data, err := os.ReadFile(useKeyPath(directory, identity))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil || len(data) > 65 {
		return "", errors.New("credential coordination lock is unavailable")
	}
	return strings.TrimSpace(string(data)), nil
}

func writeUseKey(directory, identity, key string) error {
	if !replaceCoordinationFile(directory, useKeyPath(directory, identity), key+"\n") {
		return errors.New("credential coordination lock is unavailable")
	}
	return nil
}

// replaceCoordinationFile replaces path atomically, so a reader never sees
// partial contents.
func replaceCoordinationFile(directory, path, contents string) bool {
	staged, err := os.CreateTemp(directory, "coordination-*.tmp")
	if err != nil {
		return false
	}
	defer os.Remove(staged.Name())
	_, err = staged.WriteString(contents)
	if errors.Join(err, staged.Sync(), staged.Close()) != nil {
		return false
	}
	// A durability failure leaves the new contents visible now; losing them
	// later only makes a command sign in or wait instead of sharing a session.
	err = fsreplace.Replace(staged.Name(), path)
	return err == nil || errors.As(err, new(*fsreplace.DurabilityError))
}

// Close releases all PAT use locks and discards command-local cached state.
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
	failure := c.releaseUses()
	c.credentials, c.sessions, c.uses = nil, nil, nil
	c.resolver = nil
	return failure
}
