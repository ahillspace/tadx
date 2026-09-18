package app_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/artifact"
)

func TestAmbiguousArtifactCLIReportsExactCandidates(t *testing.T) {
	configPath := writePhaseOneConfigWithSite(t, "https://tableau.example.test", "marketing")
	workspace := createNamedWorkspace(t, configPath, "ambiguous")
	manager := artifact.NewWorkbookManager(nil)
	for _, candidate := range []struct {
		luid, environment, site, siteLUID string
	}{
		{luid: "wb-a", environment: "production", site: "marketing", siteLUID: "site-a"},
		{luid: "wb-b", environment: "staging", site: "sandbox", siteLUID: "site-b"},
	} {
		_, err := manager.Pull(context.Background(), artifact.WorkbookPull{
			Workspace: workspace,
			Filename:  fmt.Sprintf("Finance-%s.twb", candidate.luid),
			Content:   []byte("<workbook/>"),
			Metadata: artifact.WorkbookMetadata{
				Name:               candidateName,
				TableauID:          candidate.luid,
				SourceServerOrigin: "https://tableau.example.test",
				SourceSiteLUID:     candidate.siteLUID,
				SourceEnvironment:  candidate.environment,
				SourceSite:         candidate.site,
				SourceProjectName:  "Ops",
				SourceProjectID:    "project-1",
			},
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	var output bytes.Buffer
	exit := app.Run(context.Background(), []string{
		"content", "workbook", "publish", "--workspace", "ambiguous", "--environment", "production",
		"--artifact-name", candidateName, "--project-id", "project-1", "--preview", "--json",
	}, &output, app.Options{ConfigPath: configPath, MutationsEnabled: true})
	if exit == 0 {
		t.Fatalf("ambiguous selector succeeded: %s", output.String())
	}
	for _, want := range []string{
		"artifacts/workbook/",
		"wb-a",
		"wb-b",
		"production",
		"staging",
		"site-a",
		"site-b",
		"marketing",
		"sandbox",
		"tadx workspace status --workspace ambiguous --full",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("ambiguous selector output missing %q: %s", want, output.String())
		}
	}
}

const candidateName = "Finance"
