package group

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
)

type receiptAdapter struct{ reads, writes int }

func runGroupUpdate(ctx context.Context, resolver Resolver, writer UpdateWriter, members MembershipWriter, input UpdateInput, preview bool) (UpdateOutput, error) {
	if err := ValidateUpdateInput(&input); err != nil {
		return UpdateOutput{}, err
	}
	return Update(ctx, resolver, writer, members, input, preview)
}

func (a *receiptAdapter) ResolveGroup(context.Context, Selector, bool) (Record, error) {
	a.reads++
	return Record{LUID: "group-1", Name: "Before", Members: []Member{{LUID: "old-user"}}}, nil
}
func (a *receiptAdapter) UpdateGroup(context.Context, string, UpdateRequest) (Record, error) {
	a.writes++
	return Record{LUID: "group-1", Name: "Confirmed", RequestID: "metadata-request", Members: []Member{{LUID: "should-not-leak"}}}, nil
}
func (a *receiptAdapter) AddGroupUser(context.Context, string, string) (string, error) {
	a.writes++
	return "add-request", nil
}
func (a *receiptAdapter) RemoveGroupUser(context.Context, string, string) (string, error) {
	a.writes++
	return "", errors.New("remove failed")
}

func TestPartialReceiptKeepsOnlyCompletedResponseEvidence(t *testing.T) {
	a := &receiptAdapter{}
	name := "Requested"
	out, err := runGroupUpdate(context.Background(), a, a, a, UpdateInput{Environment: "test", Site: "site-1", GroupLUID: "group-1", Name: &name, MembershipSet: true, DesiredMemberLUIDs: []string{"new-user"}}, false)
	var detail *errs.Error
	if !errors.As(err, &detail) || detail.Failed != "member.remove:old-user" || len(detail.Completed) != 2 {
		t.Fatalf("partial error: %v", err)
	}
	if a.reads != 2 || a.writes != 3 {
		t.Fatalf("reads=%d writes=%d", a.reads, a.writes)
	}
	r := out.CompactOutput().(UpdateCompactResult).Result
	if r == nil || r.Status != "partial" || r.Group == nil || r.Group.Name != "Confirmed" || r.Added != 1 || r.Removed != 0 || len(r.AddedUserLUIDs) != 1 || r.AddedUserLUIDs[0] != "new-user" || len(r.RemovedUserLUIDs) != 0 {
		t.Fatalf("receipt: %#v", r)
	}
}

func TestCompactReceiptBoundsChangedMembershipIDsWithoutLosingFullEvidence(t *testing.T) {
	ids := make([]string, 14)
	for i := range ids {
		ids[i] = fmt.Sprintf("user-%02d", i)
	}
	out := UpdateOutput{Result: &UpdateResult{Status: "updated", Added: len(ids), Removed: len(ids), AddedUserLUIDs: ids, RemovedUserLUIDs: ids}}
	compact := out.CompactOutput().(UpdateCompactResult).Result
	if len(compact.AddedUserLUIDs) != 10 || compact.AddedUserLUIDsOmitted != 4 || len(compact.RemovedUserLUIDs) != 10 || compact.RemovedUserLUIDsOmitted != 4 {
		t.Fatalf("compact bounds: %#v", compact)
	}
	full := out.FullOutput().(UpdateOutput).Result
	if len(full.AddedUserLUIDs) != 14 || len(full.RemovedUserLUIDs) != 14 {
		t.Fatalf("full receipt: %#v", full)
	}
}
