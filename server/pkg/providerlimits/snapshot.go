// Package providerlimits records the last-known provider plan-limit state for
// a Multica agent runtime (e.g. Claude Code's rolling 5-hour / weekly seat
// allowance). Slice 1 only captures limits from task failure text when an
// agent hits a hard wall; a later probe can write the same Snapshot shape
// with SourceProbe and non-nil UsedPct.
package providerlimits

import "time"

// Status values for Snapshot.Status.
const (
	StatusOK          = "ok"
	StatusExhausted   = "exhausted"
	StatusUnknown     = "unknown"
	StatusUnsupported = "unsupported"
)

// Source values for Snapshot.Source.
const (
	SourceTaskError = "task_error"
	SourceProbe     = "probe"
)

// Window kind values.
const (
	KindFiveHour = "five_hour"
	KindSevenDay = "seven_day"
	KindOpus     = "opus"
	KindUnknown  = "unknown"
)

// Snapshot is the JSON shape stored at
// agent_runtime.metadata.provider_limits and rendered on the runtime detail
// page. Fields are optional so partial observations (reset label without a
// percentage, or exhausted without a parseable reset) still serialize cleanly.
type Snapshot struct {
	Provider    string   `json:"provider"`
	Status      string   `json:"status"`
	Source      string   `json:"source"`
	ObservedAt  string   `json:"observed_at"` // RFC3339 UTC
	Windows     []Window `json:"windows"`
	Message     string   `json:"message,omitempty"`
}

// Window is one plan window (5h session, weekly, Opus-only, …).
type Window struct {
	Kind        string   `json:"kind"`
	UsedPct     *float64 `json:"used_pct"`               // 0–100; null when unknown
	ResetsAt    *string  `json:"resets_at"`              // RFC3339 when parseable
	ResetsLabel *string  `json:"resets_label,omitempty"` // raw "3:45pm" / "Mon 12:00am"
	Label       string   `json:"label,omitempty"`        // human "session limit"
}

// NewExhausted builds a Snapshot for a recognized plan-limit hit.
func NewExhausted(provider, source string, observedAt time.Time, windows []Window, message string) Snapshot {
	if windows == nil {
		windows = []Window{}
	}
	return Snapshot{
		Provider:   provider,
		Status:     StatusExhausted,
		Source:     source,
		ObservedAt: observedAt.UTC().Format(time.RFC3339),
		Windows:    windows,
		Message:    message,
	}
}
