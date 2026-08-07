# Notification Workspace Context & Mute Action — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show workspace name in OS notifications, and let users mute an issue's notifications via a long-press action (iOS) or action button (macOS).

**Architecture:** Four independent tasks, each deployable alone. Server changes gate Task 4 (mobile category registration is inert without the server sending `category`), but all four can be reviewed in parallel. No new abstractions — the changes slot into existing extension points.

**Tech Stack:** Go (server push), TypeScript / Electron (desktop), TypeScript / Expo (mobile), TanStack Query (cache reads).

## Global Constraints

- Mobile target: iOS. Android falls back gracefully (action omitted, workspace name still shows).
- Workspace name in the notification body, not the title — title is the notification subject line.
- Expo SDK 55, React Native 0.82, React 19.1 — do not upgrade anything.
- No new npm/Go dependencies.
- Code comments in English only.
- Go: `gofmt` + checked errors. TypeScript: strict mode, no `as T` on network JSON.

---

### Task 1: Server — workspace name in push body + iOS category identifier

**Files:**
- Modify: `server/cmd/server/push_send.go`
- Modify: `server/cmd/server/notification_listeners.go`
- Test: `server/cmd/server/push_send_test.go`

**Interfaces:**
- Produces: `sendPushDirect(tokens []string, title, workspaceSlug, issueID, workspaceName string)` — callers in `notification_listeners.go` pass workspace name; the Expo payload gains `Body` (workspace name) and `Data["category"] = "issue_notification"`.

---

- [ ] **Step 1.1: Write the failing test for body + category fields**

In `server/cmd/server/push_send_test.go`, replace `TestSendPushDirect_FiresForExpoToken` with a version that also asserts the body and category are present. Also update the `_SkipsOnNoTokens` call signature.

```go
func TestSendPushDirect_SkipsOnNoTokens(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	origURL := expoPushURL
	expoPushURL = srv.URL
	defer func() { expoPushURL = origURL }()

	sendPushDirect(nil, "Test title", "my-workspace", "issue-id-123", "My Workspace")
	time.Sleep(100 * time.Millisecond)

	if called {
		t.Fatal("expected no HTTP call with empty token list")
	}
}

func TestSendPushDirect_FiresForExpoToken(t *testing.T) {
	var mu sync.Mutex
	var received []byte
	done := make(chan struct{})
	var once sync.Once

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received = make([]byte, r.ContentLength)
		r.Body.Read(received) //nolint:errcheck
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[]}`)) //nolint:errcheck

		once.Do(func() { close(done) })
	}))
	defer srv.Close()

	origURL := expoPushURL
	expoPushURL = srv.URL
	defer func() { expoPushURL = origURL }()

	sendPushDirect([]string{"ExponentPushToken[abc123]"}, "New comment", "my-workspace", "issue-id-456", "My Workspace")

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for push HTTP request")
	}

	mu.Lock()
	body := string(received)
	mu.Unlock()

	if body == "" {
		t.Fatal("expected non-empty request body")
	}

	var msgs []struct {
		Title string         `json:"title"`
		Body  string         `json:"body"`
		Data  map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &msgs); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("expected at least one push message")
	}
	msg := msgs[0]
	if msg.Title != "New comment" {
		t.Errorf("expected title %q, got %q", "New comment", msg.Title)
	}
	if msg.Body != "My Workspace" {
		t.Errorf("expected body %q, got %q", "My Workspace", msg.Body)
	}
	cat, _ := msg.Data["category"].(string)
	if cat != "issue_notification" {
		t.Errorf("expected data.category %q, got %q", "issue_notification", cat)
	}
	wsSlug, _ := msg.Data["workspace_slug"].(string)
	if wsSlug != "my-workspace" {
		t.Errorf("expected data.workspace_slug %q, got %q", "my-workspace", wsSlug)
	}
}
```

- [ ] **Step 1.2: Run the test to confirm it fails**

```bash
cd server && go test ./cmd/server/ -run TestSendPushDirect -v
```

Expected: compile error (wrong arg count to `sendPushDirect`) or test failure on the assertions.

- [ ] **Step 1.3: Update `push_send.go`**

Replace the entire file content with:

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
)

var expoPushURL = "https://exp.host/--/api/v2/push/send"

type expoPushMessage struct {
	To    string         `json:"to"`
	Title string         `json:"title"`
	Body  string         `json:"body,omitempty"`
	Sound string         `json:"sound"`
	Data  map[string]any `json:"data,omitempty"`
}

func sendPushDirect(tokens []string, title, workspaceSlug, issueID, workspaceName string) {
	if len(tokens) == 0 {
		return
	}

	messages := make([]expoPushMessage, 0, len(tokens))
	for _, tok := range tokens {
		messages = append(messages, expoPushMessage{
			To:    tok,
			Title: title,
			Body:  workspaceName,
			Sound: "default",
			Data: map[string]any{
				"workspace_slug": workspaceSlug,
				"issue_id":       issueID,
				"category":       "issue_notification",
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
			return
		}
		var result struct {
			Data []struct {
				Status  string `json:"status"`
				Message string `json:"message"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return
		}
		for _, ticket := range result.Data {
			if ticket.Status != "ok" {
				slog.Error("push: expo ticket error", "status", ticket.Status, "message", ticket.Message)
			}
		}
	}()
}

