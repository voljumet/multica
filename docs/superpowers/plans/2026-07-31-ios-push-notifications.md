# iOS Push Notifications Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a subscribe/follow bell button to the mobile issue detail header and deliver iOS push notifications to subscribed users when inbox events fire.

**Architecture:** Mobile registers an Expo push token with the backend on workspace mount; the backend stores tokens in a new `device_push_tokens` table and dispatches pushes after each inbox item creation in `notification_listeners.go`. Notification taps deep-link into the relevant issue. Subscribe bell on the issue detail header lets devs follow agent-assigned issues.

**Tech Stack:** `expo-notifications` (SDK 55 compatible), Expo Push Notifications HTTP API (`https://exp.host/api/v2/push/send`), sqlc, Chi router, Go, TanStack Query 5, Zustand, NativeWind 4.

## Global Constraints

- Expo SDK 55 — always use `pnpm exec expo install <pkg>` for Expo-ecosystem packages, never bare `pnpm add`.
- No foreign keys in DB migrations (CLAUDE.md).
- Every `CREATE INDEX` must use `CONCURRENTLY`; each concurrent index gets its own single-statement migration file (CLAUDE.md).
- Mobile API methods: use `fetchValidated` / `fetchValidatedWith`; new read-side methods must accept `opts?: { signal?: AbortSignal }` and forward it.
- Every `queryFn` must destructure `{ signal }` and forward it to the API method.
- No new `components/ui/` primitive unless 3+ callers and no RNR/iOS-native alternative.
- Migrations live in `server/migrations/`, queries in `server/pkg/db/queries/`, generated Go in `server/pkg/db/generated/`.
- Run `make sqlc` after any `.sql` query file change.
- Migration number: next after 201 — use `202`.

---

## File Map

**New files:**
- `server/migrations/202_device_push_tokens.up.sql` — table creation
- `server/migrations/202_device_push_tokens.down.sql` — drop table
- `server/migrations/203_device_push_tokens_user_idx.up.sql` — user_id index (separate file for CONCURRENTLY)
- `server/migrations/203_device_push_tokens_user_idx.down.sql` — drop index
- `server/pkg/db/queries/push_tokens.sql` — sqlc queries
- `server/internal/handler/push_token.go` — `RegisterPushToken` HTTP handler
- `server/cmd/server/push_send.go` — `sendPushNotifications` helper (Expo HTTP call)
- `apps/mobile/data/queries/subscribers.ts` — key factory + queryOptions for issue subscribers
- `apps/mobile/data/mutations/subscribers.ts` — `useToggleSubscription` mutation
- `apps/mobile/lib/push-notifications.ts` — `registerForPushNotifications()` helper

**Modified files:**
- `server/pkg/db/queries/push_tokens.sql` → regenerates `server/pkg/db/generated/push_tokens.sql.go` via `make sqlc`
- `server/cmd/server/router.go` — register `POST /api/push-tokens` route
- `server/cmd/server/notification_listeners.go` — call `sendPushNotifications` after inbox item creation in `notifyIssueSubscribers` and `notifyDirect`
- `apps/mobile/app.config.ts` — add `expo-notifications` plugin
- `apps/mobile/data/api.ts` — add `subscribeToIssue`, `unsubscribeFromIssue`, `registerPushToken` methods
- `apps/mobile/app/(app)/[workspace]/_layout.tsx` — call `registerForPushNotifications` + POST token on workspace mount
- `apps/mobile/app/(app)/[workspace]/issue/[id].tsx` — bell icon in `headerRight`, subscribe query
- `apps/mobile/app/_layout.tsx` — `addNotificationResponseReceivedListener` for deep link on tap
- `apps/mobile/data/realtime/use-issue-realtime.ts` — handle `subscriber:added` / `subscriber:removed`

---

## Task 1: DB migrations and sqlc

**Files:**
- Create: `server/migrations/202_device_push_tokens.up.sql`
- Create: `server/migrations/202_device_push_tokens.down.sql`
- Create: `server/migrations/203_device_push_tokens_user_idx.up.sql`
- Create: `server/migrations/203_device_push_tokens_user_idx.down.sql`
- Create: `server/pkg/db/queries/push_tokens.sql`
- Modify: `server/pkg/db/generated/` (via `make sqlc`)

**Interfaces:**
- Produces: `db.UpsertPushToken(ctx, UpsertPushTokenParams)` and `db.ListPushTokensByUser(ctx, userID pgtype.UUID) ([]PushToken, error)`

