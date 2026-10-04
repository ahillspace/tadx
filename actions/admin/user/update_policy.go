package user

import (
	"context"
	"net/url"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

// ValidateUpdatePolicy rejects a changed full name only when the trusted server
// host or freshly observed caller role makes the operation unsupported.
func ValidateUpdatePolicy(ctx context.Context, serverURL string, callerSiteRole func(context.Context) (string, error), input UpdateRequest) error {
	if input.FullName == nil {
		return nil
	}
	server, err := url.Parse(serverURL)
	if err != nil {
		return err
	}
	host := strings.ToLower(server.Hostname())
	if host == "online.tableau.com" || strings.HasSuffix(host, ".online.tableau.com") {
		return unsupportedFullName("Tableau Cloud does not support updating a user's full name.")
	}
	role, err := callerSiteRole(ctx)
	if err != nil {
		return nil
	}
	if role == "SiteAdministratorCreator" || role == "SiteAdministratorExplorer" {
		return unsupportedFullName("A site administrator cannot update a user's full name.")
	}
	return nil
}

func unsupportedFullName(summary string) error {
	return &errs.Error{ID: "admin.user.update.unsupported_full_name", Kind: errs.KindUsage, Operation: "admin.user.update", Summary: summary, Retryable: errs.Bool(false), CorrectiveAction: "Remove --full-name and review a new preview.", Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted}
}
