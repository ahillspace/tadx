package workbook_test

import (
	"context"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"strings"
	"testing"
)

type publishPublishPhaseKey struct{}
type publishPhasedPublishResolver struct {
	publishResolver
	phases int
}

func (r *publishPhasedPublishResolver) BeginProjectResolution(ctx context.Context) context.Context {
	r.phases++
	return context.WithValue(ctx, publishPublishPhaseKey{}, append([]workbookops.Record(nil), r.existing...))
}
func (r *publishPhasedPublishResolver) FindWorkbooks(ctx context.Context, _, _ string) ([]workbookops.Record, error) {
	return ctx.Value(publishPublishPhaseKey{}).([]workbookops.Record), nil
}

func TestPublishRetainedApplyAndPostUploadEachStartFreshProjectPhase(t *testing.T) {
	for _, afterUpload := range []bool{false, true} {
		t.Run(map[bool]string{false: "retained apply", true: "post upload"}[afterUpload], func(t *testing.T) {
			r := &publishPhasedPublishResolver{publishResolver: publishResolver{project: workbookops.Project{LUID: "project-1", Name: "Ops", Path: "Ops"}, existing: []workbookops.Record{{LUID: "original", Name: "Finance", ProjectLUID: "project-1"}}}}
			p := &publishPublisher{}
			action := workbookops.NewPublish(publishArtifactReader{artifact: workbookops.PublishArtifact{Path: "artifact", Filename: "Finance.twb", Name: "Finance"}}, r, p)
			plan, err := action.Plan(context.Background(), workbookops.PublishInput{ArtifactPath: "artifact", Environment: "production", Site: "site", Overwrite: true})
			if err != nil {
				t.Fatal(err)
			}
			change := func() {
				r.existing = []workbookops.Record{{LUID: "replacement", Name: "Finance", ProjectLUID: "project-1"}}
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
