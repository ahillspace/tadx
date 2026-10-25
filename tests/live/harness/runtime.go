package harness

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/config"
)

func NewRunID() string {
	var b [10]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("cryptographic randomness unavailable")
	}
	return hex.EncodeToString(b[:])
}
func ConfigurePython(path string) error {
	if path == "" {
		return nil
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return Blocked("configured Python executable is unavailable")
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return err
	}
	return os.Setenv("TADX_LIVECLI_PYTHON", resolved)
}

// Preflight checks the actual candidate's fixed managed policy and current
// saved site consent. It never installs policy or changes persisted consent.
func Preflight(ctx context.Context, p Profile, binary, dir string, capabilities []string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	c := &Case{Binary: binary, ConfigPath: p.ConfigPath, Profile: p, RunDir: dir}
	r, err := c.Command(ctx, "policy", "status", "--full", "--json")
	if err != nil {
		return Blocked("candidate managed-policy status is unavailable")
	}
	state, _ := r.Value["state"].(string)
	if state != "unmanaged" && state != "active" {
		return Blocked("candidate administrator-managed policy is invalid or unavailable; an administrator must repair it")
	}
	allowed, ok := r.Value["remote_mutations"].(bool)
	if !ok || !allowed {
		return Blocked("administrator-managed policy disables fixture and subject remote mutations")
	}
	if state == "active" {
		ids, ok := r.Value["allowed_capability_ids"].([]any)
		if !ok {
			return Blocked("managed policy did not expose capability authorization")
		}
		set := map[string]bool{}
		for _, id := range ids {
			if s, ok := id.(string); ok {
				set[s] = true
			}
		}
		for _, id := range capabilities {
			if !set[id] {
				return Blocked("managed policy denies required subject or fixture capability " + id)
			}
		}
	}
	return p.VerifyConsent()
}

func VerifyIndependentCredentials(ctx context.Context, p Profile) error {
	if p.FixturePAT.NameEnv == "" && p.FixturePAT.CredentialRef == "" {
		return nil
	}
	an, as, err := ResolvePAT(ctx, func() Profile { a := p; a.FixturePAT = Credential{}; return a }())
	if err != nil {
		return err
	}
	fn, fs, err := ResolvePAT(ctx, p)
	if err != nil {
		return err
	}
	if an == fn || as == fs {
		return Blocked("actor and fixture PATs must be distinct")
	}
	return nil
}