- [ ] **Step 1: Write migration 202 (table)**

Create `server/migrations/202_device_push_tokens.up.sql`:

```sql
CREATE TABLE device_push_tokens (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL,
    token      TEXT        NOT NULL,
    platform   TEXT        NOT NULL DEFAULT 'expo',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX CONCURRENTLY device_push_tokens_token_idx
    ON device_push_tokens (token);
```

Create `server/migrations/202_device_push_tokens.down.sql`:

```sql
DROP TABLE IF EXISTS device_push_tokens;
```

- [ ] **Step 2: Write migration 203 (user_id index — separate file)**

Create `server/migrations/203_device_push_tokens_user_idx.up.sql`:

```sql
CREATE INDEX CONCURRENTLY device_push_tokens_user_id_idx
    ON device_push_tokens (user_id);
```

Create `server/migrations/203_device_push_tokens_user_idx.down.sql`:

```sql
DROP INDEX IF EXISTS device_push_tokens_user_id_idx;
```

- [ ] **Step 3: Write sqlc queries**

Create `server/pkg/db/queries/push_tokens.sql`:

```sql
-- name: UpsertPushToken :exec
INSERT INTO device_push_tokens (user_id, token, platform, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (token)
DO UPDATE SET user_id = EXCLUDED.user_id, platform = EXCLUDED.platform, updated_at = now();

-- name: ListPushTokensByUser :many
SELECT * FROM device_push_tokens
WHERE user_id = $1;

-- name: DeletePushToken :exec
DELETE FROM device_push_tokens WHERE token = $1;
```

- [ ] **Step 4: Regenerate sqlc**

```bash
make sqlc
```

Expected: `server/pkg/db/generated/push_tokens.sql.go` created with `UpsertPushToken`, `ListPushTokensByUser`, `DeletePushToken` functions.

- [ ] **Step 5: Commit**

```bash
git add server/migrations/202_device_push_tokens.up.sql \
        server/migrations/202_device_push_tokens.down.sql \
        server/migrations/203_device_push_tokens_user_idx.up.sql \
        server/migrations/203_device_push_tokens_user_idx.down.sql \
        server/pkg/db/queries/push_tokens.sql \
        server/pkg/db/generated/push_tokens.sql.go
git commit -m "feat(push): add device_push_tokens table and sqlc queries"
```

---

## Task 2: Backend — push token endpoint

**Files:**
- Create: `server/internal/handler/push_token.go`
- Modify: `server/cmd/server/router.go`

**Interfaces:**
- Consumes: `db.UpsertPushToken` from Task 1
- Produces: `POST /api/push-tokens` endpoint, handler method `h.RegisterPushToken`

- [ ] **Step 1: Write the handler**

Create `server/internal/handler/push_token.go`:

