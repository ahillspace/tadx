package artifact_test

import (
	"context"
	"github.com/ahillspace/tadx/internal/artifact"
	"testing"
	"time"
)

func TestPublishArtifactExactNameAndSourceID(t *testing.T) {
	root := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	for _, id := range []string{"book-1", "book-2"} {
		_, err := manager.Pull(context.Background(), artifact.WorkbookPull{Workspace: root, Filename: "Finance.twb", Content: []byte("<workbook/>"), Metadata: validMetadata("Finance", id)})
		if err != nil {
			t.Fatal(err)
		}
		selected, err := artifact.Resolve(context.Background(), root, artifact.Selector{Kind: "workbook", LUID: id})
		if err != nil || selected.LUID != id {
			t.Fatalf("selection=%#v err=%v", selected, err)
		}
		selected, err = artifact.Resolve(context.Background(), root, artifact.Selector{Kind: "workbook", Name: "Finance"})
		if id == "book-1" && (err != nil || selected.LUID != id) {
			t.Fatalf("unique name=%#v err=%v", selected, err)
		}
		if id == "book-2" && err == nil {
			t.Fatal("ambiguous name accepted")
		}
	}
	if _, err := artifact.Resolve(context.Background(), root, artifact.Selector{Kind: "workbook", Name: "finance"}); err == nil {
		t.Fatal("non-exact name accepted")
	}
}
