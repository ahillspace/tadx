package env_test

import (
	"context"
	"github.com/ahillspace/tadx/actions/env/profile"
	envcli "github.com/ahillspace/tadx/internal/cli/env"
	"reflect"
	"strings"
	"testing"
)

type actions struct {
	list       []profile.ListInput
	get        []profile.GetInput
	add        []profile.AddInput
	update     []profile.UpdateInput
	remove     []profile.RemoveInput
	setDefault []profile.SetDefaultInput
}

func (a *actions) List(_ context.Context, input profile.ListInput) (profile.ListOutput, error) {
	a.list = append(a.list, input)
	return profile.ListOutput{}, nil
}
func (a *actions) Get(_ context.Context, input profile.GetInput) (profile.GetOutput, error) {
	a.get = append(a.get, input)
	return profile.GetOutput{}, nil
}
func (a *actions) Add(_ context.Context, input profile.AddInput) (profile.AddOutput, error) {
	a.add = append(a.add, input)
	return profile.AddOutput{}, nil
}
func (a *actions) Update(_ context.Context, input profile.UpdateInput) (profile.UpdateOutput, error) {
	a.update = append(a.update, input)
	return profile.UpdateOutput{}, nil
}
func (a *actions) Remove(_ context.Context, input profile.RemoveInput) (profile.RemoveOutput, error) {
	a.remove = append(a.remove, input)
	return profile.RemoveOutput{}, nil
}
func (a *actions) SetDefault(_ context.Context, input profile.SetDefaultInput) (profile.SetDefaultOutput, error) {
	a.setDefault = append(a.setDefault, input)
	return profile.SetDefaultOutput{}, nil
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

	if !reflect.DeepEqual(a.list, []profile.ListInput{{Limit: 5, Cursor: "10"}}) {
		t.Fatalf("list inputs = %#v", a.list)
	}
	if !reflect.DeepEqual(a.get, []profile.GetInput{{Alias: "dev"}}) {
		t.Fatalf("get inputs = %#v", a.get)
	}
	if !reflect.DeepEqual(a.add, []profile.AddInput{{Alias: "dev", ServerURL: "https://tableau.example.com", SiteContentURL: "test-site", APIVersion: "3.29", PATNameEnv: "DEV_PAT_NAME", PATSecretEnv: "DEV_PAT_SECRET", DefaultWorkspace: "development"}}) {
		t.Fatalf("add inputs = %#v", a.add)
	}
	wantUpdate := profile.UpdateInput{Alias: "dev", Patch: profile.Patch{
		ServerURL:        profile.StringField{Set: true, Value: "https://new.example.com"},
		SiteContentURL:   profile.StringField{Set: true},
		APIVersion:       profile.StringField{Set: true, Value: "3.30"},
		PATNameEnv:       profile.StringField{Set: true, Value: "NEW_PAT_NAME"},
		PATSecretEnv:     profile.StringField{Set: true},
		DefaultWorkspace: profile.StringField{Set: true, Value: "development"},
	}}
	if !reflect.DeepEqual(a.update, []profile.UpdateInput{wantUpdate}) {
		t.Fatalf("update inputs = %#v", a.update)
	}
	if !reflect.DeepEqual(a.remove, []profile.RemoveInput{{Alias: "old"}}) || !reflect.DeepEqual(a.setDefault, []profile.SetDefaultInput{{Alias: "dev"}}) {
		t.Fatalf("remove = %#v, default = %#v", a.remove, a.setDefault)
	}
	if r.calls != len(commands) {
		t.Fatalf("render calls = %d, want %d", r.calls, len(commands))
	}
}

