package datasource

import (
	"context"
	"errors"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

type DatasourceTags interface {
	ReadDatasourceTags(context.Context, string) ([]string, error)
	AddDatasourceTags(context.Context, string, []string) (value.TDSReceipt, error)
	DeleteDatasourceTag(context.Context, string, string) (value.TDSReceipt, error)
}

type TagInput struct {
	Environment, DatasourceLUID, Tag string
	Add                              bool
}

type TagOutput struct {
	Environment      string   `json:"environment"`
	DatasourceLUID   string   `json:"datasource_luid"`
	Tag              string   `json:"tag"`
	PresentBefore    bool     `json:"present_before"`
	Desired          bool     `json:"desired_present"`
	Status           string   `json:"status"`
	Observed         *bool    `json:"observed_present,omitempty"`
	TableauRequestID string   `json:"tableau_request_id,omitempty"`
	Help             []string `json:"help,omitempty"`
}

func (in TagInput) Validate() error {
	id := in.DatasourceLUID
	if identity.ValidateLUIDShape("datasource", id) != nil || strings.TrimSpace(id) != id || id == "." || id == ".." {
		return errs.New(errs.KindUsage, "Provide an exact published datasource LUID with --id.")
	}
	if strings.TrimSpace(in.Tag) == "" || !utf8.ValidString(in.Tag) || strings.ContainsFunc(in.Tag, unicode.IsControl) {
		return errs.New(errs.KindUsage, "Provide one nonblank UTF-8 tag without control characters.")
	}
	if !in.Add && (in.Tag == "." || in.Tag == "..") {
		return errs.New(errs.KindUsage, "This tag cannot be safely addressed as a URL path segment.")
	}
	return nil
}

func ChangeDatasourceTag(ctx context.Context, client DatasourceTags, in TagInput, preview bool) (TagOutput, error) {
	if err := in.Validate(); err != nil {
		return TagOutput{}, err
	}
	op := "datasource.remove-tag"
	if in.Add {
		op = "datasource.add-tag"
	}
	read := func() (bool, error) {
		labels, err := client.ReadDatasourceTags(ctx, in.DatasourceLUID)
		return slices.Contains(labels, in.Tag), err
	}
	before, err := read()
	if err != nil {
		return TagOutput{}, err
	}
	out := TagOutput{Environment: in.Environment, DatasourceLUID: in.DatasourceLUID, Tag: in.Tag, PresentBefore: before, Desired: in.Add, Status: "preview"}
	if before == in.Add {
		out.Status = "already_current"
		return out, nil
	}
	if preview {
		return out, nil
	}
	fresh, err := read()
	if err != nil {
		return out, err
	}
	if fresh != before {
		out.Status = "stale"
		return out, &errs.Error{Kind: errs.KindOperation, Operation: op, Summary: "The selected tag membership changed during preflight.", Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted, Retryable: new(false)}
	}
	var receipt value.TDSReceipt
	if in.Add {
		receipt, err = client.AddDatasourceTags(ctx, in.DatasourceLUID, []string{in.Tag})
	} else {
		receipt, err = client.DeleteDatasourceTag(ctx, in.DatasourceLUID, in.Tag)
	}
	out.TableauRequestID = receipt.TableauRequestID
	hint := commandhint.Environment(in.Environment, "content", "datasource", strings.TrimPrefix(op, "datasource."), "--id", in.DatasourceLUID, "--tag", in.Tag, "--preview")
	fail := func(status, mutationStatus, summary string, cause error) (TagOutput, error) {
		out.Status = status
		out.Help = []string{hint}
		return out, errs.MutationFailure(errs.Error{Kind: errs.KindOperation, Operation: op, Environment: in.Environment, Resource: in.DatasourceLUID, Summary: summary, Cause: cause, TableauRequestID: receipt.TableauRequestID, CorrectiveAction: hint}, mutationStatus)
	}
	if receipt.Status == "rejected" {
		return fail("rejected", "not_applied", "Tableau rejected the tag change.", err)
	}
	if receipt.Status == "not_attempted" {
		return fail("not_attempted", "not_attempted", "The tag change was stopped before submission.", err)
	}
	observed, readErr := read()
	if readErr == nil {
		out.Observed = &observed
	}
	if err != nil || receipt.Status != "acknowledged" {
		return fail("unknown", "unknown", "Tag submission outcome is unknown.", errors.Join(err, readErr))
	}
	if readErr != nil || observed != in.Add {
		return fail("acknowledged_unverified", "partial", "Tableau acknowledged the tag change. Membership verification failed.", readErr)
	}
	out.Status = "verified"
	return out, nil
}