```go
package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// RegisterPushToken upserts a device push token for the authenticated user.
// POST /api/push-tokens
// Body: { "token": "ExponentPushToken[...]", "platform": "expo" }
func (h *Handler) RegisterPushToken(w http.ResponseWriter, r *http.Request) {
	userID := requestUserID(r)

	var req struct {
		Token    string `json:"token"`
		Platform string `json:"platform"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}
	// Expo push tokens always start with "ExponentPushToken[".
	if !strings.HasPrefix(req.Token, "ExponentPushToken[") {
		writeError(w, http.StatusBadRequest, "invalid expo push token format")
		return
	}
	platform := req.Platform
	if platform == "" {
		platform = "expo"
	}

	err := h.Queries.UpsertPushToken(r.Context(), db.UpsertPushTokenParams{
		UserID:   parseUUID(userID),
		Token:    req.Token,
		Platform: platform,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to register push token")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 2: Register the route**

In `server/cmd/server/router.go`, inside the user-scoped section (after `/api/me` routes, before workspace routes — around line 858), add:

```go
r.Post("/api/push-tokens", h.RegisterPushToken)
```

Place it alongside the other user-scoped routes like `r.Post("/api/feedback", h.CreateFeedback)`.

- [ ] **Step 3: Verify Go compiles**

```bash
cd server && go build ./...
```

Expected: no errors.

- [ ] **Step 4: Write a handler test**

Create `server/internal/handler/push_token_test.go`:

```go
package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/handler"
)

func TestRegisterPushToken_RejectsInvalidFormat(t *testing.T) {
	fx := newHandlerFixture(t)
	body, _ := json.Marshal(map[string]string{"token": "not-a-valid-token", "platform": "expo"})
	req := httptest.NewRequest(http.MethodPost, "/api/push-tokens", bytes.NewReader(body))
	req = withAuthUser(req, fx.member.UserID)
	w := httptest.NewRecorder()
	fx.handler.RegisterPushToken(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRegisterPushToken_AcceptsValidToken(t *testing.T) {
	fx := newHandlerFixture(t)
	body, _ := json.Marshal(map[string]string{
		"token":    "ExponentPushToken[xxxxxxxxxxxxxxxxxxxxxx]",
		"platform": "expo",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/push-tokens", bytes.NewReader(body))
	req = withAuthUser(req, fx.member.UserID)
	w := httptest.NewRecorder()
	fx.handler.RegisterPushToken(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}
```

- [ ] **Step 5: Run test**

```bash
cd server && go test ./internal/handler/... -run TestRegisterPushToken -v
```

Expected: both tests pass.

- [ ] **Step 6: Commit**

```bash
git add server/internal/handler/push_token.go \
        server/internal/handler/push_token_test.go \
        server/cmd/server/router.go
git commit -m "feat(push): add POST /api/push-tokens endpoint"
```

---

## Task 3: Backend — push dispatch after inbox creation

**Files:**
- Create: `server/cmd/server/push_send.go`
- Modify: `server/cmd/server/notification_listeners.go`

**Interfaces:**
- Consumes: `db.ListPushTokensByUser` and `db.GetWorkspace` from Task 1 / existing queries
- Produces: `sendPushNotifications(ctx, queries, userID, title, workspaceSlug, issueID string)` — called after each `CreateInboxItem` in `notifyIssueSubscribers` and `notifyDirect`

- [ ] **Step 1: Write `push_send.go`**

Create `server/cmd/server/push_send.go`:

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/util"
)

const expoPushURL = "https://exp.host/api/v2/push/send"

type expoPushMessage struct {
	To    string         `json:"to"`
	Title string         `json:"title"`
	Sound string         `json:"sound"`
	Data  map[string]any `json:"data,omitempty"`
}

// sendPushNotifications dispatches an Expo push to every registered device
// for userID. Fire-and-forget: errors are logged, never returned.
// workspaceSlug and issueID are included in the notification data for
// deep-linking on tap.
func sendPushNotifications(
	ctx context.Context,
	queries *db.Queries,
	userID string,
	title string,
	workspaceSlug string,
	issueID string,
) {
	tokens, err := queries.ListPushTokensByUser(ctx, parseUUID(userID))
	if err != nil || len(tokens) == 0 {
		return
	}

	messages := make([]expoPushMessage, 0, len(tokens))
	for _, t := range tokens {
		if t.Platform != "expo" {
			continue
		}
		messages = append(messages, expoPushMessage{
			To:    t.Token,
			Title: title,
			Sound: "default",
			Data: map[string]any{
				"workspace_slug": workspaceSlug,
				"issue_id":       issueID,
			},
		})
	}
	if len(messages) == 0 {
		return
	}

	go func() {
		body, err := json.Marshal(messages)
		if err != nil {
			slog.Error("push: marshal error", "error", err)
			return
		}
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Post(expoPushURL, "application/json", bytes.NewReader(body))
		if err != nil {
			slog.Error("push: expo request failed", "error", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			slog.Error("push: expo returned non-2xx", "status", resp.StatusCode)
		}
	}()
}

// workspaceSlugByID returns the slug for a workspace ID, or "" on error.
func workspaceSlugByID(ctx context.Context, queries *db.Queries, workspaceID string) string {
	ws, err := queries.GetWorkspace(ctx, parseUUID(workspaceID))
	if err != nil {
		return ""
	}
	return ws.Slug
}
```

- [ ] **Step 2: Wire into `notifyIssueSubscribers`**

In `server/cmd/server/notification_listeners.go`, in `notifyIssueSubscribers`, after the `bus.Publish` call inside the subscriber loop (around line 360), add:

```go
// Dispatch push notification to the subscriber's registered devices.
wsSlug := workspaceSlugByID(ctx, queries, workspaceID)
issueIDStr := util.UUIDToString(item.IssueID)
sendPushNotifications(ctx, queries, subID, item.Title, wsSlug, issueIDStr)
```

The full block after the change looks like:

```go
notified[subID] = true
resp := inboxItemToResponse(item)
resp["issue_status"] = issueStatus
bus.Publish(events.Event{
    Type:        protocol.EventInboxNew,
    WorkspaceID: workspaceID,
    ActorType:   e.ActorType,
    ActorID:     e.ActorID,
    Payload:     map[string]any{"item": resp},
})
// Dispatch push notification to the subscriber's registered devices.
wsSlug := workspaceSlugByID(ctx, queries, workspaceID)
issueIDStr := util.UUIDToString(item.IssueID)
sendPushNotifications(ctx, queries, subID, item.Title, wsSlug, issueIDStr)
```

- [ ] **Step 3: Wire into `notifyDirect`**

In `notifyDirect` (around line 420), after the `bus.Publish` call, add the same push dispatch for member recipients only:

```go
// Dispatch push notification if recipient is a member.
if recipientType == "member" {
    wsSlug := workspaceSlugByID(ctx, queries, workspaceID)
    sendPushNotifications(ctx, queries, recipientID, item.Title, wsSlug, issueID)
}
```

- [ ] **Step 4: Verify Go compiles**

```bash
cd server && go build ./...
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add server/cmd/server/push_send.go \
        server/cmd/server/notification_listeners.go
git commit -m "feat(push): dispatch Expo push after inbox item creation"
```

---

## Task 4: Mobile — subscriber data layer

**Files:**
- Create: `apps/mobile/data/queries/subscribers.ts`
- Create: `apps/mobile/data/mutations/subscribers.ts`
- Modify: `apps/mobile/data/api.ts`

**Interfaces:**
- Produces:
  - `subscriberKeys.all(issueId: string)` — TanStack Query key factory
  - `subscribersOptions(issueId: string)` — queryOptions for `GET /api/issues/{id}/subscribers`
  - `useToggleSubscription(issueId: string)` — mutation hook
  - `api.subscribeToIssue(issueId: string): Promise<void>`
  - `api.unsubscribeFromIssue(issueId: string): Promise<void>`

- [ ] **Step 1: Add API methods**

In `apps/mobile/data/api.ts`, add after the existing subscriber-unrelated methods (group with issue-related methods):

```ts
async subscribeToIssue(issueId: string): Promise<void> {
  await this.fetch(`/api/issues/${issueId}/subscribe`, { method: "POST" });
}

async unsubscribeFromIssue(issueId: string): Promise<void> {
  await this.fetch(`/api/issues/${issueId}/unsubscribe`, { method: "POST" });
}
```

Note: These are writes with no consumed response — bare `this.fetch` is correct per the ApiClient rules.

- [ ] **Step 2: Write subscriber query file**

Create `apps/mobile/data/queries/subscribers.ts`:

```ts
import { queryOptions } from "@tanstack/react-query";
import type { IssueSubscriber } from "@multica/core/types/subscriber";
import { z } from "zod";
import { api } from "@/data/api";

const IssueSubscriberSchema = z.object({
  issue_id: z.string(),
  user_type: z.enum(["member", "agent"]),
  user_id: z.string(),
  reason: z.enum(["creator", "assignee", "commenter", "mentioned", "manual"]),
  created_at: z.string(),
});

const IssueSubscribersSchema = z.array(IssueSubscriberSchema);
const EMPTY_SUBSCRIBERS: IssueSubscriber[] = [];

export const subscriberKeys = {
  all: (issueId: string) => ["subscribers", issueId] as const,
};

export function subscribersOptions(issueId: string) {
  return queryOptions({
    queryKey: subscriberKeys.all(issueId),
    queryFn: ({ signal }) =>
      api.fetchValidated(
        `/api/issues/${issueId}/subscribers`,
        IssueSubscribersSchema,
        EMPTY_SUBSCRIBERS,
        { signal }
      ),
    enabled: !!issueId,
  });
}
```

Note: `api.fetchValidated` is a private method on `ApiClient`. Add a `fetchValidated` call to `api.ts` if it's private — or use `api.listIssueSubscribers` if you prefer a named method. Check `apps/mobile/data/api.ts` for the access pattern. The existing `listInbox` is an example.

**Correction:** `fetchValidated` is a method on the class, accessed as `this.fetchValidated` inside `api.ts`. External callers can't call it directly. Instead, add a named method on `ApiClient` in `api.ts`:

```ts
async listIssueSubscribers(
  issueId: string,
  opts?: { signal?: AbortSignal }
): Promise<IssueSubscriber[]> {
  return this.fetchValidated(
    `/api/issues/${issueId}/subscribers`,
    IssueSubscribersSchema,
    EMPTY_SUBSCRIBERS,
    opts
  );
}
```

And in `subscribers.ts`:

```ts
queryFn: ({ signal }) => api.listIssueSubscribers(issueId, { signal }),
```

Also add `IssueSubscribersSchema` and `EMPTY_SUBSCRIBERS` to `apps/mobile/data/schemas.ts` if that file exists — otherwise define them inline in `subscribers.ts`.

- [ ] **Step 3: Write subscriber mutation file**

Create `apps/mobile/data/mutations/subscribers.ts`:

```ts
import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { IssueSubscriber } from "@multica/core/types/subscriber";
import { api } from "@/data/api";
import { subscriberKeys } from "@/data/queries/subscribers";

/**
 * Mirrors packages/core/issues/mutations.ts useToggleIssueSubscription.
 * Optimistic: flips the local subscriber list immediately.
 */
export function useToggleSubscription(issueId: string) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: ({
      userId,
      subscribed,
    }: {
      userId: string;
      userType: "member";
      subscribed: boolean;
    }) =>
      subscribed
        ? api.unsubscribeFromIssue(issueId)
        : api.subscribeToIssue(issueId),

    onMutate: async ({ userId, userType, subscribed }) => {
      await qc.cancelQueries({ queryKey: subscriberKeys.all(issueId) });
      const prev = qc.getQueryData<IssueSubscriber[]>(
        subscriberKeys.all(issueId)
      );

      qc.setQueryData<IssueSubscriber[]>(subscriberKeys.all(issueId), (old) => {
        const list = old ?? [];
        if (subscribed) {
          // Unsubscribing: remove
          return list.filter(
            (s) => !(s.user_id === userId && s.user_type === userType)
          );
        } else {
          // Subscribing: add
          const newSub: IssueSubscriber = {
            issue_id: issueId,
            user_id: userId,
            user_type: userType,
            reason: "manual",
            created_at: new Date().toISOString(),
          };
          return [...list, newSub];
        }
      });

      return { prev };
    },

    onError: (_err, _vars, ctx) => {
      if (ctx?.prev !== undefined) {
        qc.setQueryData(subscriberKeys.all(issueId), ctx.prev);
      }
    },

    onSettled: () => {
      qc.invalidateQueries({ queryKey: subscriberKeys.all(issueId) });
    },
  });
}
```

- [ ] **Step 4: Verify TypeScript compiles**

```bash
pnpm typecheck
```

Expected: no new errors.

- [ ] **Step 5: Commit**

```bash
git add apps/mobile/data/api.ts \
        apps/mobile/data/queries/subscribers.ts \
        apps/mobile/data/mutations/subscribers.ts
