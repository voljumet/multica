package providerlimits

import (
	"regexp"
	"strings"
	"time"
	"unicode"
)

// Claude subscription plan-limit messages look like:
//
//	You've hit your session limit · resets 3:45pm
//	You've hit your weekly limit · resets Mon 12:00am
//	You've hit your Opus limit · resets 3:45pm
//
// Apostrophe may be ASCII ' or U+2019. Separator after "limit" is usually
// middle-dot ·, but dash / pipe / plain space also appear in copied text.
//
// Capture groups:
//  1. window name: session | weekly | opus
//  2. optional reset tail after "resets" / "resets at"
var claudePlanLimitRe = regexp.MustCompile(
	`(?i)you(?:'|\x{2019})ve hit your (session|weekly|opus) limit` +
		`(?:\s*(?:[·•|—–\-]|)\s*resets?(?:\s+at)?\s+([^\n\r]+))?`,
)

// ParseFromError extracts a provider-limits Snapshot from a free-form agent
// error string. Returns nil when the message is not a recognized plan-limit
// hit — generic quota / 429 text is intentionally ignored so we do not
// overwrite a previous useful snapshot with an unhelpful "unknown" row.
//
// provider is the runtime's protocol family (e.g. "claude"); it is stored on
// the snapshot for display even though matching is message-driven.
func ParseFromError(provider, errMsg string, observedAt time.Time) *Snapshot {
	trimmed := strings.TrimSpace(errMsg)
	if trimmed == "" {
		return nil
	}

	m := claudePlanLimitRe.FindStringSubmatch(trimmed)
	if m == nil {
		return nil
	}

	windowName := strings.ToLower(m[1])
	kind, label := windowMeta(windowName)

	var resetsLabel *string
	if len(m) > 2 {
		if tail := cleanResetsTail(m[2]); tail != "" {
			resetsLabel = &tail
		}
	}

	win := Window{
		Kind:        kind,
		UsedPct:     nil,
		ResetsAt:    nil,
		ResetsLabel: resetsLabel,
		Label:       label,
	}

	// Keep the matched sentence (first line) as a short diagnostic message.
	msg := firstLine(trimmed)
	if len(msg) > 240 {
		msg = msg[:240]
	}

	snap := NewExhausted(normalizeProvider(provider), SourceTaskError, observedAt, []Window{win}, msg)
	return &snap
}

func windowMeta(name string) (kind, label string) {
	switch name {
	case "session":
		return KindFiveHour, "session limit"
	case "weekly":
		return KindSevenDay, "weekly limit"
	case "opus":
		return KindOpus, "Opus limit"
	default:
		return KindUnknown, name + " limit"
	}
}

func cleanResetsTail(raw string) string {
	s := strings.TrimSpace(raw)
	// Drop trailing sentence punctuation / ellipsis that sometimes trails
	// the reset phrase when the error is concatenated with more text.
	s = strings.TrimRightFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || r == '.' || r == ',' || r == ';' || r == '…'
	})
	// Cap length so a pathological error cannot bloat metadata.
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func normalizeProvider(p string) string {
	p = strings.TrimSpace(strings.ToLower(p))
	if p == "" {
		return "unknown"
	}
	return p
}
