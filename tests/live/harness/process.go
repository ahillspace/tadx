package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/term"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const maxCommandOutput = 4 << 20

type boundedBuffer struct {
	buffer   bytes.Buffer
	exceeded bool
}

func (b *boundedBuffer) Len() int       { return b.buffer.Len() }
func (b *boundedBuffer) String() string { return b.buffer.String() }
func (b *boundedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if n > maxCommandOutput-b.Len() {
		b.exceeded = true
		p = p[:max(0, maxCommandOutput-b.Len())]
	}
	_, err := b.buffer.Write(p)
	return n, err
}

type Command struct {
	Args     []string       `json:"args"`
	ExitCode int            `json:"exit_code"`
	Stdout   string         `json:"stdout,omitempty"`
	Stderr   string         `json:"stderr,omitempty"`
	Value    map[string]any `json:"value,omitempty"`
	Started  time.Time      `json:"started_at"`
	Ended    time.Time      `json:"ended_at"`
}

type CommandError struct {
	ExitCode int
	ID       string
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("candidate exited %d (%s)", e.ExitCode, e.ID)
}

func (c *Case) Command(ctx context.Context, args ...string) (Command, error) {
	return c.command(ctx, false, args...)
}
func (c *Case) Interactive(ctx context.Context, args ...string) (Command, error) {
	b, ok := c.Profile.Bindings["native_login"]
	if !ok || b.Params["operator_assisted"] != "true" || !term.IsTerminal(int(os.Stdin.Fd())) {
		return Command{}, Blocked("native login requires a real interactive terminal and explicit native_login.operator_assisted binding")
	}
	return c.command(ctx, true, args...)
}
func (c *Case) command(ctx context.Context, interactive bool, args ...string) (Command, error) {
	limit := c.Profile.Budgets.CommandSeconds
	if limit <= 0 {
		limit = 120
	}
	if slices.Contains(args, "publish") || slices.Contains(args, "pull") || slices.Contains(args, "wait") {
		limit = c.Profile.Budgets.OperationSeconds + 120
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(limit)*time.Second)
	defer cancel()
	actual := slices.Clone(args)
	nativeTransfer := mayStartNativeTransfer(args)
	if nativeTransfer && c.Journal != nil {
		c.Journal.TransferPending = true
		if err := c.Journal.save(); err != nil {
			return Command{}, err
		}
	}
	if !slices.Contains(actual, "--config") {
		actual = append(actual, "--config", c.ConfigPath)
	}
	cmd := exec.CommandContext(ctx, c.Binary, actual...)
	cmd.Dir = c.RunDir
	cmd.Env = append(os.Environ(), "TADX_FEEDBACK_MODE=test")
	cmd.Env = append(cmd.Env, c.ExtraEnv...)
	var stdout, stderr boundedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if interactive {
		cmd.Stdin = os.Stdin
		cmd.Stdout = io.MultiWriter(os.Stdout, &stdout)
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	}
	r := Command{Args: slices.Clone(actual), ExitCode: -1, Started: time.Now().UTC()}
	err := cmd.Run()
	r.Ended = time.Now().UTC()
	if cmd.ProcessState != nil {
		r.ExitCode = cmd.ProcessState.ExitCode()
	}
	if c.API != nil && c.Profile.FixturePAT.NameEnv == "" && c.Profile.FixturePAT.CredentialRef == "" {
		c.API.MarkSessionStale()
	}
	r.Stdout = stdout.String()
	r.Stderr = stderr.String()
	if c.API != nil {
		r.Stdout = c.API.Redact(r.Stdout)
		r.Stderr = c.API.Redact(r.Stderr)
	}
	for _, secret := range c.secrets {
		if secret != "" {
			r.Stdout = strings.ReplaceAll(r.Stdout, secret, "[REDACTED]")
			r.Stderr = strings.ReplaceAll(r.Stderr, secret, "[REDACTED]")
		}
	}
	leaked := r.Stdout != stdout.String() || r.Stderr != stderr.String()
	// Evidence retains redacted bytes, never the provider's raw credential values.
	dec := json.NewDecoder(strings.NewReader(r.Stdout))
	dec.UseNumber()
	parseErr := dec.Decode(&r.Value)
	if parseErr == nil {
		trailingErr := dec.Decode(new(any))
		if !errors.Is(trailingErr, io.EOF) {
			parseErr = errors.New("candidate output has trailing content")
		}
	}
	capturedAcceptance := false
	if c.Journal != nil {
		var capture func(any) error
		capture = func(v any) error {
			switch x := v.(type) {
			case map[string]any:
				for k, item := range x {
					if id, ok := item.(string); ok && id != "" {
						var recordErr error
						switch k {
						case "operation_id":
							if nativeTransfer {
								capturedAcceptance = true
								recordErr = c.AcceptOperation(id)
							}
						case "tableau_job_id", "job_id":
							capturedAcceptance = true
							recordErr = c.AcceptJob(id)
						}
						if recordErr != nil {
							return recordErr
						}
					}
					if err := capture(item); err != nil {
						return err
					}
				}
			case []any:
				for _, item := range x {
					if err := capture(item); err != nil {
						return err
					}
				}
			}
			return nil
		}
		if captureErr := capture(r.Value); captureErr != nil {
			return r, fmt.Errorf("save confirmed acceptance identity: %w", captureErr)
		}
		if nativeTransfer && !stdout.exceeded && !stderr.exceeded && ((err == nil && parseErr == nil) || capturedAcceptance) {
			c.Journal.TransferPending = false
			if saveErr := c.Journal.save(); saveErr != nil {
				return r, saveErr
			}
		}
	}
	c.mu.Lock()
	c.Commands = append(c.Commands, r)
	evidence := slices.Clone(c.Commands)
	c.mu.Unlock()
	if c.Journal != nil {
		if evidenceErr := WriteJSON(filepath.Join(filepath.Dir(c.Journal.path), "commands.json"), evidence); evidenceErr != nil {
			return r, errors.New("save bounded command evidence failed")
		}
	}
	if leaked {
		return r, errors.New("candidate emitted credential or session material; evidence was redacted")
	}
	if stdout.exceeded || stderr.exceeded {
		return r, errors.New("candidate output exceeded bounded evidence limit")
	}
	if ctx.Err() != nil {
		return r, Inconclusive("candidate deadline elapsed; observe accepted identities before retrying")
	}
	if err != nil {
		id := "process-failure"
		if er, ok := r.Value["error"].(map[string]any); ok {
			if s, ok := er["id"].(string); ok {
				id = s
			}
		}
		return r, &CommandError{r.ExitCode, id}
	}
	if slices.Contains(args, "--json") && (parseErr != nil || r.Value == nil) {
		return r, errors.New("candidate did not return exactly one JSON object")
	}
	return r, nil
}

func mayStartNativeTransfer(args []string) bool {
	return len(args) >= 3 && args[0] == "content" && (args[2] == "publish" || args[2] == "pull") && !slices.Contains(args, "--preview")
}

func (c *Case) Run(ctx context.Context, args ...string) (map[string]any, error) {
	if !slices.Contains(args, "--json") {
		args = append(slices.Clone(args), "--json")
	}
	r, err := c.Command(ctx, args...)
	if err != nil {
		return r.Value, err
	}
	if r.Value == nil {
		return nil, errors.New("candidate returned no parseable JSON object")
	}
	return r.Value, nil
}
