package env_test

import (
	"context"
	"reflect"
	"testing"

	profileadd "github.com/ahillspace/tadx/actions/env/profile/add"
	profileget "github.com/ahillspace/tadx/actions/env/profile/get"
	profilelist "github.com/ahillspace/tadx/actions/env/profile/list"
	profileremove "github.com/ahillspace/tadx/actions/env/profile/remove"
	profilesetdefault "github.com/ahillspace/tadx/actions/env/profile/setdefault"
	profileupdate "github.com/ahillspace/tadx/actions/env/profile/update"
	envcli "github.com/ahillspace/tadx/internal/cli/env"
)

type actions struct {
	list       []profilelist.Input
	get        []profileget.Input
	add        []profileadd.Input
	update     []profileupdate.Input
	remove     []profileremove.Input
	setDefault []profilesetdefault.Input
}

func (a *actions) List(_ context.Context, input profilelist.Input) (profilelist.Output, error) {
	a.list = append(a.list, input)
	return profilelist.Output{}, nil
}
func (a *actions) Get(_ context.Context, input profileget.Input) (profileget.Output, error) {
	a.get = append(a.get, input)
	return profileget.Output{}, nil
}
func (a *actions) Add(_ context.Context, input profileadd.Input) (profileadd.Output, error) {
	a.add = append(a.add, input)
	return profileadd.Output{}, nil
}
func (a *actions) Update(_ context.Context, input profileupdate.Input) (profileupdate.Output, error) {
	a.update = append(a.update, input)
	return profileupdate.Output{}, nil
}
func (a *actions) Remove(_ context.Context, input profileremove.Input) (profileremove.Output, error) {
	a.remove = append(a.remove, input)
	return profileremove.Output{}, nil
}
func (a *actions) SetDefault(_ context.Context, input profilesetdefault.Input) (profilesetdefault.Output, error) {
	a.setDefault = append(a.setDefault, input)
	return profilesetdefault.Output{}, nil
}

type renderer struct{ calls int }

func (r *renderer) Render(any) error { r.calls++; return nil }

func TestEnvCommandsMapInputsAndRender(t *testing.T) {
	a := &actions{}
	r := &renderer{}
	command := envcli.New(envcli.Dependencies{
		Lister: a, Getter: a, Adder: a, Updater: a, Remover: a, DefaultSetter: a, Renderer: r,
	})

	commands := [][]string{
		{"list", "--limit", "5", "--cursor", "10"},
		{"get", "dev"},
		{"add", "dev", "--url", "https://tableau.example.com", "--site", "test-site", "--api-version", "3.29", "--pat-name-env", "DEV_PAT_NAME", "--pat-secret-env", "DEV_PAT_SECRET", "--default-workspace", "development"},
		{"update", "dev", "--url", "https://new.example.com", "--clear-site", "--api-version", "3.30", "--pat-name-env", "NEW_PAT_NAME", "--clear-pat-secret-env", "--default-workspace", "development"},
		{"remove", "old"},
		{"default", "dev"},
	}
	for _, args := range commands {
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Fatalf("Execute(%v) error = %v", args, err)
		}
	}

	if !reflect.DeepEqual(a.list, []profilelist.Input{{Limit: 5, Cursor: "10"}}) {
		t.Fatalf("list inputs = %#v", a.list)
	}
	if !reflect.DeepEqual(a.get, []profileget.Input{{Alias: "dev"}}) {
		t.Fatalf("get inputs = %#v", a.get)
	}
	if !reflect.DeepEqual(a.add, []profileadd.Input{{Alias: "dev", ServerURL: "https://tableau.example.com", SiteContentURL: "test-site", APIVersion: "3.29", PATNameEnv: "DEV_PAT_NAME", PATSecretEnv: "DEV_PAT_SECRET", DefaultWorkspace: "development"}}) {
		t.Fatalf("add inputs = %#v", a.add)
	}
	wantUpdate := profileupdate.Input{Alias: "dev", Patch: profileupdate.Patch{
		ServerURL:        profileupdate.StringField{Set: true, Value: "https://new.example.com"},
		SiteContentURL:   profileupdate.StringField{Set: true},
		APIVersion:       profileupdate.StringField{Set: true, Value: "3.30"},
		PATNameEnv:       profileupdate.StringField{Set: true, Value: "NEW_PAT_NAME"},
		PATSecretEnv:     profileupdate.StringField{Set: true},
		DefaultWorkspace: profileupdate.StringField{Set: true, Value: "development"},
	}}
	if !reflect.DeepEqual(a.update, []profileupdate.Input{wantUpdate}) {
		t.Fatalf("update inputs = %#v", a.update)
	}
	if !reflect.DeepEqual(a.remove, []profileremove.Input{{Alias: "old"}}) || !reflect.DeepEqual(a.setDefault, []profilesetdefault.Input{{Alias: "dev"}}) {
		t.Fatalf("remove = %#v, default = %#v", a.remove, a.setDefault)
	}
	if r.calls != len(commands) {
		t.Fatalf("render calls = %d, want %d", r.calls, len(commands))
	}
}

func TestEnvUpdateRejectsSetAndClearForSameField(t *testing.T) {
	command := envcli.New(envcli.Dependencies{Lister: &actions{}, Getter: &actions{}, Adder: &actions{}, Updater: &actions{}, Remover: &actions{}, DefaultSetter: &actions{}, Renderer: &renderer{}})
	command.SetArgs([]string{"update", "dev", "--site", "test-site", "--clear-site"})
	if err := command.Execute(); err == nil {
		t.Fatal("Execute() error = nil")
	}
}
