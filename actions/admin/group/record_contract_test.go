package group

import (
	"context"
	"encoding/json"
	"testing"
)

type recordBackend struct {
	observations []Record
	writes       int
}

func (b *recordBackend) ResolveGroup(context.Context, Selector, bool) (Record, error) {
	r := b.observations[0]
	b.observations = b.observations[1:]
	return r, nil
}
func (b *recordBackend) UpdateGroup(context.Context, string, UpdateRequest) (Record, error) {
	b.writes++
	return Record{LUID: "g", Name: "after", RequestID: "write"}, nil
}
func (b *recordBackend) DeleteGroup(context.Context, string) (DeleteResult, error) {
	b.writes++
	return DeleteResult{Status: "deleted", GroupLUID: "g"}, nil
}
func (*recordBackend) AddGroupUser(context.Context, string, string) (string, error) {
	panic("unexpected membership addition")
}
func (*recordBackend) RemoveGroupUser(context.Context, string, string) (string, error) {
	panic("unexpected membership removal")
}

func TestMutationRevalidationKeepsOperationSnapshotScope(t *testing.T) {
	for _, operation := range []string{"update", "delete"} {
		for _, relevant := range []bool{false, true} {
			t.Run(operation+map[bool]string{false: "/unrelated", true: "/changed-identity"}[relevant], func(t *testing.T) {
				before := Record{LUID: "g", Name: "before", Domain: "local", Members: []Member{{LUID: "u", Name: "login", SiteRole: "Viewer"}}}
				current := before
				current.GrantLicenseMode = "changed-mode"
				current.RequestID = "new-read"
				current.MutationStatus = "irrelevant"
				current.Members = []Member{{LUID: "u", Name: "login", SiteRole: "Explorer"}}
				if operation == "delete" {
					current.MinimumSiteRole = "Creator"
					current.Members = nil
				}
				if relevant {
					current.Domain = "changed-domain"
				}
				backend := &recordBackend{observations: []Record{before, current}}
				var err error
				if operation == "update" {
					out, updateErr := runGroupUpdate(t.Context(), backend, backend, backend, UpdateInput{Environment: "test", GroupLUID: "g", Name: new("after"), MembershipSet: true, DesiredMemberLUIDs: []string{"u"}}, false)
					err = updateErr
					if err == nil && (len(out.Result.TableauRequestIDs) != 1 || out.Result.TableauRequestIDs[0] != "write" || out.Plan.Target.RequestID != "") {
						t.Fatalf("diagnostic projection changed: %#v", out)
					}
				} else {
					_, err = Delete(t.Context(), backend, backend, DeleteInput{Environment: "test", GroupLUID: "g"}, false)
				}
				if (err != nil) != relevant || backend.writes != map[bool]int{false: 1, true: 0}[relevant] || len(backend.observations) != 0 {
					t.Fatalf("error=%v backend=%#v", err, backend)
				}
			})
		}
	}
}

type richListReader struct{}

func (richListReader) ListGroups(_ context.Context, in ListPageRequest) (ListPage, error) {
	return ListPage{Number: in.PageNumber, Size: in.PageSize, Total: 1, Groups: []Record{{LUID: "g", Name: "Group", GrantLicenseMode: "onLogin", ExternalUserEnabled: new(false), Members: []Member{{LUID: "hidden"}}, ExternalUserEnabledState: "hidden", MembersOmitted: 2, RequestID: "hidden", MutationStatus: "hidden"}}}, nil
}
func TestListProjectsSharedRecordWithoutInspectFields(t *testing.T) {
	out, err := runGroupList(t.Context(), richListReader{}, ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(out.Groups[0])
	const want = `{"luid":"g","name":"Group","grant_license_mode":"onLogin","external_user_enabled":false}`
	if err != nil || string(got) != want {
		t.Fatalf("projection=%s, %v", got, err)
	}
}
