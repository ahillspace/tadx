package artifact_test

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/artifact"
	"path/filepath"
	"strings"
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

func TestAmbiguousArtifactSelectorReturnsBoundedCandidates(t *testing.T) {
	root := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	for index := 0; index < 22; index++ {
		metadata := validMetadata("Finance", "book-"+string(rune('a'+index)))
		metadata.SourceEnvironment = "environment-" + string(rune('a'+index))
		metadata.SourceSite = "site-" + string(rune('a'+index))
		metadata.SourceSiteLUID = "site-luid-" + string(rune('a'+index))
		if _, err := manager.Pull(context.Background(), artifact.WorkbookPull{Workspace: root, Filename: "Finance-" + metadata.TableauID + ".twb", Content: []byte("<workbook/>"), Metadata: metadata}); err != nil {
			t.Fatal(err)
		}
	}

	_, err := artifact.Resolve(context.Background(), root, artifact.Selector{Kind: "workbook", Name: "Finance"})
	if err == nil {
		t.Fatal("ambiguous selector unexpectedly succeeded")
	}
	var ambiguous *artifact.AmbiguousSelectorError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("error type = %T, want AmbiguousSelectorError", err)
	}
	if len(ambiguous.Candidates) != 20 || ambiguous.Truncated != 2 || ambiguous.FullStatusCommand != "tadx workspace status --full" {
		t.Fatalf("ambiguity details = %#v", ambiguous)
	}
	for index, candidate := range ambiguous.Candidates {
		if filepath.IsAbs(candidate.Path) || strings.Contains(candidate.Path, `\`) || candidate.Kind != "workbook" || candidate.Name != "Finance" || candidate.LUID == "" || candidate.SourceEnvironment == "" || candidate.SourceSite == "" || candidate.SiteLUID == "" {
			t.Fatalf("candidate[%d] = %#v", index, candidate)
		}
		if index > 0 && ambiguous.Candidates[index-1].Path >= candidate.Path {
			t.Fatalf("candidates are not sorted by path: %#v", ambiguous.Candidates)
		}
	}
	if err.Error() != "managed artifact selector is ambiguous across source identities" {
		t.Fatalf("error was not concise: %v", err)
	}
}