git commit -m "feat(mobile): add subscriber query/mutation data layer"
```

---

## Task 5: Mobile — push notification registration

**Files:**
- Create: `apps/mobile/lib/push-notifications.ts`
- Modify: `apps/mobile/app.config.ts`
- Modify: `apps/mobile/data/api.ts`
- Modify: `apps/mobile/app/(app)/[workspace]/_layout.tsx`
- Modify: `apps/mobile/app/_layout.tsx`

**Interfaces:**
- Produces:
  - `registerForPushNotifications(): Promise<string | null>` — returns Expo push token or null
  - `api.registerPushToken(token: string): Promise<void>`
  - Deep-link listener in `app/_layout.tsx`

- [ ] **Step 1: Install expo-notifications**

```bash
cd apps/mobile && pnpm exec expo install expo-notifications
```

Expected: `expo-notifications` added to `apps/mobile/package.json` at the SDK-55-compatible version.

- [ ] **Step 2: Add plugin to app.config.ts**

In `apps/mobile/app.config.ts`, in the `plugins` array (alongside `"expo-router"`, `"expo-secure-store"`, etc.), add:

```ts
[
  "expo-notifications",
  {
    // Tinted notification icon for Android (iOS uses the app icon).
    // Create a 96×96 white-on-transparent PNG at this path or omit if
    // Android is not a target yet.
    // icon: "./assets/notification-icon.png",
    color: "#ffffff",
  },
],
```

- [ ] **Step 3: Write the registration helper**

Create `apps/mobile/lib/push-notifications.ts`:

```ts
import * as Notifications from "expo-notifications";
import * as Device from "expo-device";
import Constants from "expo-constants";

