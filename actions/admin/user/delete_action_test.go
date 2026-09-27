package user

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
)

type deleteResolver struct {
	users []Record
}

func (r *deleteResolver) ResolveUser(context.Context, Selector) (Record, error) {
	if len(r.users) == 0 {
		return Record{}, errors.New("user not found")
	}
	user := r.users[0]
	r.users = r.users[1:]
	return user, nil
}

type deleteResult struct {
	result DeleteResult
	err    error
}

func (d deleteResult) DeleteUser(context.Context, string) (DeleteResult, error) {
	return d.result, d.err
}

type assetConflictError struct{}

func (assetConflictError) Error() string {
	return "Tableau request failed with HTTP 409 code 409003: User asset conflict"
}
func (assetConflictError) HTTPStatus() int     { return 409 }
func (assetConflictError) TableauCode() string { return "409003" }

func TestAssetConflictRetainsUnlicensedReadbackAndRefusedRemoval(t *testing.T) {
	resolver := &deleteResolver{users: []Record{
		{LUID: "user-1", Name: "alex@example.com", SiteRole: "Viewer"},
		{LUID: "user-1", Name: "alex@example.com", SiteRole: "Viewer"},
		{LUID: "user-1", Name: "alex@example.com", SiteRole: "Unlicensed"},
	}}

	out, err := Delete(context.Background(), resolver, deleteResult{result: DeleteResult{Status: "unknown", UserLUID: "user-1", TableauRequestID: "delete-request"}, err: assetConflictError{}}, DeleteInput{Environment: "prod", Site: "site", UserLUID: "user-1"}, false)
	if err == nil || out.Result == nil {
		t.Fatalf("output=%#v error=%v", out, err)
	}
	if out.Result.Status != "unlicensed" || out.Result.RemovalStatus != "refused" || out.Result.LicenseStatus != "unlicensed" || out.Result.UserLUID != "user-1" {
		t.Fatalf("result=%#v", out.Result)
	}
	if len(out.Help) != 1 || out.Help[0] != "tadx admin user inspect --id user-1 --full --environment prod" {
		t.Fatalf("help=%v", out.Help)
	}
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "admin.user.delete.partial" || structured.Phase != errs.PhaseVerification || structured.Outcome != errs.OutcomeConfirmed || structured.Resource != "user-1" {
		t.Fatalf("error=%#v", structured)
	}
}

func TestAssetConflictDoesNotInferUnlicensedWithoutReadback(t *testing.T) {
	resolver := &deleteResolver{users: []Record{
		{LUID: "user-1", Name: "alex@example.com", SiteRole: "Viewer"},
		{LUID: "user-1", Name: "alex@example.com", SiteRole: "Viewer"},
	}}

	out, err := Delete(context.Background(), resolver, deleteResult{result: DeleteResult{Status: "unknown", UserLUID: "user-1"}, err: assetConflictError{}}, DeleteInput{Environment: "prod", Site: "site", UserLUID: "user-1"}, false)
	if err == nil || out.Result == nil || out.Result.RemovalStatus != "refused" || out.Result.LicenseStatus != "unknown" {
		t.Fatalf("output=%#v error=%v", out, err)
	}
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Outcome != errs.OutcomeUnknown || structured.Phase != errs.PhaseVerification {
		t.Fatalf("error=%#v", structured)
	}
}
