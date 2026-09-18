package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

func TestMapArtifactResolutionErrorAddsContextualStatusRoute(t *testing.T) {
	cause := &artifact.AmbiguousSelectorError{Candidates: []artifact.AmbiguousCandidate{{Kind: "workbook", LUID: "wb-1", Path: "artifacts/workbook/Finance", SourceEnvironment: "production", SourceSite: "marketing", SiteLUID: "site-1"}}}
	err := mapArtifactResolutionError("workspace.artifact.delete", "development", "", cause)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || structured.Phase != errs.PhaseValidation || structured.Outcome != errs.OutcomeNotAttempted {
		t.Fatalf("mapped error = %#v", err)
	}
	if !strings.Contains(structured.CorrectiveAction, "tadx workspace status --workspace development --full") || err.Error() != "The managed artifact selector matched more than one exact artifact.: managed artifact selector is ambiguous across source identities" {
		t.Fatalf("mapped error lost concise recovery context: %v", err)
	}
}

func TestMapArtifactResolutionErrorCarriesBoundedCandidatesInJSON(t *testing.T) {
	candidates := make([]artifact.AmbiguousCandidate, 20)
	for index := range candidates {
		candidates[index] = artifact.AmbiguousCandidate{
			Kind:              "workbook",
			LUID:              fmt.Sprintf("wb-%02d", index),
			Name:              "Finance",
			Path:              "artifacts/workbook/" + strings.Repeat("nested/", 100) + fmt.Sprintf("Finance-%02d", index),
			SourceEnvironment: fmt.Sprintf("environment-%02d", index),
			SourceSite:        fmt.Sprintf("site-%02d", index),
			ServerOrigin:      "https://tableau.example.test",
			SiteLUID:          fmt.Sprintf("site-luid-%02d", index),
		}
	}
	cause := &artifact.AmbiguousSelectorError{Kind: "workbook", Name: "Finance", Candidates: candidates, Truncated: 3}
	err := mapArtifactResolutionError("workbook.publish", "ambiguous", "", cause)
	var rendered bytes.Buffer
	if renderErr := output.RenderError(&rendered, err, output.Options{JSON: true}); renderErr != nil {
		t.Fatal(renderErr)
	}
	var document struct {
		Output struct {
			Status            string                        `json:"status"`
			Workspace         string                        `json:"workspace"`
			Candidates        []artifact.AmbiguousCandidate `json:"candidates"`
			Truncated         int                           `json:"truncated"`
			FullStatusCommand string                        `json:"full_status_command"`
		} `json:"output"`
		Error struct {
			UpstreamCause string `json:"upstream_cause"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rendered.Bytes(), &document); err != nil {
		t.Fatalf("rendered JSON: %v\n%s", err, rendered.String())
	}
	if document.Output.Status != "ambiguous" || document.Output.Workspace != "ambiguous" || document.Output.Truncated != 3 || document.Output.FullStatusCommand != "tadx workspace status --workspace ambiguous --full" || !reflect.DeepEqual(document.Output.Candidates, candidates) {
		t.Fatalf("candidate output lost detail: %#v", document.Output)
	}
	if document.Error.UpstreamCause != "managed artifact selector is ambiguous across source identities" || strings.Contains(document.Error.UpstreamCause, candidates[0].Path) {
		t.Fatalf("cause was not concise: %#v", document.Error)
	}
}
