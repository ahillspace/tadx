package value

import (
	"encoding/json"
	"time"
)

// SavedExecution contains output only, never arguments or replay instructions.
type SavedExecution struct {
	RecordedAt           time.Time       `json:"recorded_at"`
	Operation            string          `json:"operation"`
	ExitCode             int             `json:"exit_code"`
	Result               json.RawMessage `json:"result,omitempty"`
	Unavailable          string          `json:"unavailable,omitempty"`
	RequiredCapabilities []string        `json:"required_capabilities,omitempty"`
}

// IsSavedResult marks an already bounded full projection for display without
// applying the ordinary compact projection a second time.
func (SavedExecution) IsSavedResult() bool { return true }
