// Command runner selects and runs opt-in live Tableau CLI contracts.
package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/ahillspace/tadx/tests/live"
	"github.com/ahillspace/tadx/tests/live/harness"
)

type repeated []string

func (r *repeated) String() string { return strings.Join(*r, ",") }
func (r *repeated) Set(v string) error {
	if v == "" {
		return errors.New("selectors must not be empty")
	}
	*r = append(*r, v)
	return nil
}

type options struct {
	target, binary, state, selection, out, base, environment, config, run, python string
	ids, caps, groups, paths                                                      repeated
}

func parse(args []string) (string, options, error) {
	var o options
	if len(args) == 0 {
		return "", o, errors.New("choose catalog, init, plan, setup, verify-setup, run, or cleanup")
	}
	verb := args[0]
	f := flag.NewFlagSet("live "+verb, flag.ContinueOnError)
	f.SetOutput(os.Stderr)
	f.StringVar(&o.target, "target-file", "", "ignored JSON target profile")
	f.StringVar(&o.binary, "binary", "", "explicit candidate executable")
	f.StringVar(&o.state, "state-dir", ".tadx-live", "ignored local state directory")
	f.StringVar(&o.selection, "selection", "", "selection JSON from plan")
	f.StringVar(&o.out, "out", "", "output JSON path")
	f.StringVar(&o.base, "changed-from", "", "Git base commit for affected tests")
	f.StringVar(&o.environment, "environment", "", "existing environment to import without credentials")
	f.StringVar(&o.config, "config", "", "source non-secret TADX configuration")
	f.StringVar(&o.run, "run", "", "exact generated run ID for cleanup")
	f.StringVar(&o.python, "python", "", "Python executable containing tableauhyperapi")
	f.Var(&o.ids, "id", "exact stable case ID; repeatable")
	f.Var(&o.caps, "capability", "exact capability ID; repeatable")
	f.Var(&o.groups, "group", "resource group; repeatable")
	f.Var(&o.paths, "changed-path", "repository-relative changed owner; repeatable")
	if err := f.Parse(args[1:]); err != nil {
		return verb, o, err
	}
	if f.NArg() != 0 {
		return verb, o, errors.New("unexpected positional arguments")
	}
	return verb, o, nil
}

func loadPlan(o options) (live.Plan, error) {
	if o.selection != "" {
		if len(o.ids)+len(o.caps)+len(o.groups)+len(o.paths) > 0 || o.base != "" {
			return live.Plan{}, errors.New("selection file cannot be combined with selectors")
		}
		data, err := os.ReadFile(o.selection)
		if err != nil {
			return live.Plan{}, err
		}
		var p live.Plan
		if err = json.Unmarshal(data, &p); err != nil {
			return p, err
		}
		return p, p.Validate()
	}
	paths := append([]string(nil), o.paths...)
	if o.base != "" {
		cmd := exec.Command("git", "diff", "--name-only", "-z", o.base, "--")
		output, err := cmd.Output()
		if err != nil {
			return live.Plan{}, errors.New("resolve changed Git paths failed")
		}
		for _, p := range strings.Split(string(output), "\x00") {
			if p != "" {
				paths = append(paths, p)
			}
		}
		untracked, err := exec.Command("git", "ls-files", "--others", "--exclude-standard", "-z").Output()
		if err != nil {
			return live.Plan{}, err
		}
		for _, p := range strings.Split(string(untracked), "\x00") {
			if p != "" {
				paths = append(paths, p)
			}
		}
	}
	return live.Select(live.Selectors{IDs: o.ids, Capabilities: o.caps, Groups: o.groups, ChangedPaths: paths})
}

