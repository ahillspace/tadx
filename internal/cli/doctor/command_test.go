package doctor_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	doctorrun "github.com/ahillspace/tadx/actions/doctor/run"
	doctorcli "github.com/ahillspace/tadx/internal/cli/doctor"
	"github.com/ahillspace/tadx/internal/errs"
)

type runner struct {
	inputs []doctorrun.Input
	output doctorrun.Output
	err    error
}

func (r *runner) Execute(_ context.Context, input doctorrun.Input) (doctorrun.Output, error) {
	r.inputs = append(r.inputs, input)
	return r.output, r.err
}

type renderer struct {
	values []any
	err    error
}

func (r *renderer) Render(value any) error {
	r.values = append(r.values, value)
	return r.err
}

func TestDoctorMapsOptionalScopesAndRendersOneResult(t *testing.T) {
	want := doctorrun.Output{Status: doctorrun.StatusWarn}
	run := &runner{output: want}
	render := &renderer{}
	command := doctorcli.New(doctorcli.Dependencies{Runner: run, Renderer: render, Use: "doctor", Short: "Diagnose TADX readiness."})
	command.SetArgs([]string{"--environment", "dev", "--workspace", "development"})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if command.Use != "doctor" || command.Short != "Diagnose TADX readiness." || command.Annotations["tadx.capability"] != "doctor.run" {
		t.Fatalf("command = use:%q short:%q annotations:%#v", command.Use, command.Short, command.Annotations)
	}
	if len(run.inputs) != 1 || run.inputs[0].Environment != "dev" || run.inputs[0].Workspace != "development" {
		t.Fatalf("inputs = %#v", run.inputs)
	}
	if len(render.values) != 1 || !reflect.DeepEqual(render.values[0], want) {
		t.Fatalf("rendered = %#v", render.values)
	}
}

func TestDoctorAcceptsOmittedScopes(t *testing.T) {
	run := &runner{}
	command := doctorcli.New(doctorcli.Dependencies{Runner: run, Renderer: &renderer{}})
	command.SetArgs(nil)
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(run.inputs) != 1 || run.inputs[0] != (doctorrun.Input{}) {
		t.Fatalf("inputs = %#v", run.inputs)
	}
}

func TestDoctorRejectsArgumentsBeforeRunning(t *testing.T) {
	run := &runner{}
	command := doctorcli.New(doctorcli.Dependencies{Runner: run, Renderer: &renderer{}})
	command.SetArgs([]string{"unexpected"})
	err := command.ExecuteContext(context.Background())
	if err == nil || errs.ExitCode(err) != 2 || len(run.inputs) != 0 {
		t.Fatalf("error=%v inputs=%#v", err, run.inputs)
	}
}

func TestDoctorPropagatesRunnerAndRendererFailures(t *testing.T) {
	runnerFailure := errors.New("runner failed")
	run := &runner{err: runnerFailure}
	render := &renderer{}
	command := doctorcli.New(doctorcli.Dependencies{Runner: run, Renderer: render})
	command.SetArgs(nil)
	if err := command.ExecuteContext(context.Background()); !errors.Is(err, runnerFailure) || len(render.values) != 0 {
		t.Fatalf("runner error=%v rendered=%#v", err, render.values)
	}

	renderFailure := errors.New("render failed")
	command = doctorcli.New(doctorcli.Dependencies{Runner: &runner{}, Renderer: &renderer{err: renderFailure}})
	command.SetArgs(nil)
	if err := command.ExecuteContext(context.Background()); !errors.Is(err, renderFailure) {
		t.Fatalf("renderer error=%v", err)
	}
}

func TestDoctorReportsMissingWiringWithoutPanic(t *testing.T) {
	command := doctorcli.New(doctorcli.Dependencies{})
	command.SetArgs(nil)
	err := command.ExecuteContext(context.Background())
	if err == nil || errs.ExitCode(err) != 1 {
		t.Fatalf("error = %v", err)
	}
}
