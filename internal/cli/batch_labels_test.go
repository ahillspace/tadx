package cli

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/ahillspace/tadx/actions/contentlabel"
	"github.com/ahillspace/tadx/internal/capability"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
	"github.com/spf13/cobra"
)

type batchLabelProvider struct {
	mu     sync.Mutex
	labels map[string]value.ContentLabel
	writes int
}

func (p *batchLabelProvider) GetLabel(_ context.Context, id string) (value.ContentLabel, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.labels[id], nil
}
func (p *batchLabelProvider) GetLabels(_ context.Context, target value.LabelTarget, _ []string) ([]value.ContentLabel, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var labels []value.ContentLabel
	for _, label := range p.labels {
		if label.Type == target.Type && label.TargetLUID == target.LUID {
			labels = append(labels, label)
		}
	}
	return labels, nil
}
func (*batchLabelProvider) GetLabelValue(_ context.Context, name string) (value.LabelValue, error) {
	return value.LabelValue{Name: name, Category: "quality"}, nil
}
func (p *batchLabelProvider) UpdateLabel(_ context.Context, id string, change value.LabelUpdate) (value.ContentLabel, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	label := p.labels[id]
	label.Value, label.Message, label.Active, label.Elevated = change.Value, change.Message, change.Active, change.Elevated
	p.labels[id] = label
	p.writes++
	return label, nil
}
func (p *batchLabelProvider) SetLabel(_ context.Context, target value.LabelTarget, change value.LabelUpdate) (value.ContentLabel, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writes++
	label := value.ContentLabel{LUID: fmt.Sprintf("new-%d", p.writes), TargetLUID: target.LUID, Type: target.Type, Value: change.Value, Message: change.Message, Active: change.Active, Elevated: change.Elevated}
	p.labels[label.LUID] = label
	return label, nil
}
func (p *batchLabelProvider) Execute(ctx context.Context, in contentlabel.UpdateInput, preview bool) (contentlabel.UpdateOutput, error) {
	in.Site = "site"
	return contentlabel.Update(ctx, p, p, in, preview)
}
func (*batchLabelProvider) ResolveBatchResource(context.Context, string, string, identity.Selector) (identity.LUID, error) {
	return "", fmt.Errorf("unexpected resource resolution")
}
func (*batchLabelProvider) AuthorizeBatchPreparation(context.Context, string) error { return nil }

func TestBatchLabelTargetsRespectAttachmentAndValueIdentity(t *testing.T) {
	for _, test := range []struct {
		name, body string
		writes     int
	}{
		{"attachment ID and target/value aliases", `{"items":[{"id":"l1","message":"A"},{"type":"datasource","target-id":"d1","value":"Certified","message":"B"}]}`, 0},
		{"renamed value and concurrent creation", `{"items":[{"id":"l1","value":"Reviewed"},{"type":"datasource","target-id":"d1","value":"Reviewed","message":"B"}]}`, 0},
		{"same attachment selected by its old value", `{"items":[{"id":"l1","value":"Reviewed"},{"type":"datasource","target-id":"d1","value":"Certified","message":"B"}]}`, 0},
		{"different values on one asset", `{"items":[{"id":"l1","message":"A"},{"type":"datasource","target-id":"d1","value":"Reviewed","message":"B"}]}`, 2},
		{"same value on different assets", `{"items":[{"id":"l1","message":"A"},{"type":"datasource","target-id":"d2","value":"Certified","message":"B"}]}`, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := &batchLabelProvider{labels: map[string]value.ContentLabel{"l1": {LUID: "l1", Type: "datasource", TargetLUID: "d1", Value: "Certified", Active: true}}}
			factory := func(renderer Renderer) *cobra.Command {
				root := &cobra.Command{Use: "tadx", SilenceErrors: true, SilenceUsage: true}
				catalog := &cobra.Command{Use: "catalog"}
				catalog.AddCommand(contentcli.NewLabels(contentcli.LabelDependencies{Updater: provider, Renderer: renderer, BatchTargets: provider}))
				root.AddCommand(catalog)
				return root
			}
			renderer := &batchCollector{}
			root := factory(renderer)
			command, _, err := root.Find([]string{"catalog", "label", "update"})
			if err != nil {
				t.Fatal(err)
			}
			attachBatchWithOptions(root, command, capability.BatchOptions()["content.label.update"], renderer, factory)
			root.SetArgs([]string{"catalog", "label", "update", "--environment", "test", "--batch-file", batchFile(t, test.body)})
			err = root.ExecuteContext(t.Context())
			if test.writes == 0 {
				if err == nil || !strings.Contains(err.Error(), "batch item 2 duplicates batch item 1") {
					t.Fatalf("err=%v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if provider.writes != test.writes {
				t.Fatalf("writes=%d, want %d", provider.writes, test.writes)
			}
		})
	}
}

func TestBatchPreparedLabelAbsenceCannotRetargetNewAttachment(t *testing.T) {
	provider := &batchLabelProvider{labels: map[string]value.ContentLabel{}}
	in := contentlabel.UpdateInput{Environment: "test", Type: "datasource", TargetID: "d1", Value: new("Reviewed"), Message: new("requested")}
	out, err := provider.Execute(t.Context(), in, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := in.PinTarget(out.Plan); err != nil {
		t.Fatal(err)
	}
	provider.labels["new-generation"] = value.ContentLabel{LUID: "new-generation", Type: "datasource", TargetLUID: "d1", Value: "Reviewed", Active: true, Message: "independent change"}
	_, err = provider.Execute(t.Context(), in, false)
	if err == nil || provider.writes != 0 || provider.labels["new-generation"].Message != "independent change" {
		t.Fatalf("writes=%d err=%v", provider.writes, err)
	}
}