func NewCase(ctx context.Context, p Profile, binary, dir, id, run, parent, target string, required []string) (*Case, error) {
	local, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(local, 0o700); err != nil {
		return nil, err
	}
	name := "tadx-live-" + run[:min(len(run), 12)] + "-" + strings.ReplaceAll(strings.TrimPrefix(id, "live."), ".", "-")
	if len(name) > 100 {
		name = name[:100]
	}
	j, err := NewJournal(filepath.Join(local, "journal.json"), run, target, name)
	if err != nil {
		return nil, err
	}
	j.RequiredCapabilities = slices.Clone(required)
	if err = j.save(); err != nil {
		return nil, err
	}
	c := &Case{ID: id, Root: parent, RunDir: local, NamePrefix: name, ConfigPath: filepath.Join(local, "config.yaml"), Environment: p.Environment, Workspace: "live-" + run[:min(10, len(run))], WorkspacePath: filepath.Join(local, "workspace"), TargetFingerprint: target, Profile: p, Fixtures: map[string]Object{}, Journal: j, Binary: binary, RequiredCapabilities: slices.Clone(required)}
	if err = c.makeConfig(); err != nil {
		return c, err
	}
	actor := p
	actor.FixturePAT = Credential{}
	n, s, err := ResolvePAT(ctx, actor)
	if err != nil {
		return c, err
	}
	c.secrets = []string{n, s}
	home := filepath.Join(local, "home")
	if err = os.MkdirAll(home, 0o700); err != nil {
		return c, err
	}
	// These values apply only to candidate subprocesses. The host session and
	// native credential store retain their original user identity.
	c.ExtraEnv = []string{"XDG_CONFIG_HOME=" + filepath.Join(home, "config"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache"), "XDG_STATE_HOME=" + filepath.Join(home, "state")}
	if runtime.GOOS == "windows" {
		c.ExtraEnv = append(c.ExtraEnv, "USERPROFILE="+home, "APPDATA="+filepath.Join(home, "config"), "LOCALAPPDATA="+filepath.Join(home, "cache"))
	} else {
		c.ExtraEnv = append(c.ExtraEnv, "HOME="+home)
	}
	return c, nil
}
func (c *Case) makeConfig() error {
	source, err := config.Load(c.Profile.ConfigPath)
	if err != nil {
		return err
	}
	env, err := source.ResolveEnvironment(c.Environment)
	if err != nil {
		return err
	}
	actual, err := config.CanonicalMutationServer(env.URL)
	if err != nil {
		return err
	}
	wanted, err := config.CanonicalMutationServer(c.Profile.ServerURL)
	if err != nil {
		return err
	}
	if actual != wanted || env.SiteContentURL != c.Profile.SiteContentURL {
		return errors.New("source configuration changed target during preparation")
	}
	env.DefaultWorkspace = ""
	env.Auth = config.Auth{Type: "pat", PATNameEnv: c.Profile.ActorPAT.NameEnv, PATSecretEnv: c.Profile.ActorPAT.SecretEnv, CredentialRef: c.Profile.ActorPAT.CredentialRef}
	consents := []config.SiteMutation{}
	for _, setting := range source.SiteMutations {
		canonical, err := config.CanonicalMutationServer(setting.ServerURL)
		if err != nil {
			return err
		}
		if canonical == wanted && setting.SiteContentURL == c.Profile.SiteContentURL {
			consents = append(consents, setting)
		}
	}
	copy := config.Config{Version: config.CurrentVersion, DefaultEnvironment: c.Environment, Environments: map[string]config.Environment{c.Environment: env}, SiteMutations: consents}
	return config.Save(c.ConfigPath, copy)
}

func (c *Case) BindAPI(api *Client) {
	c.API = api
	if api == nil {
		return
	}
	api.JournalJob = c.AcceptJob
	api.JournalLabelIntent = func(name, parent string, expected map[string]any) error {
		if err := c.Journal.AddIntent("label", name, parent); err != nil {
			return err
		}
		return c.Journal.SetLabelExpectation(name, parent, expected)
	}
	api.AuthorizeWrite = func(ctx context.Context, method, path string) error {
		return Preflight(ctx, c.Profile, c.Binary, c.RunDir, c.RequiredCapabilities)
	}
	api.BeforeRefresh = c.GuardWorkerStopped
}

func (c *Case) AddRequiredCapabilities(ids []string) error {
	c.RequiredCapabilities = append(slices.Clone(c.RequiredCapabilities), ids...)
	slices.Sort(c.RequiredCapabilities)
	c.RequiredCapabilities = slices.Compact(c.RequiredCapabilities)
	if c.Journal == nil {
		return errors.New("durable fixture authorization requires an ownership journal")
	}
	c.Journal.RequiredCapabilities = slices.Clone(c.RequiredCapabilities)
	return c.Journal.save()
}

func (c *Case) CreateWorkspace(ctx context.Context) error {
	_, err := c.Run(ctx, "workspace", "create", c.Workspace, "--path", c.WorkspacePath)
	return err
}

// ResumeCase restores only local context. It never creates remote resources or
// overwrites an existing journal while recovering uncertain mutation outcomes.
func ResumeCase(ctx context.Context, p Profile, binary, dir, target string, api *Client, required []string) (*Case, error) {
	j, err := LoadJournal(filepath.Join(dir, "journal.json"), target)
	if err != nil {
		return nil, err
	}
	commands, err := loadCommandHistory(filepath.Join(dir, "commands.json"))
	if err != nil {
		return nil, err
	}
	if err = recoverOperationClosures(j, commands); err != nil {
		return nil, err
	}
	if len(j.RequiredCapabilities) > 0 {
		required = slices.Clone(j.RequiredCapabilities)
	}
	c := &Case{Root: "", RunDir: dir, NamePrefix: j.Prefix, ConfigPath: filepath.Join(dir, "config.yaml"), Environment: p.Environment, Workspace: "live-" + j.RunID[:min(10, len(j.RunID))], WorkspacePath: filepath.Join(dir, "workspace"), TargetFingerprint: target, Profile: p, Fixtures: map[string]Object{}, Journal: j, Binary: binary, Commands: commands, RequiredCapabilities: required}
	home := filepath.Join(dir, "home")
	c.ExtraEnv = []string{"XDG_CONFIG_HOME=" + filepath.Join(home, "config"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache"), "XDG_STATE_HOME=" + filepath.Join(home, "state")}
	if runtime.GOOS == "windows" {
		c.ExtraEnv = append(c.ExtraEnv, "USERPROFILE="+home, "APPDATA="+filepath.Join(home, "config"), "LOCALAPPDATA="+filepath.Join(home, "cache"))
	} else {
		c.ExtraEnv = append(c.ExtraEnv, "HOME="+home)
	}
	c.BindAPI(api)
	actor := p
	actor.FixturePAT = Credential{}
	name, secret, err := ResolvePAT(ctx, actor)
	if err != nil {
		return nil, err
	}
	c.secrets = []string{name, secret}
	return c, nil
}

type operationClosureCandidate struct {
	phase   string
	command int
}

func recoverOperationClosures(j *Journal, commands []Command) error {
	accepted := make(map[string]bool)
	candidates := make(map[string]operationClosureCandidate)
	touched := make(map[string]bool)
	for index, command := range commands {
		if mayStartNativeTransfer(command.Args) {
			for _, id := range operationIDs(command.Value) {
				if slices.Contains(j.Operations, id) {
					accepted[id] = true
					touched[id] = true
					delete(candidates, id)
				}
			}
		}
		id, isInspection := inspectedOperationID(command.Args)
		if !isInspection || command.ExitCode != 0 || !accepted[id] {
			continue
		}
		touched[id] = true
		actual, _ := command.Value["operation_id"].(string)
		tracking, _ := command.Value["tracking"].(map[string]any)
		alive, observed := tracking["worker_alive"].(bool)
		phase, _ := tracking["phase"].(string)
		if actual == id && observed && !alive && (phase == "completed" || phase == "failed") {
			candidates[id] = operationClosureCandidate{phase: phase, command: index + 1}
		} else {
			delete(candidates, id)
		}
	}
	for id := range touched {
		candidate, hasCandidate := candidates[id]
		if hasCandidate {
			if err := j.closeOperationFromHistory(id, candidate.phase, candidate.command); err != nil {
				return err
			}
			continue
		}
		j.mu.Lock()
		_, existed := j.OperationClosures[id]
		delete(j.OperationClosures, id)
		var err error
		if existed {
			err = j.save()
		}
		j.mu.Unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

func operationIDs(value any) []string {
	var ids []string
	var walk func(any)
	walk = func(value any) {
		switch current := value.(type) {
		case map[string]any:
			for key, child := range current {
				if id, ok := child.(string); ok && key == "operation_id" && id != "" {
					ids = append(ids, id)
				}
				walk(child)
			}
		case []any:
			for _, child := range current {
				walk(child)
			}
		}
	}
	walk(value)
	return ids
}

func inspectedOperationID(args []string) (string, bool) {
	if len(args) < 2 || args[0] != "job" || args[1] != "inspect" || !slices.Contains(args, "--full") {
		return "", false
	}
	id := ""
	for index := 2; index < len(args); index++ {
		if args[index] != "--operation-id" {
			continue
		}
		if id != "" || index+1 >= len(args) || args[index+1] == "" {
			return "", false
		}
		id = args[index+1]
		index++
	}
	return id, id != ""
}

func loadCommandHistory(path string) ([]Command, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("command history is not a regular local file")
	}
	if info.Size() > 64<<20 {
		return nil, errors.New("command history exceeds size limit")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || data[0] != '[' {
		return nil, errors.New("invalid command history")
	}
	var commands []Command
	if err = json.Unmarshal(data, &commands); err != nil || commands == nil {
		return nil, errors.New("invalid command history")
	}
	for _, command := range commands {
		if len(command.Args) == 0 {
			return nil, errors.New("command history contains an entry without arguments")
		}
	}
	return commands, nil
}

func (c *Case) cleanLocalState() error {
	root, err := filepath.Abs(c.RunDir)
	if err != nil {
		return err
	}
	for _, name := range []string{"workspace", "destination-workspace", "home"} {
		path := filepath.Join(root, name)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("local cleanup refuses a substituted workspace or home directory")
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, resolved)
		if err != nil || rel != name {
			return errors.New("local cleanup path escaped the owned case directory")
		}
		if err = os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}

func targetCoordinationDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cache, "tadx-live", "locks")
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// TargetStateRegistryPath shares the lock's user-wide namespace across checkouts.
func TargetStateRegistryPath(target string) (string, error) {
	dir, err := targetCoordinationDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, target+".states.json"), nil
}

func AcquireTargetLock(target string) (func() error, error) {
	dir, err := targetCoordinationDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, target+".lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, Blocked("target is locked by another live-suite phase; inspect its ownership before removing a stale lock")
	}
	if _, err = fmt.Fprintf(f, "pid=%d\n", os.Getpid()); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	return func() error { return os.Remove(path) }, nil
}
