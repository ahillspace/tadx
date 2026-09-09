package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ahillspace/tadx/internal/lock"
)

// CommandSessions owns ephemeral sessions and credential locks for one command.
// Close must run after every request using these sessions has finished.
type CommandSessions struct {
	gate        chan struct{}
	resolver    *PATSourceResolver
	directory   string
	closed      bool
	credentials map[credentialSourceKey]credentialResult
	locks       map[string]*lock.Handle
	sessions    map[sessionKey]sessionResult
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
	return &CommandSessions{gate: make(chan struct{}, 1), resolver: NewPATSourceResolver(lookup, store), directory: directory, credentials: make(map[credentialSourceKey]credentialResult), locks: make(map[string]*lock.Handle), sessions: make(map[sessionKey]sessionResult)}
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

// Authenticate resolves each source once and reuses only the same actual PAT,
// normalized server, and exact site. Session requests do not acquire this gate.
func (c *CommandSessions) Authenticate(ctx context.Context, target Target, signer Signer) (Session, error) {
	if err := c.enter(ctx); err != nil {
		return nil, err
	}
	defer func() { <-c.gate }()
	if c.closed {
		return nil, errors.New("command authentication is closed")
	}
	server, err := normalizeCredentialServerTarget(target.ServerURL)
	if err != nil {
		return nil, err
	}
	key := credentialSourceKey{server, target.SiteContentURL, target.PATNameVariable, target.PATSecretVariable, target.CredentialReference}
	resolved, ok := c.credentials[key]
	if !ok {
		resolved.value, resolved.err = c.resolver.Resolve(ctx, target)
		c.credentials[key] = resolved
	}
	if resolved.err != nil {
		return nil, resolved.err
	}
	return c.authenticate(ctx, target, resolved.value, signer, server)
}

// AuthenticateCredentials validates an explicitly provided login PAT through
// the same credential coordination boundary without persisting credentials.
func (c *CommandSessions) AuthenticateCredentials(ctx context.Context, target Target, credentials PATCredentials, signer Signer) (Session, error) {
	if err := c.enter(ctx); err != nil {
		return nil, err
	}
	defer func() { <-c.gate }()
	if c.closed {
		return nil, errors.New("command authentication is closed")
	}
	server, err := normalizeCredentialServerTarget(target.ServerURL)
	if err != nil {
		return nil, err
	}
	return c.authenticate(ctx, target, credentials, signer, server)
}

func (c *CommandSessions) authenticate(ctx context.Context, target Target, credentials PATCredentials, signer Signer, server string) (Session, error) {
	if target.SiteContentURL != strings.TrimSpace(target.SiteContentURL) {
		return nil, errors.New("authentication site must not contain surrounding whitespace")
	}
	if strings.TrimSpace(credentials.Name) == "" || strings.TrimSpace(credentials.Secret) == "" {
		return nil, errors.New("a complete PAT pair is required")
	}
	hash := sha256.New()
	for _, part := range []string{"tadx-pat-lock-v1", server, credentials.Name, credentials.Secret} {
		_, _ = fmt.Fprintf(hash, "%d:%s", len(part), part)
	}
	identity := fmt.Sprintf("%x", hash.Sum(nil))
	key := sessionKey{identity, target.SiteContentURL}
	if previous, ok := c.sessions[key]; ok {
		return previous.session, previous.err
	}
	if c.locks[identity] == nil {
		directory := c.directory
		if directory == "" {
			cache, err := os.UserCacheDir()
			if err != nil {
				return nil, errors.New("credential coordination directory is unavailable")
			}
			directory = filepath.Join(cache, "tadx", "credential-locks")
		}
		if err := os.MkdirAll(directory, 0700); err != nil {
			return nil, errors.New("credential coordination directory is unavailable")
		}
		path := filepath.Join(directory, identity+".lock")
		var held *lock.Handle
		var err error
		if len(c.locks) == 0 {
			held, err = lock.AcquireContext(ctx, path)
		} else {
			// Holding one PAT while waiting for another permits lock-order cycles
			// between multi-target commands. Additional credentials fail promptly.
			held, err = lock.TryAcquire(path)
		}
		if err != nil {
			if errors.Is(err, lock.ErrLocked) {
				return nil, errors.New("PAT is in use by another command; retry after that command finishes")
			}
			if ctx.Err() != nil {
				return nil, fmt.Errorf("PAT is in use by another command; credential wait ended: %w", ctx.Err())
			}
			return nil, errors.New("credential coordination lock is unavailable")
		}
		c.locks[identity] = held
	}
	// A new site sign-in may invalidate the same PAT's earlier site session.
	// Never retain a prior site session as a reusable authenticated target.
	for previous := range c.sessions {
		if previous.credential == identity {
			delete(c.sessions, previous)
		}
	}
	session, err := signIn(ctx, target, credentials, signer)
	c.sessions[key] = sessionResult{session, err}
	return session, err
}

// Close releases all credential locks and discards command-local cached state.
func (c *CommandSessions) Close() error {
	if c == nil {
		return nil
	}
	c.gate <- struct{}{}
	defer func() { <-c.gate }()
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