func emit(path string, value any) error {
	if path != "" {
		return harness.WriteJSON(path, value)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func stateDirectory(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	for current := absolute; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return errors.New("state directory must not traverse symbolic links")
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		next := filepath.Dir(current)
		if next == current {
			break
		}
	}
	git := exec.Command("git", "rev-parse", "--show-toplevel")
	data, err := git.Output()
	if err == nil {
		root := strings.TrimSpace(string(data))
		relative, err := filepath.Rel(root, absolute)
		if err != nil {
			return err
		}
		if filepath.IsLocal(relative) {
			check := exec.Command("git", "check-ignore", "--quiet", "--", filepath.Join(absolute, "manifest.json"))
			if check.Run() != nil {
				return errors.New("live state inside the repository must be ignored by Git")
			}
		}
	}
	return os.MkdirAll(absolute, 0o700)
}

func execute(ctx context.Context, verb string, o options) error {
	if o.python != "" {
		path, err := exec.LookPath(o.python)
		if err != nil {
			return errors.New("selected Python executable is unavailable")
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return err
		}
		o.python = path
	}
	switch verb {
	case "catalog":
		c, err := live.Catalog()
		if err != nil {
			return err
		}
		return emit(o.out, c)
	case "init":
		if o.environment == "" || o.out == "" {
			return errors.New("init requires --environment and an ignored --out profile")
		}
		if err := stateDirectory(filepath.Dir(o.out)); err != nil {
			return err
		}
		p, err := harness.ProfileFromEnvironment(o.config, o.environment)
		if err != nil {
			return err
		}
		if err = emit(o.out, p); err != nil {
			return err
		}
		return emit("", map[string]any{"status": "profile-created", "credentials_saved": false, "consent_changed": false})
	case "plan":
		p, err := loadPlan(o)
		if err != nil {
			return err
		}
		return emit(o.out, p)
	case "setup", "verify-setup", "run", "cleanup":
		if o.target == "" || o.binary == "" {
			return errors.New("live phases require --target-file and --binary")
		}
		if err := stateDirectory(o.state); err != nil {
			return err
		}
		p, err := harness.LoadProfile(o.target)
		if err != nil {
			return err
		}
		if o.python != "" {
			p.Python, err = filepath.Abs(o.python)
			if err != nil {
				return err
			}
		}
		var plan live.Plan
		if verb != "cleanup" {
			plan, err = loadPlan(o)
			if err != nil {
				return err
			}
		}
		if verb == "setup" {
			m, err := live.Setup(ctx, p, o.binary, o.state, plan)
			if err != nil {
				return err
			}
			return emit(o.out, map[string]any{"status": "ready", "setup_run": m.SetupRun, "target_fingerprint": m.TargetFingerprint, "selected_tests": len(m.Selection)})
		}
		if verb == "verify-setup" {
			m, err := live.VerifySetup(ctx, p, o.binary, o.state, plan)
			if err != nil {
				return err
			}
			return emit(o.out, map[string]any{"status": "verified", "target_fingerprint": m.TargetFingerprint, "selected_tests": len(plan.Tests)})
		}
		if verb == "cleanup" {
			if err = live.Cleanup(ctx, p, o.binary, o.state, o.run); err != nil {
				return err
			}
			return emit(o.out, map[string]string{"status": "cleanup-confirmed", "run_id": o.run})
		}
		r, runErr := live.Run(ctx, p, o.binary, o.state, plan)
		if len(r.Results) > 0 {
			if err = emit(o.out, r); err != nil {
				return err
			}
			if err = writeJUnit(filepath.Join(o.state, "runs", r.RunID, "junit.xml"), r); err != nil {
				return err
			}
		}
		return runErr
	default:
		return errors.New("unknown live runner phase")
	}
}

type junitCase struct {
	Name    string  `xml:"name,attr"`
	Time    string  `xml:"time,attr"`
	Failure *string `xml:"failure,omitempty"`
	Skipped *string `xml:"skipped,omitempty"`
}
type junitSuite struct {
	XMLName xml.Name    `xml:"testsuite"`
	Name    string      `xml:"name,attr"`
	Cases   []junitCase `xml:"testcase"`
}

func writeJUnit(path string, r live.Report) error {
	s := junitSuite{Name: "tadx-live"}
	for _, result := range r.Results {
		c := junitCase{Name: result.ID, Time: fmt.Sprintf("%.3f", result.Ended.Sub(result.Started).Seconds())}
		if result.Status != harness.Passed {
			message := string(result.Status) + ": " + result.Message
			c.Failure = &message
		}
		s.Cases = append(s.Cases, c)
	}
	data, err := xml.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, append([]byte(xml.Header), data...), 0o600)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	verb, o, err := parse(os.Args[1:])
	if err == nil {
		err = execute(ctx, verb, o)
	}
	if err != nil {
		_ = emit("", map[string]any{"status": harness.ErrorStatus(err), "error": err.Error()})
		os.Exit(1)
	}
}
