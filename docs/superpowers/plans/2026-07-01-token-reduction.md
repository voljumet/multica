# Token Reduction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce LLM API token consumption per agent task by shipping the slim runtime brief to production and disabling session resumption for comment-triggered tasks.

**Architecture:** Two independent, minimal changes to the Go daemon. Change 1 flips one default value in a feature flag function. Change 2 adds one conditional nil-out before `ExecOptions` is built in `runTask`.

**Tech Stack:** Go, existing `featureflag.Service` (already wired), existing `daemon.Task` struct fields.

## Global Constraints

- All Go code must pass `make test` (Go test suite) and `go vet ./...`
- No new dependencies
- Commit each task independently with conventional prefix `fix(daemon):`

---

### Task 1: Enable slim brief by default

**Files:**
- Modify: `server/internal/daemon/execenv/runtime_config_flag.go` — change `useSlimBrief()` default from `false` to `true`
- Test: `server/internal/daemon/execenv/runtime_config_test.go` — add a test asserting slim output when no flag service is wired (the nil-service default path)

**Interfaces:**
- Consumes: nothing from other tasks
- Produces: nothing consumed by Task 2

- [ ] **Step 1: Write the failing test**

Open `server/internal/daemon/execenv/runtime_config_test.go` and add this test after the existing ones (the test must NOT call `withSlimBrief` — it verifies the bare default path):

```go
// TestBuildMetaSkillContent_DefaultIsSlim verifies that useSlimBrief()
// returns true when no feature flag service is wired (i.e. the bare
// package default). This is the production path after MUL-3560 ships.
func TestBuildMetaSkillContent_DefaultIsSlim(t *testing.T) {
	// Ensure no flag service is wired for this test.
	saved := runtimeFlags.Load()
	runtimeFlags.Store(nil)
	t.Cleanup(func() { runtimeFlags.Store(saved) })

	if !useSlimBrief() {
		t.Fatal("expected useSlimBrief() to return true by default (production path); got false")
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
cd server && go test ./internal/daemon/execenv/... -run TestBuildMetaSkillContent_DefaultIsSlim -v
```

Expected: `FAIL` — `expected useSlimBrief() to return true by default; got false`

- [ ] **Step 3: Flip the default in `runtime_config_flag.go`**

In `server/internal/daemon/execenv/runtime_config_flag.go`, change the last line of `useSlimBrief()`:

```go
// Before:
func useSlimBrief() bool {
	return briefFlags().IsEnabled(context.Background(), runtimeBriefSlimFlag, false)
}

// After:
func useSlimBrief() bool {
	return briefFlags().IsEnabled(context.Background(), runtimeBriefSlimFlag, true)
}
```

- [ ] **Step 4: Run the new test to confirm it passes**

```bash
cd server && go test ./internal/daemon/execenv/... -run TestBuildMetaSkillContent_DefaultIsSlim -v
```

Expected: `PASS`

- [ ] **Step 5: Run the full execenv test suite to confirm no regressions**

```bash
cd server && go test ./internal/daemon/execenv/... -v 2>&1 | tail -30
```

Expected: all tests pass. The existing `runtime_config_test.go` already runs both legacy (default) and slim (`withSlimBrief`) sub-paths; after this change the "default" subpath exercises slim, but the legacy subpath is still exercised by tests that explicitly wire the flag OFF via a static provider.

> **Note:** Some existing tests that relied on legacy-by-default may now generate slim output. If any assertion fails because it expected legacy text, update the assertion to expect slim output — the slim content is correct; the legacy assertion was testing the old default.

- [ ] **Step 6: Commit**

```bash
git add server/internal/daemon/execenv/runtime_config_flag.go \
        server/internal/daemon/execenv/runtime_config_test.go
git commit -m "fix(daemon): enable slim runtime brief by default in production

The runtime_brief_slim flag has been running in staging. Flip the
default from false to true so production gets the same ~7k-char
reduction per task without needing a YAML or env-var override.

Rollback: FF_RUNTIME_BRIEF_SLIM=false (no redeploy needed)."
```

---

### Task 2: Disable session resumption for comment-triggered tasks

**Files:**
- Modify: `server/internal/daemon/daemon.go` — gate `PriorSessionID` before building `execOpts` in `runTask` (~line 3908)
- Test: `server/internal/daemon/daemon_test.go` — add a table-driven test asserting that comment tasks never pass `ResumeSessionID`

**Interfaces:**
- Consumes: `Task.TriggerCommentID` (non-empty string when task is comment-triggered), `Task.PriorSessionID`
- Produces: nothing consumed by other tasks

- [ ] **Step 1: Write the failing test**

Open `server/internal/daemon/daemon_test.go`. Add this test after `TestGateResumeToReusedWorkdir` (search for that name):

