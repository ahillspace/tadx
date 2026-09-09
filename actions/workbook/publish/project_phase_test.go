package publish_test

import (
	"context"
	"github.com/ahillspace/tadx/actions/workbook/publish"
	"strings"
	"testing"
)

type publishPhaseKey struct{}
type phasedPublishResolver struct {
	resolver
	phases int
}

func (r *phasedPublishResolver) BeginProjectResolution(ctx context.Context) context.Context {
	r.phases++
	return context.WithValue(ctx, publishPhaseKey{}, append([]publish.Workbook(nil), r.existing...))
}
func (r *phasedPublishResolver) FindWorkbooks(ctx context.Context, _, _ string) ([]publish.Workbook, error) {
	return ctx.Value(publishPhaseKey{}).([]publish.Workbook), nil
}

func TestRetainedApplyAndPostUploadEachStartFreshProjectPhase(t *testing.T) {
	for _, afterUpload := range []bool{false, true} {
		t.Run(map[bool]string{false: "retained apply", true: "post upload"}[afterUpload], func(t *testing.T) {
			r := &phasedPublishResolver{resolver: resolver{project: publish.Project{LUID: "project-1", Name: "Ops", Path: "Ops"}, existing: []publish.Workbook{{LUID: "original", Name: "Finance", ProjectLUID: "project-1"}}}}
			p := &publisher{}
			action := publish.New(artifactReader{artifact: publish.Artifact{Path: "artifact", Filename: "Finance.twb", Name: "Finance"}}, r, p)
			plan, err := action.Plan(context.Background(), publish.Input{ArtifactPath: "artifact", Environment: "production", Site: "site", Overwrite: true})
			if err != nil {
				t.Fatal(err)
			}
			change := func() {
				r.existing = []publish.Workbook{{LUID: "replacement", Name: "Finance", ProjectLUID: "project-1"}}
			}
			if afterUpload {
				p.onPrepare = change
			} else {
				change()
			}
			_, err = action.Apply(context.Background(), plan)
			if err == nil || !strings.Contains(err.Error(), "changed") || p.calls != 0 {
				t.Fatalf("error=%v commits=%d", err, p.calls)
			}
			want := 2
			if afterUpload {
				want = 3
			}
			if r.phases != want {
				t.Fatalf("phases=%d want%d", r.phases, want)
			}
		})
	}
}
