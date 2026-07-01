package execenv

import (
	"context"
	"sync/atomic"

	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// runtimeBriefSlimFlag is the feature-flag key that switches the runtime
// brief from the legacy verbose form (the canonical pre-MUL-3560 prompt that
// has shipped to production for ~2 years) to the post-MUL-3560 slim form
// (kind-driven dispatcher + per-section compression).
//
// Default is ON in every environment. Production uses the package default → slim.
// Ops can force the legacy brief per process with FF_RUNTIME_BRIEF_SLIM=false
// (EnvProvider) without a redeploy.
//
// Naming follows the docs/feature-flags.md convention `{team}_{area}_{behavior}`:
// `runtime` is the area (the agent runtime brief), `brief_slim` is the
// behavior toggle.
const runtimeBriefSlimFlag = "runtime_brief_slim"

// runtimeFlags is the package-scope feature flag service used by
// buildMetaSkillContent / BuildCommentReplyInstructions to pick between the
// legacy and slim brief paths. Stored behind an atomic.Pointer so the daemon
// can wire the service exactly once at startup (and tests can swap it under
// a t.Cleanup without races against parallel test goroutines).
//
// A nil service is valid: featureflag.Service is nil-safe and returns the
// caller's default (true → slim) when no provider is wired. That is what
// keeps every existing call site working even on a daemon that never bothered
// to call SetFeatureFlags.
var runtimeFlags atomic.Pointer[featureflag.Service]

// SetFeatureFlags wires the daemon's feature flag service into execenv. The
// daemon should call this once at startup right after constructing the
// service in cmd/server/main.go. Passing nil clears the wiring (every flag
// then falls back to its default), which is useful for tests.
func SetFeatureFlags(svc *featureflag.Service) {
	runtimeFlags.Store(svc)
}

// briefFlags is the unexported reader used by build-time toggle points. It
// returns the currently wired service, which may be nil — Service.IsEnabled
// handles that case (returns the caller's default).
func briefFlags() *featureflag.Service {
	return runtimeFlags.Load()
}

// useSlimBrief is the canonical toggle point for "should this run render the
// slim brief or the legacy brief". Always evaluated against a fresh
// background context (the brief is generated outside any HTTP request, so
// there is no per-request EvalContext to attach; per-workspace targeting
// must go through Rule.Allow / Rule.Deny on `workspace_id` or similar
// once we plumb workspace id through TaskContextForEnv — out of scope for
// the initial rollout).
//
// Default is `true` everywhere unless a Provider explicitly returns false.
// Production uses the package default → slim. Staging YAML can override.
// Ops can force legacy per process with `FF_RUNTIME_BRIEF_SLIM=false`
// (EnvProvider) without a redeploy.
func useSlimBrief() bool {
	return briefFlags().IsEnabled(context.Background(), runtimeBriefSlimFlag, true)
}