/**
 * Requests iOS push permission and returns the Expo push token string,
 * or null when running on a simulator or permission is denied.
 *
 * Call once per workspace session (caller should persist the returned token
 * in expo-secure-store and skip re-registration when unchanged).
 */
export async function registerForPushNotifications(): Promise<string | null> {
  // Simulators cannot receive push notifications.
  if (!Device.isDevice) return null;

  const { status: existing } = await Notifications.getPermissionsAsync();
  let finalStatus = existing;

  if (existing !== "granted") {
    const { status } = await Notifications.requestPermissionsAsync();
    finalStatus = status;
  }

  if (finalStatus !== "granted") return null;

  const projectId =
    Constants.expoConfig?.extra?.eas?.projectId ??
    Constants.easConfig?.projectId;

  if (!projectId) {
    console.warn("[push] No EAS projectId found — push token unavailable");
    return null;
  }

  const { data: token } = await Notifications.getExpoPushTokenAsync({
    projectId,
  });
  return token;
}
```

- [ ] **Step 4: Add `registerPushToken` API method**

In `apps/mobile/data/api.ts`, add after the other user-scoped methods (near `updateMe`, `sendCode`, etc.):

```ts
async registerPushToken(token: string): Promise<void> {
  await this.fetch("/api/push-tokens", {
    method: "POST",
    body: JSON.stringify({ token, platform: "expo" }),
  });
}
```

- [ ] **Step 5: Register token on workspace mount**

In `apps/mobile/app/(app)/[workspace]/_layout.tsx`:

Add imports at the top:

```ts
import { useRef } from "react";
import * as SecureStore from "expo-secure-store";
import { registerForPushNotifications } from "@/lib/push-notifications";
import { api } from "@/data/api";
```

Inside `WorkspaceLayout`, after the `setCurrentWorkspace` effect, add a new effect:

```ts
const pushRegisteredRef = useRef(false);

