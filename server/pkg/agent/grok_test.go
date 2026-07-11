package agent

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewReturnsGrokBackend(t *testing.T) {
	t.Parallel()
	b, err := New("grok", Config{ExecutablePath: "/nonexistent/grok"})
	if err != nil {
		t.Fatalf("New(grok) error: %v", err)
	}
	if _, ok := b.(*grokBackend); !ok {
		t.Fatalf("expected *grokBackend, got %T", b)
	}
}

func TestBuildGrokArgs(t *testing.T) {
	t.Parallel()
	logger := slog.Default()
	args := buildGrokArgs("hello world", ExecOptions{
		Cwd:             "/tmp/work",
		Model:           "grok-build",
		ResumeSessionID: "sess-123",
		ThinkingLevel:   "high",
	}, logger)

	want := []string{
		"--no-auto-update",
		"-p", "hello world",
		"--output-format", "streaming-json",
		"--always-approve",
		"--cwd", "/tmp/work",
		"-m", "grok-build",
		"-r", "sess-123",
		"--reasoning-effort", "high",
	}
	if strings.Join(args, "|") != strings.Join(want, "|") {
		t.Fatalf("buildGrokArgs() = %v, want %v", args, want)
	}
}

func TestBuildGrokArgsFiltersBlockedCustomArgs(t *testing.T) {
	t.Parallel()
	args := buildGrokArgs("hi", ExecOptions{
		CustomArgs: []string{"--output-format", "plain", "--always-approve", "--extra", "ok"},
	}, slog.Default())

	joined := strings.Join(args, " ")
	if strings.Contains(joined, "--output-format plain") {
		t.Fatalf("blocked --output-format should be filtered: %v", args)
	}
	if !strings.Contains(joined, "--extra ok") {
		t.Fatalf("expected custom arg preserved: %v", args)
	}
}

func TestHandleGrokEvent(t *testing.T) {
	t.Parallel()
	st := newGrokEventState()

	msgs := handleGrokEvent(grokStreamEvent{Type: "thought", Data: "hmm"}, st)
	if len(msgs) != 1 || msgs[0].Type != MessageThinking {
		t.Fatalf("thought event: got %#v", msgs)
	}

	msgs = handleGrokEvent(grokStreamEvent{Type: "text", Data: "Hello"}, st)
	if len(msgs) != 1 || msgs[0].Type != MessageText || st.output.String() != "Hello" {
		t.Fatalf("text event: got %#v output=%q", msgs, st.output.String())
	}

	msgs = handleGrokEvent(grokStreamEvent{
		Type: "end", StopReason: "EndTurn", SessionID: "abc",
	}, st)
	if len(msgs) != 0 || !st.endSeen || st.sessionID != "abc" || st.finalStatus != "completed" {
		t.Fatalf("end event: endSeen=%v session=%q status=%q", st.endSeen, st.sessionID, st.finalStatus)
	}

	handleGrokEvent(grokStreamEvent{Type: "end", StopReason: "MaxTurns"}, st)
	if st.finalStatus != "failed" {
		t.Fatalf("non-ok stop reason should fail, got %q", st.finalStatus)
	}
}

func fakeGrokStreamingScript() string {
	return `#!/bin/sh
while IFS= read -r line; do :; done
printf '%s\n' \
  '{"type":"thought","data":"ok"}' \
  '{"type":"text","data":"pong"}' \
  '{"type":"end","stopReason":"EndTurn","sessionId":"fake-session"}'
`
}

func TestGrokBackendExecute(t *testing.T) {
	t.Parallel()
	fakePath := filepath.Join(t.TempDir(), "grok")
	if err := os.WriteFile(fakePath, []byte(fakeGrokStreamingScript()), 0o755); err != nil {
		t.Fatalf("write fake grok: %v", err)
	}

	backend, err := New("grok", Config{ExecutablePath: fakePath, Logger: slog.Default()})
	if err != nil {
		t.Fatalf("new grok backend: %v", err)
	}

	session, err := backend.Execute(context.Background(), "ping", ExecOptions{Cwd: t.TempDir()})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var result Result
	select {
	case result = <-session.Result:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for grok result")
	}

	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed (err=%q)", result.Status, result.Error)
	}
	if result.Output != "pong" {
		t.Fatalf("output = %q, want pong", result.Output)
	}
	if result.SessionID != "fake-session" {
		t.Fatalf("sessionID = %q, want fake-session", result.SessionID)
	}
}

func TestDiscoverGrokModelsParsesOutput(t *testing.T) {
	t.Parallel()
	fakePath := filepath.Join(t.TempDir(), "grok")
	script := `#!/bin/sh
if [ "$1" = "models" ]; then
  cat <<'EOF'
You are logged in with grok.com.

Default model: grok-composer-2.5-fast

Available models:
  * grok-composer-2.5-fast (default)
  - grok-build
EOF
  exit 0
fi
exit 1
`
	if err := os.WriteFile(fakePath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake grok: %v", err)
	}

	models, err := discoverGrokModels(context.Background(), fakePath)
	if err != nil {
		t.Fatalf("discoverGrokModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("models = %#v, want 2 entries", models)
	}
	if !models[0].Default || models[0].ID != "grok-composer-2.5-fast" {
		t.Fatalf("default model = %#v", models[0])
	}
}

func TestDiscoverGrokModelsRealBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-binary smoke in short mode")
	}
	path, err := exec.LookPath("grok")
	if err != nil {
		t.Skip("grok not on PATH")
	}
	models, err := discoverGrokModels(context.Background(), path)
	if err != nil {
		t.Fatalf("discoverGrokModels: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected at least one model from real grok binary")
	}
}