func TestEnvListAllCarriesCompleteInventoryMode(t *testing.T) {
	a := &actions{}
	command := envcli.New(envcli.Dependencies{Lister: a, Renderer: &renderer{}})
	command.SetArgs([]string{"list", "--all"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.list, []profile.ListInput{{All: true}}) {
		t.Fatalf("list inputs = %#v", a.list)
	}
}

func TestEnvListAllRejectsLimitAndCursor(t *testing.T) {
	for _, args := range [][]string{{"list", "--all", "--limit", "1"}, {"list", "--all", "--cursor", "0"}} {
		a := &actions{}
		command := envcli.New(envcli.Dependencies{Lister: a, Renderer: &renderer{}})
		command.SetArgs(args)
		if err := command.Execute(); err == nil || len(a.list) != 0 {
			t.Fatalf("args=%v error=%v calls=%d", args, err, len(a.list))
		}
	}
}

func TestEnvironmentRegistryUsePreservesAliasArgument(t *testing.T) {
	command := envcli.New(envcli.Dependencies{Uses: map[string]string{"env.profile.add": "add"}})
	found, _, err := command.Find([]string{"add"})
	if err != nil || found.Use != "add <alias>" {
		t.Fatalf("Find(add) use = %q, error = %v, want add <alias>", found.Use, err)
	}
}

func TestEnvUpdateRejectsSetAndClearForSameField(t *testing.T) {
	command := envcli.New(envcli.Dependencies{Lister: &actions{}, Getter: &actions{}, Adder: &actions{}, Updater: &actions{}, Remover: &actions{}, DefaultSetter: &actions{}, Renderer: &renderer{}})
	command.SetArgs([]string{"update", "dev", "--site", "test-site", "--clear-site"})
	if err := command.Execute(); err == nil {
		t.Fatal("Execute() error = nil")
	}
}

func TestEnvCacheConcurrencyFlags(t *testing.T) {
	for _, args := range [][]string{
		{"add", "dev", "--url", "https://tableau.example.com", "--cache-max-concurrency", "8"},
		{"update", "dev", "--cache-max-concurrency", "8"},
		{"update", "dev", "--clear-cache-max-concurrency"},
	} {
		a := &actions{}
		command := envcli.New(envcli.Dependencies{Adder: a, Updater: a, Renderer: &renderer{}})
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Fatalf("Execute(%v): %v", args, err)
		}
		if args[0] == "add" {
			if len(a.add) != 1 || a.add[0].CacheMaxConcurrency != 8 {
				t.Fatalf("add=%+v", a.add)
			}
		} else {
			want := 8
			if args[2] == "--clear-cache-max-concurrency" {
				want = 0
			}
			if len(a.update) != 1 || !a.update[0].Patch.CacheMaxConcurrency.Set || a.update[0].Patch.CacheMaxConcurrency.Value != want {
				t.Fatalf("update=%+v", a.update)
			}
		}
	}
}

func TestEnvCacheConcurrencyBoundsAndHelp(t *testing.T) {
	for _, verb := range []string{"add", "update"} {
		for _, value := range []string{"0", "-1", "257"} {
			a := &actions{}
			command := envcli.New(envcli.Dependencies{Adder: a, Updater: a, Renderer: &renderer{}})
			args := []string{verb, "dev", "--cache-max-concurrency", value}
			if verb == "add" {
				args = append(args, "--url", "https://tableau.example.com")
			}
			command.SetArgs(args)
			if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "between 1 and 256") {
				t.Fatalf("args=%v error=%v", args, err)
			}
			if len(a.add)+len(a.update) != 0 {
				t.Fatal("invalid value reached action")
			}
		}
		command := envcli.New(envcli.Dependencies{})
		found, _, err := command.Find([]string{verb})
		if err != nil {
			t.Fatal(err)
		}
		if flag := found.Flags().Lookup("cache-max-concurrency"); flag == nil || !strings.Contains(flag.Usage, "default 32") {
			t.Fatalf("flag=%+v", flag)
		}
	}
	command := envcli.New(envcli.Dependencies{})
	command.SetArgs([]string{"update", "dev", "--cache-max-concurrency", "8", "--clear-cache-max-concurrency"})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("error=%v", err)
	}
}
