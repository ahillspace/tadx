package user

import (
	"context"
	"encoding/json"
	"testing"
)

type recordBackend struct {
	observations []Record
	writes       int
}

func (b *recordBackend) ResolveUser(context.Context, Selector) (Record, error) {
	r := b.observations[0]
	b.observations = b.observations[1:]
	return r, nil
}
func (b *recordBackend) UpdateUser(context.Context, string, UpdateRequest) (Record, error) {
	b.writes++
	return Record{LUID: "u", Name: "login", FullName: "after", RequestID: "write"}, nil
}
func (b *recordBackend) DeleteUser(context.Context, string) (DeleteResult, error) {
	b.writes++
	return DeleteResult{Status: "deleted", UserLUID: "u"}, nil
}

func TestMutationRevalidationKeepsOperationSnapshotScope(t *testing.T) {
	for _, operation := range []string{"update", "delete"} {
		for _, relevant := range []bool{false, true} {
			t.Run(operation+map[bool]string{false: "/unrelated", true: "/changed-identity"}[relevant], func(t *testing.T) {
				before := Record{LUID: "u", Name: "login", SiteRole: "Viewer", FullName: "before"}
				current := before
				current.LastLogin = "new-login"
				current.ExternalAuthUserID = "new-external"
				current.Domain = "new-domain"
				current.RequestID = "new-read"
				current.MutationStatus = "irrelevant"
				if operation == "delete" {
					current.FullName = "not-in-delete-snapshot"
				}
				if relevant {
					current.SiteRole = "Explorer"
				}
				backend := &recordBackend{observations: []Record{before, current}}
				var err error
				if operation == "update" {
					in := UpdateInput{Environment: "test", UserLUID: "u", FullName: new("after")}
					if err = ValidateUpdateInput(in); err != nil {
						t.Fatal(err)
					}
					out, updateErr := Update(t.Context(), backend, backend, in, false)
					err = updateErr
					if err == nil && (out.Result.TableauRequestID != "write" || out.Plan.Target.RequestID != "") {
						t.Fatalf("diagnostic projection changed: %#v", out)
					}
				} else {
					_, err = Delete(t.Context(), backend, backend, DeleteInput{Environment: "test", UserLUID: "u"}, false)
				}
				if (err != nil) != relevant || backend.writes != map[bool]int{false: 1, true: 0}[relevant] || len(backend.observations) != 0 {
					t.Fatalf("error=%v backend=%#v", err, backend)
				}
			})
		}
	}
}

type richListReader struct{}

func (richListReader) ListUsers(_ context.Context, in ListPageRequest) (ListPage, error) {
	return ListPage{Number: in.PageNumber, Size: in.PageSize, Total: 1, Users: []Record{{LUID: "u", Name: "login", SiteRole: "Viewer", LastLogin: "date", ExternalAuthUserID: "hidden", IdentityPoolName: "hidden", Language: "hidden", Locale: "hidden", RequestID: "hidden", MutationStatus: "hidden"}}}, nil
}
func TestListProjectsSharedRecordWithoutInspectFields(t *testing.T) {
	out, err := runUserList(t.Context(), richListReader{}, ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(out.Users[0])
	const want = `{"luid":"u","name":"login","site_role":"Viewer","last_login":"date"}`
	if err != nil || string(got) != want {
		t.Fatalf("projection=%s, %v", got, err)
	}
}
