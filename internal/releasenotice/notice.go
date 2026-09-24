// Package releasenotice provides a passive, best-effort CLI release notice.
package releasenotice

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ahillspace/tadx/internal/version"
	"github.com/mattn/go-isatty"
)

const cadence = 24 * time.Hour
const requestTimeout = 1500 * time.Millisecond

var stableVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// Options holds the small set of inputs needed by the entrypoint. Optional
// functions allow tests to run without a terminal, network, or user cache.
type Options struct {
	Args       []string
	Current    string
	Modified   bool
	Stdout     io.Writer
	Stderr     io.Writer
	IsTerminal func(io.Writer) bool
	Env        func(string) string
	CacheDir   func() (string, error)
	Now        func() time.Time
	Latest     func(context.Context) (version.Release, error)
}

type record struct {
	Checked time.Time `json:"checked"`
	Noticed time.Time `json:"noticed,omitzero"`
	Version string    `json:"version,omitempty"`
	URL     string    `json:"url,omitempty"`
}

type result struct {
	release version.Release
	err     error
}

// Check owns one in-flight release request, or a completed cached check.
type Check struct {
	options Options
	path    string
	record  record
	results chan result
	done    chan struct{}
	cancel  context.CancelFunc
}

// Start never blocks on the network. Ineligible invocations have no cache or
// network side effects. A canceled request does not stamp the 24-hour cadence.
func Start(ctx context.Context, options Options) *Check {
	if options.Current == "" {
		options.Current = version.Current()
		options.Modified = version.Modified()
	}
	if !eligible(options) {
		return nil
	}
	cacheDir := options.CacheDir
	if cacheDir == nil {
		cacheDir = os.UserCacheDir
	}
	root, err := cacheDir()
	if err != nil || root == "" {
		return nil
	}
	directory := filepath.Join(root, "tadx")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil
	}
	check := &Check{options: options, path: filepath.Join(directory, "release-notice.json")}
	check.record = readRecord(check.path)
	if elapsed := check.now().Sub(check.record.Checked); elapsed >= 0 && elapsed < cadence {
		return check
	}
	requestContext, cancel := context.WithTimeout(ctx, requestTimeout)
	check.cancel = cancel
	check.results = make(chan result, 1)
	check.done = make(chan struct{})
	latest := options.Latest
	if latest == nil {
		latest = (version.Checker{Client: publicClient()}).Latest
	}
	go func() {
		release, err := latest(requestContext)
		check.results <- result{release: release, err: err}
		close(check.done)
	}()
	return check
}

// Done is intended for deterministic tests. Production callers do not wait.
func (check *Check) Done() <-chan struct{} {
	if check == nil {
		return nil
	}
	return check.done
}

// Finish cancels unfinished work immediately and returns a notice only for a
// successful command. The caller writes that notice to stderr.
func (check *Check) Finish(success bool) string {
	if check == nil {
		return ""
	}
	if check.results != nil {
		select {
		case completed := <-check.results:
			check.cancel()
			if errors.Is(completed.err, context.Canceled) {
				return ""
			}
			check.record = record{Checked: check.now()}
			if completed.err == nil && validRelease(completed.release) {
				check.record.Version = completed.release.Version
				check.record.URL = completed.release.URL
			}
			if !writeRecord(check.path, check.record) {
				return ""
			}
		default:
			check.cancel()
			return ""
		}
	}
	if !success || !newerStable(check.options.Current, check.record.Version) ||
		!validRelease(version.Release{Version: check.record.Version, URL: check.record.URL}) {
		return ""
	}
	if elapsed := check.now().Sub(check.record.Noticed); elapsed >= 0 && elapsed < cadence {
		return ""
	}
	check.record.Noticed = check.now()
	if !writeRecord(check.path, check.record) {
		return ""
	}
	return "A new TADX release is available: v" + check.record.Version + " (current: v" + check.options.Current + "). Run `tadx update` to install it.\n"
}

func eligible(options Options) bool {
	if options.Modified || !stableVersion.MatchString(options.Current) || len(options.Args) == 0 {
		return false
	}
	terminal := options.IsTerminal
	if terminal == nil {
		terminal = interactiveTerminal
	}
	if !terminal(options.Stdout) || !terminal(options.Stderr) {
		return false
	}
	env := options.Env
	if env == nil {
		env = os.Getenv
	}
	if env("CI") != "" || env("TADX_NO_UPDATE_NOTIFIER") != "" {
		return false
	}
	command := ""
	for index := 0; index < len(options.Args); index++ {
		arg := options.Args[index]
		if arg == "--help" || arg == "-h" || arg == "--json" || strings.HasPrefix(arg, "--json=") ||
			arg == "--jsn" || strings.HasPrefix(arg, "--jsn=") ||
			arg == "--preview" || strings.HasPrefix(arg, "--preview=") ||
			arg == "--pv" || strings.HasPrefix(arg, "--pv=") || arg == "-p" || strings.HasPrefix(arg, "-p=") ||
			arg == "--cache" || strings.HasPrefix(arg, "--cache=") ||
			arg == "--cch" || strings.HasPrefix(arg, "--cch=") {
			return false
		}
		if arg == "--config" || arg == "--cfg" || arg == "--environment" || arg == "--env" || arg == "-e" {
			index++
			continue
		}
		if !strings.HasPrefix(arg, "-") && command == "" {
			command = arg
		}
	}
	return command != "" && command != "update" && command != "upd" && command != "completion" && command != "cmp" &&
		command != "cache" && command != "cch" && command != "help" &&
		!strings.HasPrefix(command, "__complete")
}

func interactiveTerminal(writer io.Writer) bool {
	fdWriter, ok := writer.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	fd := fdWriter.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

func newerStable(current, latest string) bool {
	currentParts := stableVersion.FindStringSubmatch(current)
	latestParts := stableVersion.FindStringSubmatch(latest)
	if currentParts == nil || latestParts == nil {
		return false
	}
	for index := 1; index <= 3; index++ {
		left, right := currentParts[index], latestParts[index]
		if len(left) != len(right) {
			return len(right) > len(left)
		}
		if left != right {
			return right > left
		}
	}
	return false
}

func validRelease(release version.Release) bool {
	return stableVersion.MatchString(release.Version) &&
		release.URL == "https://github.com/ahillspace/tadx/releases/tag/v"+release.Version
}

func readRecord(path string) record {
	file, err := os.Open(path)
	if err != nil {
		return record{}
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 2049))
	if err != nil || len(data) > 2048 {
		return record{}
	}
	var cached record
	if json.Unmarshal(data, &cached) != nil {
		return record{}
	}
	return cached
}

func writeRecord(path string, cached record) bool {
	data, err := json.Marshal(cached)
	if err != nil {
		return false
	}
	file, err := os.CreateTemp(filepath.Dir(path), "release-notice-*")
	if err != nil {
		return false
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return false
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return false
	}
	if err := file.Close(); err != nil {
		return false
	}
	return os.Rename(file.Name(), path) == nil
}

func (check *Check) now() time.Time {
	if check.options.Now != nil {
		return check.options.Now()
	}
	return time.Now()
}

func publicClient() *http.Client {
	return &http.Client{
		Timeout:       requestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