```go
// TestCommentTaskDropsPriorSession verifies that comment-triggered tasks
// never pass --resume to the backend, regardless of whether the server
// sent a PriorSessionID. Session history from prior runs accumulates
// unbounded input tokens over back-and-forth comment cycles; comment
// tasks re-read issue context fresh via CLI so the history adds nothing.
func TestCommentTaskDropsPriorSession(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		triggerCommentID string // non-empty → comment task
		priorSessionID   string
		wantResume       string // expected ResumeSessionID passed to backend
	}{
		{
			name:             "comment task with prior session: resume must be dropped",
			triggerCommentID: "comment-abc",
			priorSessionID:   "session-xyz",
			wantResume:       "",
		},
		{
			name:             "comment task without prior session: no-op",
			triggerCommentID: "comment-abc",
			priorSessionID:   "",
			wantResume:       "",
		},
		{
			name:             "assignment task with prior session: resume kept",
			triggerCommentID: "", // assignment task (no trigger comment)
			priorSessionID:   "session-xyz",
			wantResume:       "session-xyz",
		},
		{
			name:             "assignment task without prior session: no-op",
			triggerCommentID: "",
			priorSessionID:   "",
			wantResume:       "",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := commentTaskPriorSession(tc.triggerCommentID, tc.priorSessionID)
			if got != tc.wantResume {
				t.Errorf("commentTaskPriorSession(%q, %q) = %q, want %q",
					tc.triggerCommentID, tc.priorSessionID, got, tc.wantResume)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
cd server && go test ./internal/daemon/... -run TestCommentTaskDropsPriorSession -v
```

Expected: `FAIL` — `undefined: commentTaskPriorSession`

- [ ] **Step 3: Add the helper and wire it into `runTask`**

In `server/internal/daemon/daemon.go`, directly above the `execOpts` assignment (~line 3908), add:

```go
// commentTaskPriorSession returns the session ID to resume, dropping it
// for comment-triggered tasks. Comment tasks re-read issue context and
// history fresh via the multica CLI on every run; replaying prior
// session history adds no new facts and accumulates unbounded input
// tokens over back-and-forth comment cycles.
func commentTaskPriorSession(triggerCommentID, priorSessionID string) string {
	if triggerCommentID != "" {
		return ""
	}
	return priorSessionID
}
```

Then change the `ExecOptions` construction (around line 3908) from:

```go
execOpts := agent.ExecOptions{
	Cwd:                       env.WorkDir,
	Model:                     model,
	ThreadName:                deriveTaskThreadName(task),
	Timeout:                   d.cfg.AgentTimeout,
	SemanticInactivityTimeout: d.cfg.CodexSemanticInactivityTimeout,
	ResumeSessionID:           task.PriorSessionID,
	ExtraArgs:                 extraArgs,
	CustomArgs:                customArgs,
	McpConfig:                 mcpConfig,
	ThinkingLevel:             thinkingLevel,
	OpenclawMode:              openclawMode,
}
```

to:

```go
execOpts := agent.ExecOptions{
	Cwd:                       env.WorkDir,
	Model:                     model,
	ThreadName:                deriveTaskThreadName(task),
	Timeout:                   d.cfg.AgentTimeout,
	SemanticInactivityTimeout: d.cfg.CodexSemanticInactivityTimeout,
	ResumeSessionID:           commentTaskPriorSession(task.TriggerCommentID, task.PriorSessionID),
	ExtraArgs:                 extraArgs,
	CustomArgs:                customArgs,
	McpConfig:                 mcpConfig,
	ThinkingLevel:             thinkingLevel,
	OpenclawMode:              openclawMode,
}
```

- [ ] **Step 4: Run the new test to confirm it passes**

```bash
cd server && go test ./internal/daemon/... -run TestCommentTaskDropsPriorSession -v
```

Expected: `PASS`

- [ ] **Step 5: Run the full daemon test suite**

```bash
cd server && go test ./internal/daemon/... -v 2>&1 | tail -40
```

Expected: all tests pass.

- [ ] **Step 6: Run the full Go test suite**

```bash
cd server && make test 2>&1 | tail -20
```

Expected: all tests pass, no vet errors.

- [ ] **Step 7: Commit**

```bash
git add server/internal/daemon/daemon.go \
        server/internal/daemon/daemon_test.go
git commit -m "fix(daemon): drop session resume for comment-triggered tasks

Each comment trigger re-reads issue context via the multica CLI, so
replaying prior session history adds no new facts. Over back-and-forth
comment cycles the accumulated history is the primary driver of high
input token consumption. Also eliminates the double-spend from the
failed-resume retry path on stale comment sessions."
```
