package group

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
)

type verificationWriter struct {
	*receiptAdapter
	observed Record
}

func (w *verificationWriter) UpdateGroup(context.Context, string, UpdateRequest) (Record, error) {
	w.writes++
	return w.observed, nil
}

func TestGroupMetadataVerificationStopsBeforeMembershipAndRetainsObservation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		input      UpdateInput
		observed   Record
		mismatched []string
		unverified []string
	}{
		{name: "rename", input: UpdateInput{Name: new("Requested")}, observed: Record{LUID: "group-1", Name: "Before"}, mismatched: []string{"name"}},
		{name: "role-mismatch", input: UpdateInput{MinimumSiteRole: new("Creator")}, observed: Record{LUID: "group-1", Name: "Before", MinimumSiteRole: "Viewer"}, mismatched: []string{"minimum_site_role"}},
		{name: "role-omitted", input: UpdateInput{MinimumSiteRole: new("Creator")}, observed: Record{LUID: "group-1", Name: "Before"}, unverified: []string{"minimum_site_role"}},
		{name: "external-mismatch", input: UpdateInput{ExternalUserEnabled: new(true)}, observed: Record{LUID: "group-1", Name: "Before", ExternalUserEnabled: new(false)}, mismatched: []string{"external_user_enabled"}},
		{name: "external-omitted", input: UpdateInput{ExternalUserEnabled: new(false)}, observed: Record{LUID: "group-1", Name: "Before"}, unverified: []string{"external_user_enabled"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writer := &verificationWriter{receiptAdapter: &receiptAdapter{}, observed: tc.observed}
			in := tc.input
			in.Environment, in.GroupLUID = "fixture", "group-1"
			in.MembershipSet, in.DesiredMemberLUIDs = true, []string{"new-user"}
			out, err := runGroupUpdate(t.Context(), writer, writer, writer, in, false)
			failure, ok := errors.AsType[*errs.Error](err)
			if !ok || failure.Phase != errs.PhaseVerification || failure.Outcome != errs.OutcomeUnknown || failure.Retryable == nil || *failure.Retryable || writer.writes != 1 {
				t.Fatalf("failure=%+v writes=%d", failure, writer.writes)
			}
			if out.Result == nil || out.Result.Group == nil || out.Result.Status != "partial" || out.Result.Group.Name != tc.observed.Name || !slices.Equal(out.Result.MismatchedSettings, tc.mismatched) || !slices.Equal(out.Result.UnverifiedSettings, tc.unverified) || out.Result.Added != 0 || out.Result.Removed != 0 {
				t.Fatalf("receipt=%+v", out.Result)
			}
			compact := out.CompactOutput().(UpdateCompactResult).Result
			if !slices.Equal(compact.MismatchedSettings, tc.mismatched) || !slices.Equal(compact.UnverifiedSettings, tc.unverified) {
				t.Fatalf("compact receipt=%+v", compact)
			}
		})
	}
}
