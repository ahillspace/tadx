package datasource

// Compact output retains all decisions, values and ordered outcomes. The full
// projection also carries per-request evidence, including failed-element status.
func (out EditOutput) CompactOutput() any {
	type item struct {
		Index      int          `json:"index"`
		Op         string       `json:"op"`
		Target     EditSelector `json:"target"`
		Action     string       `json:"action,omitempty"`
		Status     string       `json:"status"`
		Before     EditState    `json:"before"`
		Intended   EditState    `json:"intended"`
		Observed   *EditState   `json:"observed,omitempty"`
		Submission string       `json:"submission,omitempty"`
	}
	rows := make([]item, len(out.Items))
	for i, r := range out.Items {
		rows[i] = item{r.Index, r.Op, r.Target, r.Action, r.Status, r.Before, r.Intended, r.Observed, r.Submission}
	}
	return struct {
		Mode        string `json:"mode"`
		Environment string `json:"environment"`
		EditTarget
		PreviewToken string   `json:"preview_token,omitempty"`
		Items        []item   `json:"items"`
		Details      string   `json:"details"`
		Help         []string `json:"help,omitempty"`
	}{out.Mode, out.Environment, out.EditTarget, out.PreviewToken, rows, "--full", out.Help}
}
func (out EditOutput) FullOutput() any { return out }

func (out CopyInspection) CompactOutput() any {
	n := min(len(out.Observations), 10)
	observations := make([]EditObservation, n)
	for i, o := range out.Observations[:n] {
		o.TableauRequestID = ""
		if o.Name == "" && len(o.Elements) > 10 {
			o.DisplayOmitted = len(o.Elements) - 10
			o.Elements = o.Elements[:10]
		}
		observations[i] = o
	}
	out.OmittedObservations = len(out.Observations) - n
	out.Observations = observations
	out.SourceMetadata = nil
	out.Details = "--full"
	return out
}
func (out CopyInspection) FullOutput() any { return out }
