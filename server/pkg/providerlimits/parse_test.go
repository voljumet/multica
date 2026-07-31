package providerlimits

import (
	"testing"
	"time"
)

func TestParseFromError_SessionLimitWithReset(t *testing.T) {
	observed := time.Date(2026, 7, 29, 18, 0, 0, 0, time.UTC)
	snap := ParseFromError("claude", "You've hit your session limit · resets 3:45pm", observed)
	if snap == nil {
		t.Fatal("expected snapshot, got nil")
	}
	if snap.Status != StatusExhausted {
		t.Fatalf("status = %q, want exhausted", snap.Status)
	}
	if snap.Source != SourceTaskError {
		t.Fatalf("source = %q, want task_error", snap.Source)
	}
	if snap.Provider != "claude" {
		t.Fatalf("provider = %q, want claude", snap.Provider)
	}
	if snap.ObservedAt != "2026-07-29T18:00:00Z" {
		t.Fatalf("observed_at = %q", snap.ObservedAt)
	}
	if len(snap.Windows) != 1 {
		t.Fatalf("windows len = %d", len(snap.Windows))
	}
	w := snap.Windows[0]
	if w.Kind != KindFiveHour {
		t.Fatalf("kind = %q, want five_hour", w.Kind)
	}
	if w.Label != "session limit" {
		t.Fatalf("label = %q", w.Label)
	}
	if w.ResetsLabel == nil || *w.ResetsLabel != "3:45pm" {
		t.Fatalf("resets_label = %v, want 3:45pm", w.ResetsLabel)
	}
	if w.UsedPct != nil {
		t.Fatalf("used_pct should be nil on task_error path, got %v", *w.UsedPct)
	}
	if w.ResetsAt != nil {
		t.Fatalf("resets_at should be nil without absolute timestamp, got %v", *w.ResetsAt)
	}
}

func TestParseFromError_WeeklyAndOpusAndCurlyApostrophe(t *testing.T) {
	observed := time.Now()
	cases := []struct {
		msg  string
		kind string
		lab  string
		rst  string
	}{
		{
			msg:  "You've hit your weekly limit · resets Mon 12:00am",
			kind: KindSevenDay,
			lab:  "weekly limit",
			rst:  "Mon 12:00am",
		},
		{
			msg:  "You’ve hit your Opus limit · resets 3:45pm", // U+2019
			kind: KindOpus,
			lab:  "Opus limit",
			rst:  "3:45pm",
		},
		{
			msg:  "API Error: You've hit your session limit - resets at 9:00 AM\nmore noise",
			kind: KindFiveHour,
			lab:  "session limit",
			rst:  "9:00 AM",
		},
	}
	for _, c := range cases {
		snap := ParseFromError("claude", c.msg, observed)
		if snap == nil {
			t.Fatalf("nil snapshot for %q", c.msg)
		}
		w := snap.Windows[0]
		if w.Kind != c.kind || w.Label != c.lab {
			t.Fatalf("%q: kind/label = %q/%q, want %q/%q", c.msg, w.Kind, w.Label, c.kind, c.lab)
		}
		if w.ResetsLabel == nil || *w.ResetsLabel != c.rst {
			t.Fatalf("%q: resets_label = %v, want %q", c.msg, w.ResetsLabel, c.rst)
		}
	}
}

func TestParseFromError_NoMatch(t *testing.T) {
	observed := time.Now()
	for _, msg := range []string{
		"",
		"rate limit exceeded for tier 3",
		"API Error: 429 Too Many Requests",
		"quota exceeded for project foo",
		"You've hit your limit", // too generic — no session/weekly/opus
		"session limit almost reached",
	} {
		if snap := ParseFromError("claude", msg, observed); snap != nil {
			t.Fatalf("expected nil for %q, got %+v", msg, snap)
		}
	}
}

func TestParseFromError_NormalizesProvider(t *testing.T) {
	snap := ParseFromError("  Claude ", "You've hit your session limit · resets 1pm", time.Now())
	if snap == nil || snap.Provider != "claude" {
		t.Fatalf("provider normalize failed: %+v", snap)
	}
}