func sendPushNotifications(
	ctx context.Context,
	queries *db.Queries,
	userID string,
	title string,
	workspaceSlug string,
	issueID string,
	workspaceName string,
) {
	tokens, err := queries.ListPushTokensByUser(ctx, parseUUID(userID))
	if err != nil || len(tokens) == 0 {
		return
	}

	var tokenStrings []string
	for _, t := range tokens {
		if t.Platform != "expo" {
			continue
		}
		tokenStrings = append(tokenStrings, t.Token)
	}

	sendPushDirect(tokenStrings, title, workspaceSlug, issueID, workspaceName)
}

func workspaceSlugByID(ctx context.Context, queries *db.Queries, workspaceID string) string {
	ws, err := queries.GetWorkspace(ctx, parseUUID(workspaceID))
	if err != nil {
		return ""
	}
	return ws.Slug
}

func workspaceNameByID(ctx context.Context, queries *db.Queries, workspaceID string) string {
	ws, err := queries.GetWorkspace(ctx, parseUUID(workspaceID))
	if err != nil {
		return ""
	}
	return ws.Name
}
```

- [ ] **Step 1.4: Update `notification_listeners.go` call sites**

Find the two `sendPushNotifications` call sites and add the workspace name argument.

**Call site 1** — in `notifySubscribers` (around line 310). Add `wsName` computation alongside the existing `wsSlug`:

```go
// existing:
wsSlug := workspaceSlugByID(ctx, queries, workspaceID)
// add:
wsName := workspaceNameByID(ctx, queries, workspaceID)
```

Then update the `sendPushNotifications` call at line ~366:

```go
// before:
sendPushNotifications(ctx, queries, subID, item.Title, wsSlug, issueIDStr)
// after:
sendPushNotifications(ctx, queries, subID, item.Title, wsSlug, issueIDStr, wsName)
```

**Call site 2** — in `notifyDirect` (around line 433). Add `wsName` alongside `wsSlug`:

```go
// existing:
wsSlug := workspaceSlugByID(ctx, queries, workspaceID)
// add:
wsName := workspaceNameByID(ctx, queries, workspaceID)
```

Update the `sendPushNotifications` call:

```go
// before:
sendPushNotifications(ctx, queries, recipientID, item.Title, wsSlug, issueID)
// after:
sendPushNotifications(ctx, queries, recipientID, item.Title, wsSlug, issueID, wsName)
```

- [ ] **Step 1.5: Run tests and confirm they pass**

```bash
cd server && go test ./cmd/server/ -run TestSendPushDirect -v
make test
```

Expected: all tests pass.

- [ ] **Step 1.6: Commit**

```bash
git add server/cmd/server/push_send.go server/cmd/server/notification_listeners.go server/cmd/server/push_send_test.go
git commit -m "feat(push): add workspace name to push body and iOS category identifier"
```

---

### Task 2: Desktop — workspace name in notification banner

**Files:**
- Modify: `packages/core/platform/system-notification.ts`
- Modify: `packages/core/realtime/use-realtime-sync.ts`
- Modify: `apps/desktop/src/main/notification-gate.ts`
- Modify: `apps/desktop/src/main/index.ts`
- Test: `apps/desktop/src/main/notification-gate.test.ts`

**Interfaces:**
- Consumes: `workspaceKeys.list()` from TanStack Query cache (already imported in `use-realtime-sync.ts`).
- Produces: `SystemNotificationPayload.workspaceName?: string` — optional string, max 256 chars. `NativeNotificationPayload.workspaceName?: string` — same. Desktop notification body becomes `[workspaceName, body].filter(Boolean).join('\n')`.

---

- [ ] **Step 2.1: Write failing tests for `parseNativeNotificationPayload`**

Open `apps/desktop/src/main/notification-gate.test.ts`. Add these test cases inside the existing `describe("parseNativeNotificationPayload", ...)` block:

```typescript
it("accepts a valid payload with workspaceName", () => {
  const result = parseNativeNotificationPayload({
    slug: "acme",
    itemId: "item-1",
    issueKey: "issue-1",
    title: "New comment",
    body: "",
    workspaceName: "Acme Corp",
  });
  expect(result?.workspaceName).toBe("Acme Corp");
});

