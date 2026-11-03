package content

import (
	"context"

	"github.com/ahillspace/tadx/actions/batchinput"
	"github.com/ahillspace/tadx/actions/contentlabel"
)

func prepareLabelUpdate(ctx context.Context, deps LabelDependencies, in *contentlabel.UpdateInput) error {
	if !batchinput.Active(ctx) {
		return nil
	}
	if err := batchinput.Authorize(ctx, deps.BatchTargets, in.Environment); err != nil {
		return err
	}
	out, err := deps.Updater.Execute(ctx, *in, true)
	if err != nil {
		return err
	}
	if err := in.PinTarget(out.Plan); err != nil {
		return err
	}
	plan := out.Plan
	if plan.Before != nil {
		batchinput.Record(ctx, batchinput.Target{Environment: in.Environment, Site: in.Site, ResourceKind: "label", ResourceID: in.ID})
	}
	// Both the existing attachment and its desired target/value are owned by the
	// update. This also prevents a value rename racing with creation of that value.
	batchinput.Record(ctx, batchinput.Target{Environment: in.Environment, Site: in.Site, ResourceKind: "label." + plan.Target.Type, ResourceID: plan.Target.LUID, Name: plan.Desired.Value})
	return nil
}
