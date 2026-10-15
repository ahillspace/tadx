package workspace

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

type UnregisterInput struct {
	Name    string
	NameSet bool `json:"-"`
	Preview bool
}

type UnregisterOutput struct {
	Status         string    `json:"status"`
	Workspace      Workspace `json:"workspace"`
	FilesPreserved bool      `json:"files_preserved"`
	Help           []string  `json:"help"`
}

func (o UnregisterOutput) CompactOutput() any {
	return struct {
		Status         string    `json:"status"`
		Workspace      Workspace `json:"workspace"`
		FilesPreserved bool      `json:"files_preserved"`
		Details        string    `json:"details"`
		Help           []string  `json:"help"`
	}{o.Status, o.Workspace, o.FilesPreserved, "--full", o.Help}
}
func (o UnregisterOutput) FullOutput() any { return o }

type Registry interface {
	Unregister(context.Context, UnregisterInput) (Workspace, error)
}

func (a *Service) Unregister(ctx context.Context, input UnregisterInput) (UnregisterOutput, error) {
	if input.Name == "" && !input.NameSet {
		return UnregisterOutput{}, unregisterUsage("workspace name is required")
	}
	item, err := a.Registry.Unregister(ctx, input)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact registered workspace name, then retry.")
		return UnregisterOutput{}, &errs.Error{ID: "workspace.unregister.failed", Kind: errs.KindOperation, Operation: "workspace.unregister", Resource: input.Name, Summary: "Workspace unregister failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
	}
	invalid := item.Status == "invalid" && len(item.Violations) > 0
	if !invalid && (item.Name == "" || item.ID == "" || item.Root == "") {
		return UnregisterOutput{}, unregisterRuntimeError("workspace unregister returned an incomplete identity")
	}
	status := "unregistered"
	if input.Preview {
		status = "preview"
	}
	help := commandhint.Command("workspace", "register", "--path", item.Root)
	if invalid {
		name := item.Name
		if strings.TrimSpace(name) == "" || slices.Contains(item.Violations, "name") {
			name = "<name>"
		}
		args := []string{"workspace", "register", "--path", "<path>"}
		if strings.HasPrefix(name, "-") {
			args = append(args, "--")
		}
		help = commandhint.Command(append(args, name)...)
	}
	return UnregisterOutput{Status: status, Workspace: item, FilesPreserved: true, Help: []string{help}}, nil
}
func unregisterUsage(message string) error {
	return &errs.Error{ID: "workspace.unregister.usage", Kind: errs.KindUsage, Operation: "workspace.unregister", Summary: message, Cause: errors.New(message), Retryable: errs.Bool(false), CorrectiveAction: "Provide one registered workspace name."}
}
func unregisterRuntimeError(message string) error {
	return &errs.Error{ID: "workspace.unregister.runtime", Kind: errs.KindRuntime, Operation: "workspace.unregister", Summary: message, Retryable: errs.Bool(false)}
}