it("accepts a payload without workspaceName (backward compat)", () => {
  const result = parseNativeNotificationPayload({
    slug: "acme",
    itemId: "item-1",
    issueKey: "issue-1",
    title: "New comment",
    body: "",
  });
  expect(result).not.toBeNull();
  expect(result?.workspaceName).toBeUndefined();
});

it("ignores workspaceName that is too long", () => {
  const result = parseNativeNotificationPayload({
    slug: "acme",
    itemId: "item-1",
    issueKey: "issue-1",
    title: "New comment",
    body: "",
    workspaceName: "x".repeat(257),
  });
  // payload is valid, workspaceName is simply dropped
  expect(result).not.toBeNull();
  expect(result?.workspaceName).toBeUndefined();
});
```

- [ ] **Step 2.2: Run tests to confirm they fail**

```bash
cd apps/desktop && pnpm test src/main/notification-gate.test.ts
```

Expected: tests fail (property `workspaceName` does not exist on type).

- [ ] **Step 2.3: Add `workspaceName` to `SystemNotificationPayload`**

In `packages/core/platform/system-notification.ts`, add the optional field to the interface:

```typescript
export interface SystemNotificationPayload {
  slug: string;
  itemId: string;
  issueKey: string;
  title: string;
  body: string;
  /** Source workspace display name, shown in the notification body. */
  workspaceName?: string;
}
```

- [ ] **Step 2.4: Add `workspaceName` to `NativeNotificationPayload` and update parser**

In `apps/desktop/src/main/notification-gate.ts`, update the interface and parser:

```typescript
export interface NativeNotificationPayload {
  slug: string;
  itemId: string;
  issueKey: string;
  title: string;
  body: string;
  workspaceName?: string;
}

export function parseNativeNotificationPayload(
  value: unknown,
): NativeNotificationPayload | null {
  if (!value || typeof value !== "object") return null;
  const candidate = value as Record<string, unknown>;
  const limits: Record<
    Exclude<keyof NativeNotificationPayload, "workspaceName">,
    number
  > = {
    slug: 256,
    itemId: 256,
    issueKey: 256,
    title: 512,
    body: 2_000,
  };
  const result = {} as NativeNotificationPayload;
  for (const key of Object.keys(limits) as (keyof typeof limits)[]) {
    const field = candidate[key];
    const mayBeEmpty = key === "slug" || key === "body";
    if (
      typeof field !== "string" ||
      (!mayBeEmpty && !field.trim()) ||
      field.length > limits[key]
    ) {
      return null;
    }
    result[key] = field;
  }
  // workspaceName is optional — accept valid strings up to 256 chars, silently
  // drop anything else so old desktop builds keep working without this field.
  const wn = candidate["workspaceName"];
  if (typeof wn === "string" && wn.length <= 256) {
    result.workspaceName = wn;
  }
  return result;
}
```

- [ ] **Step 2.5: Run parser tests — confirm they pass**

```bash
cd apps/desktop && pnpm test src/main/notification-gate.test.ts
```

Expected: all pass.

- [ ] **Step 2.6: Populate `workspaceName` in `handleInboxNew`**

In `packages/core/realtime/use-realtime-sync.ts`, find where `payload` is constructed (around line 454). Before that block, resolve the workspace name from the already-warm workspace list cache:

```typescript
// Existing imports already include workspaceKeys — no new import needed.
// Resolve workspace display name for the notification body.
const workspaceList = qc.getQueryData<Workspace[]>(workspaceKeys.list());
const workspaceName =
  workspaceList?.find((w) => w.id === sourceWsId)?.name ?? undefined;