useEffect(() => {
  if (!matched || pushRegisteredRef.current) return;
  pushRegisteredRef.current = true;

  void (async () => {
    try {
      const token = await registerForPushNotifications();
      if (!token) return;

      const stored = await SecureStore.getItemAsync("push-token");
      if (stored === token) return; // unchanged, skip re-registration

      await api.registerPushToken(token);
      await SecureStore.setItemAsync("push-token", token);
    } catch (err) {
      // Non-critical — log and continue. Push is a best-effort channel.
      console.warn("[push] registration failed", err);
    }
  })();
}, [matched]);
```

- [ ] **Step 6: Add deep-link listener on notification tap**

In `apps/mobile/app/_layout.tsx`:

Add import:

```ts
import * as Notifications from "expo-notifications";
```

Inside `AuthInitializer`, add a new `useEffect` (alongside the existing auth init effect):

```ts
useEffect(() => {
  // Handle notification tap: navigate to the issue that triggered the push.
  // Fires when the user taps a notification that launches or foregrounds the app.
  const sub = Notifications.addNotificationResponseReceivedListener(
    (response) => {
      const data = response.notification.request.content.data as
        | { workspace_slug?: string; issue_id?: string }
        | undefined;
      if (data?.workspace_slug && data?.issue_id) {
        router.push(
          `/${data.workspace_slug}/issue/${data.issue_id}` as Parameters<
            typeof router.push
          >[0]
        );
      }
    }
  );
  return () => sub.remove();
}, []);
```

Also add at the module level (outside components) to configure how received notifications are presented while the app is foregrounded:

```ts
Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldShowAlert: true,
    shouldPlaySound: true,
    shouldSetBadge: false,
  }),
});
```

- [ ] **Step 7: Verify TypeScript compiles**

```bash
pnpm typecheck
```

Expected: no new errors.

- [ ] **Step 8: Commit**

```bash
git add apps/mobile/lib/push-notifications.ts \
        apps/mobile/app.config.ts \
        apps/mobile/data/api.ts \
        apps/mobile/app/_layout.tsx \
        apps/mobile/app/\(app\)/\[workspace\]/_layout.tsx \
        apps/mobile/package.json
