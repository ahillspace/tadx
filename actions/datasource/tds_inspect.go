package datasource

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

const maxTDSElements = 100

type EditObservation struct {
	DisplayOmitted   int                `json:"display_omitted,omitempty"`
	ElementType      string             `json:"element_type"`
	Name             string             `json:"name,omitempty"`
	Absent           bool               `json:"absent,omitempty"`
	Elements         []value.TDSElement `json:"elements"`
	Omitted          int                `json:"omitted"`
	Complete         bool               `json:"complete"`
	Problem          string             `json:"problem,omitempty"`
	TableauRequestID string             `json:"tableau_request_id,omitempty"`
}

type CopyInspection struct {
	Checkout            *value.TDSCheckoutContext `json:"checkout_receipt,omitempty"`
	Files               *EditExports              `json:"files,omitempty"`
	OmittedObservations int                       `json:"omitted_observations,omitempty"`
	EditTarget
	Environment     string            `json:"environment"`
	TargetKind      string            `json:"target_kind"`
	ObservedAt      string            `json:"observed_at"`
	Complete        bool              `json:"complete"`
	Observations    []EditObservation `json:"observations"`
	SourceMetadata  *SchemaOutput     `json:"-"`
	MetadataWarning string            `json:"metadata_warning,omitempty"`
	Details         string            `json:"details,omitempty"`
	Help            []string          `json:"help,omitempty"`
}

func InspectTDSElements(ctx context.Context, reader TDSReader, kind string, target EditTarget, env string, fields, tables []string) (CopyInspection, error) {
	out := CopyInspection{EditTarget: target, Environment: env, TargetKind: kind, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Complete: true}
	id := target.WorkCopyLUID
	switch kind {
	case "datasource":
		id = target.DatasourceLUID
	case "workcopy":
	default:
		return out, editUsage("Specify a datasource or work copy.")
	}
	if !value.ValidTDSLUID(id) {
		return out, editUsage("Inspecting TDS elements requires an exact LUID.")
	}
	if len(fields)+len(tables) > maxEditCount {
		return out, editUsage("Inspect up to 50 exact fields and tables in one request.")
	}
	type query struct{ typ, name string }
	var queries []query
	if len(fields)+len(tables) == 0 {
		queries = []query{{"Column", ""}, {"FieldDescription", ""}, {"TableDescription", ""}}
	} else {
		for _, name := range fields {
			q := query{"Column", name}
			if strings.TrimSpace(name) == "" || slices.Contains(queries, q) {
				return out, editUsage("Each field must have a distinct exact name.")
			}
			queries = append(queries, q, query{"FieldDescription", name})
		}
		for _, name := range tables {
			q := query{"TableDescription", name}
			if strings.TrimSpace(name) == "" || slices.Contains(queries, q) {
				return out, editUsage("Each table must have a distinct exact name.")
			}
			queries = append(queries, q)
		}
	}
	var problems []error
	for _, q := range queries {
		response, err := reader.ReadTDSElements(ctx, kind, id, q.typ, q.name)
		obs := EditObservation{ElementType: q.typ, Name: q.name, Absent: response.Absent, Complete: err == nil, TableauRequestID: response.TableauRequestID}
		for _, e := range response.Elements {
			if e.Error != nil {
				obs.Complete = false
			}
		}
		n := min(len(response.Elements), maxTDSElements)
		obs.Elements = response.Elements[:n]
		if obs.Elements == nil {
			obs.Elements = []value.TDSElement{}
		}
		obs.Omitted = len(response.Elements) - n
		if obs.Omitted > 0 {
			obs.Complete = false
		}
		if err != nil {
			obs.Problem = err.Error()
			problems = append(problems, fmt.Errorf("read %s: %w", q.typ, err))
		} else if !obs.Complete && obs.Omitted == 0 {
			problems = append(problems, fmt.Errorf("%s has incomplete read records", q.typ))
		}
		out.Complete = out.Complete && obs.Complete
		out.Observations = append(out.Observations, obs)
		if ctx.Err() != nil {
			out.Complete = false
			break
		}
	}
	if kind == "workcopy" {
		out.Help = []string{commandhint.Environment(env, "content", "datasource", "work-copy", "--work-copy", id)}
	}
	if err := errors.Join(problems...); err != nil {
		return out, &errs.Error{ID: "datasource.tds.read_incomplete", Kind: errs.KindOperation, Summary: "Some TDS observations could not be read.", Resource: id, Environment: env, Site: target.SiteLUID, Cause: err, TableauRequestID: errs.TableauRequestID(err), Phase: errs.PhaseVerification, Outcome: errs.OutcomeNotAttempted}
	}
	return out, nil
}
