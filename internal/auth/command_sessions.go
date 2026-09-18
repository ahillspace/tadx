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

// CommandSessions owns ephemeral sessions and credential locks for one command.
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
	sessions          map[sessionKey]sessionResult
}

type credentialSourceKey struct{ server, site, nameVariable, secretVariable, reference string }
type credentialResult struct {
	value PATCredentials
	err   error
}
type sessionKey struct{ credential, site string }
type sessionResult struct {
	session Session
	err     error
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
	return &CommandSessions{gate: make(chan struct{}, 1), gateChanged: make(chan struct{}), resolver: NewPATSourceResolver(lookup, store), directory: directory, credentials: make(map[credentialSourceKey]credentialResult), locks: make(map[string]*lock.Handle), sessions: make(map[sessionKey]sessionResult)}
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
	session, _, err := c.AuthenticateWithSource(ctx, target, signer)
	return session, err
}

// AuthenticateWithSource retains the source selected for this exact attempt,
// including an upstream sign-in failure, without resolving credentials twice.
func (c *CommandSessions) AuthenticateWithSource(ctx context.Context, target Target, signer Signer) (Session, CredentialSource, error) {
	return c.authenticateWithSource(ctx, target, signer, false)
}

// AuthenticateMonitor authenticates a read-only job observation. It yields to
// foreground authentication when both are waiting for the same PAT lock.
func (c *CommandSessions) AuthenticateMonitor(ctx context.Context, target Target, signer Signer) (Session, error) {
	session, _, err := c.authenticateWithSource(ctx, target, signer, true)
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

func (c *CommandSessions) authenticateWithSource(ctx context.Context, target Target, signer Signer, monitor bool) (Session, CredentialSource, error) {
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
		session, err := c.authenticate(ctx, target, credentials, signer, server, monitor)
		c.leave()
		if errors.Is(err, errMonitorCredentialBusy) {
			if err := waitMonitorRetry(ctx); err != nil {
				return nil, credentials.Source, err
			}
			continue
		}
		return session, credentials.Source, err
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
	return c.authenticate(ctx, target, credentials, signer, server, false)
}

func (c *CommandSessions) authenticate(ctx context.Context, target Target, credentials PATCredentials, signer Signer, server string, monitor bool) (Session, error) {
	if err := validateAuthenticationInputs(target, credentials); err != nil {
		return nil, err
	}
	identity := credentialIdentity(server, credentials)
	coordinationKey := coordinationIdentity(server, target.SiteContentURL, credentials)
	key := sessionKey{identity, target.SiteContentURL}
	if previous, ok := c.sessions[key]; ok && previous.session != nil && c.locks[identity] != nil {
		return previous.session, nil
	}
	newLock := false
	if c.locks[identity] == nil {
		held, err := c.acquireCredentialLock(ctx, identity, monitor)
		if err != nil {
			if errors.Is(err, errMonitorCredentialBusy) {
				return nil, errMonitorCredentialBusy
			}
			if errors.Is(err, lock.ErrLocked) {
				return nil, errors.New("PAT is in use by another command; retry after that command finishes")
			}
			if ctx.Err() != nil {
				return nil, fmt.Errorf("PAT is in use by another command; credential wait ended: %w", ctx.Err())
			}
			return nil, errors.New("credential coordination lock is unavailable")
		}
		c.locks[identity] = held
		newLock = true
	}
	directory, err := c.lockDirectory()
	if err != nil {
		if newLock {
			_ = c.locks[identity].Release()
			delete(c.locks, identity)
		}
		return nil, err
	}
	epoch, err := readSignInEpoch(directory, coordinationKey)
	if err != nil {
		if newLock {
			_ = c.locks[identity].Release()
			delete(c.locks, identity)
		}
		return nil, err
	}
	if previous, ok := c.sessions[key]; ok && previous.session != nil && previous.epoch == epoch {
		return previous.session, nil
	}
	// A new site sign-in may invalidate the same PAT's earlier site session.
	// Never retain a prior site session as a reusable authenticated target.
	for previous := range c.sessions {
		if previous.credential == identity {
			delete(c.sessions, previous)
		}
	}
	session, err := signIn(ctx, target, credentials, signer)
	if err != nil {
		c.sessions[key] = sessionResult{err: err, epoch: epoch}
		return nil, err
	}
	if epoch == ^uint64(0) {
		return nil, errors.New("credential sign-in epoch is exhausted")
	}
	epoch++
	if err := writeSignInEpoch(directory, coordinationKey, epoch); err != nil {
		return nil, err
	}
	c.sessions[key] = sessionResult{session: session, epoch: epoch}
	return session, err
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

func (c *CommandSessions) acquireCredentialLock(ctx context.Context, identity string, monitor bool) (*lock.Handle, error) {
	directory, err := c.lockDirectory()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(directory, identity+".lock")
	if monitor {
		held, err := lock.TryAcquire(path)
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