const payload: SystemNotificationPayload = {
  slug: slug ?? "",
  itemId: item.id,
  issueKey: item.issue_id ?? item.id,
  title: item.title,
  body: item.body ?? "",
  workspaceName,
};
```

Confirm `Workspace` is already imported in this file (it is, via the workspace query types).

- [ ] **Step 2.7: Prepend workspace name to the Electron notification body**

In `apps/desktop/src/main/index.ts`, find where the `Notification` is constructed (around line 817):

```typescript
// before:
const notification = new Notification({
  title: payload.title,
  body: payload.body,
});

// after:
const notificationBody = [payload.workspaceName, payload.body]
  .filter(Boolean)
  .join("\n");
const notification = new Notification({
  title: payload.title,
  body: notificationBody,
});
```

- [ ] **Step 2.8: Typecheck + run desktop tests**

```bash
pnpm typecheck
cd apps/desktop && pnpm test
```

Expected: no type errors, all tests pass.

- [ ] **Step 2.9: Commit**

```bash
git add packages/core/platform/system-notification.ts \
        packages/core/realtime/use-realtime-sync.ts \
        apps/desktop/src/main/notification-gate.ts \
        apps/desktop/src/main/index.ts \
        apps/desktop/src/main/notification-gate.test.ts
git commit -m "feat(desktop): show workspace name in notification banner body"
```

---

### Task 3: Desktop — "Turn Off Notifications" action on macOS notification

**Files:**
- Modify: `apps/desktop/src/shared/main-renderer-messages.ts`
- Modify: `apps/desktop/src/preload/index.ts`
- Modify: `apps/desktop/src/main/index.ts`
- Modify: `packages/core/api/client.ts`
- Modify: `apps/desktop/src/renderer/src/components/desktop-layout.tsx`

**Interfaces:**
- Consumes: IPC channel `notification:mute-issue` with payload `{ slug: string; issueId: string }`.
- Produces: `window.desktopAPI.onNotificationMuteIssue(cb)` — subscribe in renderer; `api.unsubscribeFromIssue(issueId, userId?, userType?, workspaceSlug?)` — workspace override param added to web API client.

---

- [ ] **Step 3.1: Add `notification:mute-issue` to the message channel list**

In `apps/desktop/src/shared/main-renderer-messages.ts`:

```typescript
export const MAIN_RENDERER_MESSAGE_CHANNELS = [
  "auth:token",
  "invite:open",
  "inbox:open",
  "notification:mute-issue",  // add this line
] as const;
```

The `MainRendererMessageChannel` type union and `parseMainRendererChannelState` validator update automatically because they derive from the array.

- [ ] **Step 3.2: Add the preload bridge method**

In `apps/desktop/src/preload/index.ts`, add after the `onInboxOpen` method:

```typescript
/** Subscribe to "mute this issue's notifications" requests from the main
 *  process when the user clicks the notification action button. Returns
 *  an unsubscribe function. */
