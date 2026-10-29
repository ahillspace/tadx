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
	"sync"
	"time"

	"github.com/ahillspace/tadx/internal/lock"
)

// CommandSessions owns one command's sessions and credential locks. Sessions
// of a stored PAT are saved beside it for later commands and renewed in place
// when Tableau rejects them.
// Close must run after every request using these sessions has finished.
type CommandSessions struct {
	gate              chan struct{}
	priorityMu        sync.Mutex
	gateChanged       chan struct{}
	foregroundWaiters int
	resolver          *PATSourceResolver
	directory         string
	closed            bool
	credentials       map[credentialSourceKey]credentialResult
	locks             map[string]*lock.Handle
	sessions          map[sessionKey]sessionEntry
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

var errMonitorCredentialBusy = errors.New("monitor credential is busy")

const monitorCredentialRetry = 25 * time.Millisecond

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
	return &CommandSessions{gate: make(chan struct{}, 1), gateChanged: make(chan struct{}), resolver: NewPATSourceResolver(lookup, store), directory: directory, credentials: make(map[credentialSourceKey]credentialResult), locks: make(map[string]*lock.Handle), sessions: make(map[sessionKey]sessionEntry)}
}

func (c *CommandSessions) enter(ctx context.Context, monitor bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !monitor {
		c.priorityMu.Lock()
		c.foregroundWaiters++
		c.priorityMu.Unlock()
	}
	for {
		c.priorityMu.Lock()
		if monitor && c.foregroundWaiters > 0 {
			changed := c.gateChanged
			c.priorityMu.Unlock()
			if err := waitForGateChange(ctx, changed); err != nil {
				return err
			}
			continue
		}
		select {
		case c.gate <- struct{}{}:
			if !monitor {
				c.foregroundWaiters--
			}
			c.priorityMu.Unlock()
			return nil
		default:
			changed := c.gateChanged
			c.priorityMu.Unlock()
			if err := waitForGateChange(ctx, changed); err != nil {
				if !monitor {
					c.priorityMu.Lock()
					c.foregroundWaiters--
					closeGateWaiters(c)
					c.priorityMu.Unlock()
				}
				return err
			}
		}
	}
}