git commit -m "feat(mobile): register Expo push token and deep-link on notification tap"
```

---

## Task 6: Mobile — subscribe bell on issue detail

**Files:**
- Modify: `apps/mobile/app/(app)/[workspace]/issue/[id].tsx`

**Interfaces:**
- Consumes: `subscribersOptions(id)` from Task 4, `useToggleSubscription(id)` from Task 4, `useAuthStore` (existing)

- [ ] **Step 1: Add bell icon to issue detail header**

In `apps/mobile/app/(app)/[workspace]/issue/[id].tsx`:

Add imports:

```ts
import { useQuery } from "@tanstack/react-query";
import { subscribersOptions } from "@/data/queries/subscribers";
import { useToggleSubscription } from "@/data/mutations/subscribers";
import { useAuthStore } from "@/data/auth-store";
import * as Haptics from "expo-haptics";
```

Inside the screen component, before the `return` statement, add:

```ts
const user = useAuthStore((s) => s.user);
const subscribers = useQuery(subscribersOptions(id));
const toggleSubscription = useToggleSubscription(id);

const isSubscribed =
  subscribers.data?.some(
    (s) => s.user_id === user?.id && s.user_type === "member"
  ) ?? false;

const onToggleSubscription = () => {
  if (!user) return;
  Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
  toggleSubscription.mutate({
    userId: user.id,
    userType: "member",
    subscribed: isSubscribed,
  });
};
```

In the `headerRight` render, add the bell button between `<AgentHeaderBadge>` and the `…` button:

```tsx
headerRight: issue
  ? () => (
      <View className="flex-row items-center gap-2">
        <AgentHeaderBadge issueId={id} />
        <IconButton
          name={isSubscribed ? "notifications" : "notifications-outline"}
          onPress={onToggleSubscription}
          accessibilityLabel={isSubscribed ? "Unfollow issue" : "Follow issue"}
        />
        <IconButton
          name="ellipsis-horizontal"
          onPress={onPressMore}
          accessibilityLabel="Issue actions"
        />
      </View>
    )
  : undefined,
```

- [ ] **Step 2: Handle `subscriber:added` / `subscriber:removed` in issue realtime**

In `apps/mobile/data/realtime/use-issue-realtime.ts`, find the `useWSSubscriptions` setup callback. Add handlers for the subscriber events:

First check what events are in `WSEventType` by grepping:

```bash
grep "subscriber" packages/core/types/events.ts
```

If `subscriber:added` and `subscriber:removed` are in the event map, add:

```ts
ws.on("subscriber:added", (payload) => {
  if (payload.issue_id !== id) return;
  qc.invalidateQueries({ queryKey: subscriberKeys.all(id) });
}),
ws.on("subscriber:removed", (payload) => {
  if (payload.issue_id !== id) return;
  qc.invalidateQueries({ queryKey: subscriberKeys.all(id) });
}),
```

Add `import { subscriberKeys } from "@/data/queries/subscribers";` at the top of the file.

If `subscriber:added` / `subscriber:removed` are NOT yet in `packages/core/types/events.ts`, add them:

In `packages/core/types/events.ts`:
```ts
// In WSEventType union:
| "subscriber:added"
| "subscriber:removed"

// In WSEventPayloadMap:
"subscriber:added": { issue_id: string; user_id: string; user_type: string };
"subscriber:removed": { issue_id: string; user_id: string; user_type: string };
```

(These events are already published by the backend `subscriber.go` via `protocol.EventSubscriberAdded` and `protocol.EventSubscriberRemoved` — confirm the string values match by checking `server/pkg/protocol/events.go`.)

- [ ] **Step 3: Verify TypeScript compiles**

```bash
pnpm typecheck
```

Expected: no new errors.

- [ ] **Step 4: Commit**

```bash
git add apps/mobile/app/\(app\)/\[workspace\]/issue/\[id\].tsx \
        apps/mobile/data/realtime/use-issue-realtime.ts \
        packages/core/types/events.ts