onNotificationMuteIssue: (
  callback: (payload: { slug: string; issueId: string }) => void,
) => subscribeToMainRendererChannel("notification:mute-issue", callback),
```

- [ ] **Step 3.3: Add notification action + IPC dispatch in main process**

In `apps/desktop/src/main/index.ts`, find the notification `click` handler block (around line 822) and add the actions array and `action` event handler immediately after `notification.on("click", ...)`:

```typescript
notification.actions = [{ type: "button", text: "Turn Off Notifications" }];
notification.on("action", (_, actionIndex) => {
  if (actionIndex !== 0) return;
  if (notificationSessionGeneration !== authSessionGeneration) return;
  dispatchToMainRenderer("notification:mute-issue", {
    slug: payload.slug,
    issueId: payload.issueKey,
  });
});
```

The `notification.actions` line must come before `notification.show()`.

- [ ] **Step 3.4: Add workspace slug override to `unsubscribeFromIssue`**

In `packages/core/api/client.ts`, update the `unsubscribeFromIssue` signature and body:

```typescript
async unsubscribeFromIssue(
  issueId: string,
  userId?: string,
  userType?: string,
  workspaceSlug?: string,
): Promise<void> {
  const body: Record<string, string> = {};
  if (userId) body.user_id = userId;
  if (userType) body.user_type = userType;
  await this.fetch(`/api/issues/${issueId}/unsubscribe`, {
    method: "POST",
    body: JSON.stringify(body),
    // workspaceSlug overrides the active-workspace header so the mute action
    // works even when the user is viewing a different workspace at the time.
    headers: workspaceSlug ? { "X-Workspace-Slug": workspaceSlug } : undefined,
  });
}
```

- [ ] **Step 3.5: Subscribe and call unsubscribe in the renderer**

In `apps/desktop/src/renderer/src/components/desktop-layout.tsx`, add a new `useEffect` alongside the existing `onInboxOpen` one. You'll also need `useQueryClient` and `issueKeys` if not already imported.

Check the existing imports at the top of the file. Add what's missing:
```typescript
import { useQueryClient } from "@tanstack/react-query";
import { issueKeys } from "@multica/core/issues/queries";
import { api } from "@multica/core/api";
```

Then add the effect:

```typescript
const qc = useQueryClient();

useEffect(() => {
  return window.desktopAPI.onNotificationMuteIssue(({ slug, issueId }) => {
    if (!slug || !issueId) return;
    void api
      .unsubscribeFromIssue(issueId, undefined, undefined, slug)
      .then(() => {
        qc.invalidateQueries({ queryKey: issueKeys.subscribers(issueId) });
      })
      .catch(() => {
        // Fire-and-forget: if the user was already unsubscribed or the
        // network request fails, the notification action is still a no-op
        // from the user's perspective — they'll see no new notifications
        // regardless because the server-side gate is the authority.
      });
  });
}, [qc]);
```

- [ ] **Step 3.6: Typecheck**

```bash
pnpm typecheck
```

Expected: no type errors.

- [ ] **Step 3.7: Commit**

```bash
git add apps/desktop/src/shared/main-renderer-messages.ts \
        apps/desktop/src/preload/index.ts \
        apps/desktop/src/main/index.ts \
        packages/core/api/client.ts \
        apps/desktop/src/renderer/src/components/desktop-layout.tsx