func waitForGateChange(ctx context.Context, changed <-chan struct{}) error {
	select {
	case <-changed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func waitMonitorRetry(ctx context.Context) error {
	timer := time.NewTimer(monitorCredentialRetry)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func closeGateWaiters(c *CommandSessions) {
	close(c.gateChanged)
	c.gateChanged = make(chan struct{})
}

func (c *CommandSessions) leave() {
	<-c.gate
	c.priorityMu.Lock()
	closeGateWaiters(c)
	c.priorityMu.Unlock()
}

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

// AuthenticateMonitor authenticates a read-only job observation. It yields to
// foreground authentication when both are waiting for the same PAT lock.
func (c *CommandSessions) AuthenticateMonitor(ctx context.Context, target Target, signer Signer) (Session, error) {
	session, _, err := c.authenticateWithSource(ctx, target, signer, authenticateMonitor)
	return session, err
}

// CoordinationKey returns an opaque identity for the resolved PAT, normalized
// server, and exact site. It contains no credential or session material.
func (c *CommandSessions) CoordinationKey(ctx context.Context, target Target) (string, error) {
	if err := c.enter(ctx, false); err != nil {
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

// Suspend releases held PAT locks while retaining command-local credentials
// and sessions for a later monitor check. It waits for active authentication
// work and yields to queued foreground authentication.
func (c *CommandSessions) Suspend(ctx context.Context) error {
	if err := c.enter(ctx, true); err != nil {
		return err
	}
	defer c.leave()
	if c.closed {
		return errors.New("command authentication is closed")
	}
	var failure error
	for identity, held := range c.locks {
		if err := held.Release(); err != nil {
			failure = errors.New("credential coordination lock release failed")
		}
		delete(c.locks, identity)
	}
	return failure
}

// authenticateMode selects how an authentication call treats an existing
// session and the PAT lock.
type authenticateMode int

const (
	// authenticateReuse reuses a current session and waits for the PAT lock.
	authenticateReuse authenticateMode = iota
	// authenticateMonitor reuses a current session and yields the PAT lock to
	// foreground work.
	authenticateMonitor
	// authenticateSignIn always signs in and waits for the PAT lock.
	authenticateSignIn
)

func (c *CommandSessions) authenticateWithSource(ctx context.Context, target Target, signer Signer, mode authenticateMode) (Session, CredentialSource, error) {
	monitor := mode == authenticateMonitor
	for {
		if err := c.enter(ctx, monitor); err != nil {
			return nil, "", err
		}
		if c.closed {
			c.leave()
			return nil, "", errors.New("command authentication is closed")
		}
		_, server, credentials, err := c.resolveCredentials(ctx, target)
		if err != nil {
			c.leave()
			return nil, credentials.Source, err
		}
		session, err := c.authenticate(ctx, sessionOrigin{owner: c, target: target, credentials: credentials, signer: signer, server: server}, mode)
		c.leave()
		if errors.Is(err, errMonitorCredentialBusy) {
			if err := waitMonitorRetry(ctx); err != nil {
				return nil, credentials.Source, err
			}
			continue
		}
		if err != nil {
			return nil, credentials.Source, err
		}
		return session, credentials.Source, nil
	}
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
	if err := c.enter(ctx, false); err != nil {
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
	session, err := c.authenticate(ctx, sessionOrigin{owner: c, target: target, credentials: credentials, signer: signer, server: server}, authenticateReuse)
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

// renew signs a rejected command session in again under the command gate.
// Renewal serves a request that already holds or held the PAT, so it waits
// for the PAT lock like foreground work instead of yielding.
func (o sessionOrigin) renew(ctx context.Context, current *session, rejected string) error {
	c := o.owner
	if err := c.enter(ctx, false); err != nil {
		return err
	}
	defer c.leave()
	if c.closed {
		return errors.New("command authentication is closed")
	}
	if current.currentToken() != rejected {
		return nil
	}
	_, err := c.authenticate(ctx, o, authenticateSignIn)
	return err
}

// authenticate returns the command's one session for a PAT and site. The
// session is reused while the PAT's sign-in epoch is unchanged; otherwise the
// PAT signs in again and the same session is updated in place.
func (c *CommandSessions) authenticate(ctx context.Context, origin sessionOrigin, mode authenticateMode) (*session, error) {
	if err := validateAuthenticationInputs(origin.target, origin.credentials); err != nil {
		return nil, err
	}
	identity := credentialIdentity(origin.server, origin.credentials)
	key := sessionKey{identity, origin.target.SiteContentURL}
	directory, epoch, err := c.holdCredential(ctx, identity, mode == authenticateMonitor)
	if err != nil {
		return nil, err
	}
	entry, exists := c.sessions[key]
	if mode != authenticateSignIn {
		if exists && entry.epoch == epoch {
			return entry.session, nil
		}
		if !exists {
			if saved, ok := c.loadSavedSession(ctx, origin, identity, epoch); ok {
				entry = sessionEntry{session: newSession(saved, origin), epoch: epoch}
				c.sessions[key] = entry
				return entry.session, nil
			}
		}
	}
	// Signing in with the same PAT invalidates its sessions across sites.
	// Epochs share the credential lock's scope, not the site-specific job pool.
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
	if exists {
		if err := entry.session.replace(response); err != nil {
			return nil, err
		}
	} else {
		entry.session = newSession(response, origin)
	}
	c.sessions[key] = sessionEntry{session: entry.session, epoch: epoch}
	c.saveSession(ctx, origin, identity, epoch, response)
	return entry.session, nil
}

// holdCredential takes the PAT lock if this command does not hold it and
// reads the PAT's sign-in epoch. A lock taken here is released on failure.
func (c *CommandSessions) holdCredential(ctx context.Context, identity string, monitor bool) (string, uint64, error) {
	newLock := false
	if c.locks[identity] == nil {
		held, err := c.acquireCredentialLock(ctx, identity, monitor)
		if err != nil {
			if errors.Is(err, errMonitorCredentialBusy) {
				return "", 0, errMonitorCredentialBusy
			}
			if errors.Is(err, lock.ErrLocked) {
				return "", 0, errors.New("PAT is in use by another command; retry after that command finishes")
			}
			if ctx.Err() != nil {
				return "", 0, fmt.Errorf("PAT is in use by another command; credential wait ended: %w", ctx.Err())
			}
			return "", 0, errors.New("credential coordination lock is unavailable")
		}
		c.locks[identity] = held
		newLock = true
	}
	directory, err := c.lockDirectory()
	if err == nil {
		var epoch uint64
		if epoch, err = readSignInEpoch(directory, identity); err == nil {
			return directory, epoch, nil
		}
	}
	if newLock {
		_ = c.locks[identity].Release()
		delete(c.locks, identity)
	}
	return "", 0, err
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

func (c *CommandSessions) acquireCredentialLock(ctx context.Context, identity string, monitor bool) (held *lock.Handle, err error) {
	directory, err := c.lockDirectory()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(directory, identity+".lock")
	admissionPath := filepath.Join(directory, identity+".admission.lock")
	var admission *lock.Handle
	if monitor {
		admission, err = lock.TryAcquire(admissionPath)
	} else if len(c.locks) == 0 {
		admission, err = lock.AcquireSharedContext(ctx, admissionPath)
	} else {
		admission, err = lock.TryAcquireShared(admissionPath)
	}
	if err != nil {
		if monitor && errors.Is(err, lock.ErrLocked) {
			return nil, errMonitorCredentialBusy
		}
		return nil, err
	}
	// Every foreground waiter holds shared admission until it acquires the
	// credential lease. A monitor needs exclusive admission and never waits
	// while holding it, so it cannot pass registered foreground work. Kernel
	// locks also remove registrations when an independent process exits.
	defer func() {
		if releaseErr := admission.Release(); releaseErr != nil {
			err = errors.Join(err, releaseErr, held.Release())
			held = nil
		}
	}()
	if monitor {
		held, err = lock.TryAcquire(path)
		if errors.Is(err, lock.ErrLocked) {
			return nil, errMonitorCredentialBusy
		}
		return held, err
	}
	if len(c.locks) == 0 {
		return lock.AcquireContext(ctx, path)
	}
	// Holding one PAT while waiting for another permits lock-order cycles
	// between multi-target commands. Additional credentials fail promptly.
	return lock.TryAcquire(path)
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

func writeSignInEpoch(directory, key string, epoch uint64) error {
	path := signInEpochPath(directory, key)
	if err := os.WriteFile(path, []byte(strconv.FormatUint(epoch, 10)+"\n"), 0600); err != nil {
		return errors.New("credential sign-in epoch is unavailable")
	}
	return nil
}

// Close releases all credential locks and discards command-local cached state.
func (c *CommandSessions) Close() error {
	if c == nil {
		return nil
	}
	if err := c.enter(context.Background(), false); err != nil {
		return err
	}
	defer c.leave()
	if c.closed {
		return nil
	}
	c.closed = true
	var failure error
	for _, held := range c.locks {
		if err := held.Release(); err != nil {
			failure = errors.New("credential coordination lock release failed")
		}
	}
	c.credentials, c.sessions, c.locks = nil, nil, nil
	c.resolver = nil
	return failure
}