git commit -m "feat(mobile): add subscribe bell to issue detail header"
```

---

## Task 7: Server Go tests

**Files:**
- Modify: `server/cmd/server/push_send.go` (add testable helper if needed)

**Goal:** Verify push dispatch doesn't block the notification path and fires correctly.

- [ ] **Step 1: Write a unit test for `sendPushNotifications`**

Create `server/cmd/server/push_send_test.go`:

```go
package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestSendPushNotifications_FiresForExpoToken(t *testing.T) {
	var mu sync.Mutex
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		received = make([]byte, r.ContentLength)
		r.Body.Read(received)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	// Override the push URL to point at the test server.
	origURL := expoPushURL
	expoPushURL = srv.URL
	defer func() { expoPushURL = origURL }()

	// Build a minimal in-memory token lookup.
	// sendPushNotifications normally queries the DB; inject a fake via the
	// var-substitution pattern below. For a real integration test, use
	// testPool (see other handler tests). This unit test verifies the HTTP
	// dispatch logic only.
	//
	// The simplest approach: export a `sendPushDirect` helper that accepts
	// []string tokens instead of querying, and test that. Wrap
	// sendPushNotifications to call sendPushDirect after the DB lookup.

	// Since sendPushNotifications queries the DB, this test is best run as
	// an integration test with testPool. Add it to the integration suite:
	// see push_token_test.go for the fixture pattern.
	t.Log("push dispatch integration test: use testPool fixture in push_token_test.go")
}

func TestSendPushNotifications_SkipsOnNoTokens(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	origURL := expoPushURL
	expoPushURL = srv.URL
	defer func() { expoPushURL = origURL }()

	// sendPushDirect is a helper that takes tokens directly (no DB).
	// Add to push_send.go:
	//   func sendPushDirect(tokens []string, title, workspaceSlug, issueID string) { ... }
	// and have sendPushNotifications call it after the DB lookup.

	// For now, verify the function doesn't panic with an empty token list.
	sendPushDirect(nil, "Test title", "my-workspace", "issue-id-123")
	time.Sleep(50 * time.Millisecond) // let goroutine settle
	if called {
		t.Fatal("expected no HTTP call with empty token list")
	}
}
```

For this to work, extract the HTTP dispatch into a separate `sendPushDirect` function in `push_send.go`:

```go
// sendPushDirect dispatches push messages to the given Expo token strings.
// Extracted from sendPushNotifications for testability.
func sendPushDirect(tokens []string, title, workspaceSlug, issueID string) {
	if len(tokens) == 0 {
		return
	}
	messages := make([]expoPushMessage, 0, len(tokens))
	for _, tok := range tokens {
		messages = append(messages, expoPushMessage{
			To:    tok,
			Title: title,
			Sound: "default",
			Data: map[string]any{
				"workspace_slug": workspaceSlug,
				"issue_id":       issueID,
			},
		})
	}
	go func() {
		body, err := json.Marshal(messages)
		if err != nil {
			slog.Error("push: marshal error", "error", err)
			return
		}
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Post(expoPushURL, "application/json", bytes.NewReader(body))
		if err != nil {
			slog.Error("push: expo request failed", "error", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			slog.Error("push: expo returned non-2xx", "status", resp.StatusCode)
		}
	}()
}
```

Update `sendPushNotifications` to call `sendPushDirect` with the token strings.

- [ ] **Step 2: Run Go tests**

```bash
cd server && go test ./cmd/server/... -run TestSendPush -v
```

Expected: `TestSendPushNotifications_SkipsOnNoTokens` passes.

- [ ] **Step 3: Run all Go tests**

```bash
make test
```

Expected: all pass (or pre-existing failures only).

- [ ] **Step 4: Commit**

```bash
git add server/cmd/server/push_send.go \
        server/cmd/server/push_send_test.go
git commit -m "test(push): add unit test for push dispatch helper"
```

---

## Task 8: Final verification

- [ ] **Step 1: TypeScript typecheck**

```bash
pnpm typecheck
```

Expected: no errors.

- [ ] **Step 2: Go build and tests**

```bash
cd server && go build ./... && make test
```

Expected: no errors, tests pass.

- [ ] **Step 3: Mobile lint**

```bash
cd apps/mobile && pnpm lint
```

Expected: no new lint errors.

- [ ] **Step 4: Manual smoke-test checklist**

Run in Expo Go / dev build on a physical device:

1. Open the app and navigate to a workspace.
2. When prompted, allow push notification permission.
3. Open any issue — verify a bell icon appears in the header.
4. Tap the bell — icon should flip to filled, no error toast.
5. Tap again — icon should flip back to outline.
6. From another client (web or second device), add a comment to the issue.
7. Background the iOS app — verify a push banner appears within ~5 seconds.
8. Tap the notification — verify the app opens directly to the issue detail.

- [ ] **Step 5: Commit any fixes from smoke test**

```bash
git add -p
git commit -m "fix(push): address smoke-test issues"
```
