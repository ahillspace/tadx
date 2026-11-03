package group

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
)

type verificationWriter struct {
	*receiptAdapter
	initial        Record
	observed       Record
	submitted      UpdateRequest
	metadataWrites int
}

func (w *verificationWriter) ResolveGroup(ctx context.Context, selector Selector, members bool) (Record, error) {
	if w.initial.LUID == "" {
		return w.receiptAdapter.ResolveGroup(ctx, selector, members)
	}
	w.reads++
	return w.initial, nil
}

func (w *verificationWriter) UpdateGroup(_ context.Context, _ string, request UpdateRequest) (Record, error) {
	w.writes++
	w.metadataWrites++
	w.submitted = request
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

func TestGroupMetadataVerificationUsesSubmittedRequestWithoutExpandingNoOps(t *testing.T) {
	for _, tc := range []struct {
		name                            string
		request                         UpdateRequest
		observed                        Record
		membership                      bool
		changes, metadataWrites, writes int
		status                          string
	}{
		{name: "all-explicit-metadata-unchanged", request: UpdateRequest{Name: new("Before"), MinimumSiteRole: new("Viewer"), ExternalUserEnabled: new(false)}, status: "unchanged"},
		{name: "membership-only-with-unchanged-explicit-metadata", request: UpdateRequest{Name: new("Before"), MinimumSiteRole: new("Viewer"), ExternalUserEnabled: new(false)}, membership: true, writes: 1, status: "updated"},
		{name: "changed-role-with-unchanged-name-and-bool", request: UpdateRequest{Name: new("Before"), MinimumSiteRole: new("Creator"), ExternalUserEnabled: new(false)}, observed: Record{LUID: "group-1", Name: "Before", MinimumSiteRole: "Creator", ExternalUserEnabled: new(false)}, membership: true, changes: 1, metadataWrites: 1, writes: 2, status: "updated"},
		{name: "unrequested-response-fields-remain-observations", request: UpdateRequest{MinimumSiteRole: new("Creator")}, observed: Record{LUID: "group-1", Name: "Observed", MinimumSiteRole: "Creator", ExternalUserEnabled: new(true)}, membership: true, changes: 1, metadataWrites: 1, writes: 2, status: "updated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writer := &verificationWriter{receiptAdapter: &receiptAdapter{}, initial: Record{LUID: "group-1", Name: "Before", MinimumSiteRole: "Viewer", ExternalUserEnabled: new(false)}, observed: tc.observed}
			input := UpdateInput{Environment: "fixture", GroupLUID: "group-1", Name: tc.request.Name, MinimumSiteRole: tc.request.MinimumSiteRole, ExternalUserEnabled: tc.request.ExternalUserEnabled, MembershipSet: tc.membership}
			if tc.membership {
				input.DesiredMemberLUIDs = []string{"new-user"}
			}
			out, err := runGroupUpdate(t.Context(), writer, writer, writer, input, false)
			if err != nil || out.Result == nil || out.Result.Status != tc.status || len(out.Plan.Changes) != tc.changes || writer.metadataWrites != tc.metadataWrites || writer.writes != tc.writes || out.Plan.NoOp != (tc.writes == 0) {
				t.Fatalf("output=%+v result=%+v writes=%d metadata=%d error=%v", out, out.Result, writer.writes, writer.metadataWrites, err)
			}
			if tc.metadataWrites == 0 {
				if writer.submitted.Name != nil || writer.submitted.MinimumSiteRole != nil || writer.submitted.ExternalUserEnabled != nil {
					t.Fatalf("no-op metadata was submitted: %+v", writer.submitted)
				}
			} else if out.Result.Group == nil || out.Result.Group.Name != tc.observed.Name || len(out.Result.MismatchedSettings)+len(out.Result.UnverifiedSettings) != 0 || !reflect.DeepEqual(writer.submitted, tc.request) {
				t.Fatalf("submitted request or observed receipt changed: request=%+v receipt=%+v", writer.submitted, out.Result)
			}
		})
	}
}