git commit -m "feat(desktop): add Turn Off Notifications action to macOS notification banner"
```

---

### Task 4: Mobile — register iOS notification category + handle mute action

**Files:**
- Modify: `apps/mobile/lib/push-notifications.ts`
- Modify: `apps/mobile/data/api.ts`
- Modify: `apps/mobile/app/_layout.tsx`

**Interfaces:**
- Consumes: `response.actionIdentifier === 'mute_issue'` and `response.actionIdentifier === Notifications.DEFAULT_ACTION_IDENTIFIER` in the notification response listener.
- Produces: `api.unsubscribeFromIssue(workspaceSlug: string, issueId: string): Promise<void>`.

---

- [ ] **Step 4.1: Register the iOS notification category**

In `apps/mobile/lib/push-notifications.ts`, add the category registration call inside `registerForPushNotifications`, after `finalStatus` is confirmed `'granted'` and before fetching the token:

```typescript
// Register the iOS notification category that enables the long-press
// "Turn Off Notifications" action. Safe to call on every registration —
// Expo deduplicates it. Android ignores this silently.
await Notifications.setNotificationCategoryAsync("issue_notification", [
  {
    identifier: "mute_issue",
    buttonTitle: "Turn Off Notifications",
    options: {
      isDestructive: false,
      isAuthenticationRequired: false,
    },
  },
]);
```

The full updated `registerForPushNotifications` function:

```typescript
export async function registerForPushNotifications(): Promise<string | null> {
  if (!Device.isDevice) return null;

  const { status: existing } = await Notifications.getPermissionsAsync();
  let finalStatus = existing;

  if (existing !== "granted") {
    const { status } = await Notifications.requestPermissionsAsync();
    finalStatus = status;
  }

  if (finalStatus !== "granted") return null;

  // Register the iOS notification category that enables the long-press
  // "Turn Off Notifications" action. Safe to call on every registration —
  // Expo deduplicates it. Android ignores this silently.
  await Notifications.setNotificationCategoryAsync("issue_notification", [
    {
      identifier: "mute_issue",
      buttonTitle: "Turn Off Notifications",
      options: {
        isDestructive: false,
        isAuthenticationRequired: false,
      },
    },
  ]);

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

- [ ] **Step 4.2: Add `unsubscribeFromIssue` to the mobile API client**

In `apps/mobile/data/api.ts`, find the section for issue-related methods. Add after the subscribe method (search for `subscribe` to find the right location). If no subscribe method exists yet, add it before the comment `// --- Labels ---` or near other issue mutation methods:

```typescript
async unsubscribeFromIssue(
  workspaceSlug: string,
  issueId: string,
): Promise<void> {
  await this.fetch<void>(`/api/issues/${issueId}/unsubscribe`, {
    method: "POST",
    // Pass the source workspace slug explicitly. The fetch helper only sets
    // X-Workspace-Slug from getCurrentSlug() when the header is absent, so
    // this header wins — correct when the user is viewing a different
    // workspace at the moment the notification action fires.
    headers: { "X-Workspace-Slug": workspaceSlug },
  });
}
```

- [ ] **Step 4.3: Handle the `mute_issue` action in the notification response listener**

In `apps/mobile/app/_layout.tsx`, find the `addNotificationResponseReceivedListener` callback (around line 73) and extend it to handle the mute action:

```typescript
const sub = Notifications.addNotificationResponseReceivedListener(
  (response) => {
    const data = response.notification.request.content.data as
      | { workspace_slug?: string; issue_id?: string }
      | undefined;

    // Long-press action: "Turn Off Notifications"
    if (response.actionIdentifier === "mute_issue") {
      if (data?.workspace_slug && data?.issue_id) {
        void api
          .unsubscribeFromIssue(data.workspace_slug, data.issue_id)
          .catch(() => {
            // Fire-and-forget: silently ignore network failures here.
            // The user will not receive notifications anyway because the
            // server's subscription check is the authority.
          });
      }
      return;
    }

    // Default tap: navigate to the issue.
    if (data?.workspace_slug && data?.issue_id) {
      router.push(
        `/${data.workspace_slug}/issue/${data.issue_id}` as Parameters<
          typeof router.push
        >[0],
      );
    }
  },
);
```

- [ ] **Step 4.4: Typecheck**

```bash
cd apps/mobile && pnpm typecheck
```

Expected: no type errors.

- [ ] **Step 4.5: Commit**

```bash
git add apps/mobile/lib/push-notifications.ts \
        apps/mobile/data/api.ts \
        apps/mobile/app/_layout.tsx
git commit -m "feat(mobile): register iOS mute-issue notification action and handle response"
```

---

## Verification Checklist

After all four tasks are merged and deployed:

- [ ] iOS: receive a notification from workspace B while viewing workspace A → notification body shows workspace B's name
- [ ] iOS: tap the notification → app navigates to the issue in workspace B
- [ ] iOS: long-press the notification banner → "Turn Off Notifications" button appears → tap it → no further notifications from that issue
- [ ] macOS: receive a notification → body shows workspace name
- [ ] macOS: click "Options" chevron on notification → "Turn Off Notifications" appears → click it → subscription cleared (verify in issue detail subscriber list)
- [ ] macOS: click notification body → navigates to correct workspace inbox even if a different workspace was active
- [ ] `make test` passes (Go)
- [ ] `pnpm typecheck` passes (TS)
