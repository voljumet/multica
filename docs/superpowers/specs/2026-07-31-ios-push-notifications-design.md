# iOS Push Notifications Design

**Date:** 2026-07-31

## Problem

Devs working on agent-assigned issues have no way to be notified when something
happens while the app is in the background. Two things are needed:

1. A way for devs to "follow" an issue that's assigned to an agent — so they
   receive inbox events when the agent comments, changes status, or completes.
2. iOS push notifications that deliver those inbox events to the device even
   when the app is backgrounded or killed.

The subscriber model (backend `issue_subscriber` table, `reason` field with
values `creator | assignee | commenter | mentioned | manual`) is already fully
built. The inbox fanout in `notification_listeners.go` already fans out to all
subscribers and respects notification preferences. What's missing is the
subscribe UI on mobile and the push delivery layer.

---

## Part 1 — Subscribe bell on issue detail

### UI placement

Bell icon added to the issue detail `headerRight`, between `AgentHeaderBadge`
and the `…` actions button.

- **Bell outline** (`notifications-outline`) = not subscribed
- **Bell filled** (`notifications`) = subscribed
- Tapping toggles subscription. Optimistic — immediate visual flip, rollback
  on error.
- Uses `expo-haptics` `ImpactFeedbackStyle.Light` on toggle.

### Data layer (mobile-local, mirrors `packages/core/issues/mutations.ts`)

**`apps/mobile/data/queries/subscribers.ts`**

```ts
subscriberKeys = {
  all: (issueId: string) => ["subscribers", issueId] as const,
}
subscribersOptions(issueId) → GET /api/issues/{id}/subscribers → IssueSubscriber[]
```

Schema: reuse `IssueSubscriber` type imported with `import type` from
`@multica/core/types/subscriber`.

**`apps/mobile/data/mutations/subscribers.ts`**

`useToggleSubscription(issueId)` — mirrors core's toggle mutation:

- `onMutate`: snapshot → optimistic flip (add/remove current user from cache)
- API: `POST /api/issues/{id}/subscribe` or `POST /api/issues/{id}/unsubscribe`
- `onError`: rollback
- `onSettled`: `invalidateQueries(subscriberKeys.all(issueId))`

**`apps/mobile/data/api.ts`** — two new methods:

```ts
subscribeToIssue(issueId: string): Promise<void>
unsubscribeFromIssue(issueId: string): Promise<void>
```

### Deriving subscribed state

In `issue/[id].tsx`, load `subscribersOptions(id)`. Derive
`isSubscribed = subscribers.some(s => s.user_id === me.id && s.user_type === "member")`.
Use `useMe()` (already available in the screen) for the current user id.

### Realtime

`subscriber:added` and `subscriber:removed` events exist in
`packages/core/types/events.ts`. On mobile, patch `subscriberKeys.all(issueId)`
cache in `useIssueRealtime(id)` (already mounted in `issue/[id].tsx`). No new
global hook needed — this is a per-record event.

---

## Part 2 — iOS push notifications

### Architecture

```
Mobile                    Backend                   Expo Push Service
  │                          │                            │
  ├─ requestPermission()     │                            │
  ├─ getExpoPushTokenAsync() │                            │
  ├─ POST /api/push-tokens ─►│                            │
  │                          │ store in device_push_tokens│
  │                          │                            │
  │   (inbox event fires)    │                            │
  │                          ├─ CreateInboxItem()         │
  │                          ├─ POST exp.host/push/send ─►│
  │                          │                            ├─ APNs
  │◄─────────────────────────────────────── notification ─┘
  │ tap notification         │
  ├─ navigate to issue/[id]  │
```

### Push service choice: Expo Push

Expo Push relays to APNs and handles certificate management. Already on Expo
SDK 55 so `expo install expo-notifications` gets the SDK-compatible version.
Can migrate to direct APNs later if needed; the server-side dispatch is one
function call that can be swapped.

### Database

**Migration 1 — table:**

```sql
CREATE TABLE device_push_tokens (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     UUID NOT NULL,
  token       TEXT NOT NULL,
  platform    TEXT NOT NULL DEFAULT 'expo',
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX CONCURRENTLY device_push_tokens_token_idx
  ON device_push_tokens (token);
```

**Migration 2 — user lookup index (separate file per CLAUDE.md):**

```sql
CREATE INDEX CONCURRENTLY device_push_tokens_user_id_idx
  ON device_push_tokens (user_id);
```

No foreign key on `user_id` (per CLAUDE.md). Tokens for deleted users are
cleaned up by a scheduled sweep or on next login attempt.

Upsert on register: `INSERT ... ON CONFLICT (token) DO UPDATE SET user_id = excluded.user_id, updated_at = now()`.

### Backend

**New endpoint: `POST /api/push-tokens`**

Request: `{ "token": "<expo-push-token>", "platform": "expo" }`

Handler in a new `push_token.go`:
- Authenticate via existing auth middleware (gets user from request context)
- Validate token format (must start with `ExponentPushToken[`)
- Upsert into `device_push_tokens`
- Return `204 No Content`

**Push dispatch in `notification_listeners.go`**

After each `CreateInboxItem` + `bus.Publish(...)` call, call a new
`sendPushNotification(ctx, queries, recipientID, item, workspaceSlug)` helper
that:

1. Looks up push tokens for `user_id = recipientID`
2. If none, returns (most users won't have a mobile device registered)
3. POSTs to `https://exp.host/api/v2/push/send` with:
   ```json
   {
     "to": "<token>",
     "title": "<item.Title>",
     "body": "<detail label text or blank>",
     "data": { "workspace_slug": "<slug>", "issue_id": "<item.IssueID>" },
     "sound": "default",
     "badge": 1
   }
   ```
4. Logs errors, never blocks the main notification path (goroutine or
   fire-and-forget with a short timeout)

Push already respects notification preferences because `sendPushNotification`
is called only after the `isNotifMuted` gate in the existing fanout. No
duplicate filtering needed.

**Workspace slug in notifications:** the `notification_listeners.go` fanout
already has `workspaceID`; add a workspace slug lookup (one query per event,
cached in the listener loop) to populate `data.workspace_slug` for deep linking.

### Mobile

**`apps/mobile/lib/push-notifications.ts`** — setup helper:

```ts
async function registerForPushNotifications(): Promise<string | null>
```

- Checks if device is physical (simulators can't receive push)
- Calls `Notifications.requestPermissionsAsync()`
- On grant: calls `Notifications.getExpoPushTokenAsync({ projectId })`
- Returns the token string or null

**Where to call it:** in workspace `_layout.tsx`, after the user has
authenticated and a workspace is selected. Call once per session; POST to
`/api/push-tokens` if a token is returned. Store the token in
`expo-secure-store` to avoid re-registering on every launch (only POST again
if the token changes).

**Deep link on notification tap:**

```ts
Notifications.addNotificationResponseReceivedListener((response) => {
  const { workspace_slug, issue_id } = response.notification.request.content.data;
  if (workspace_slug && issue_id) {
    router.push(`/${workspace_slug}/issue/${issue_id}`);
  }
});
```

Mount this listener in `app/_layout.tsx` (top-level, outside workspace scope
so it fires even when the app was killed and relaunched by the tap).

**`app.json` additions:**

```json
{
  "expo": {
    "plugins": [
      ["expo-notifications", {
        "icon": "./assets/notification-icon.png",
        "color": "#ffffff"
      }]
    ]
  }
}
```

---

## Parity points

| Behavior | Web/desktop source | Mobile implementation |
|---|---|---|
| Subscriber fanout logic | `notification_listeners.go` | No change — same server |
| Notification preference gates | `isNotifMuted()` in fanout | No change — same server gate |
| Subscribe/unsubscribe mutation shape | `packages/core/issues/mutations.ts` useToggleSubscription | Mirror in `apps/mobile/data/mutations/subscribers.ts` |
| Subscriber cache key | `issueKeys.subscribers(id)` | `subscriberKeys.all(id)` — 3-segment shape |
| Bell icon state | web header subscriber chip | mobile bell outline/filled |

---

## What's not in scope

- Android push (Expo supports FCM but mobile targets iOS; can be added later
  by extending `sendPushNotification` with FCM tokens)
- Notification grouping / threading (iOS Notification Center)
- Archived inbox view (separate ticket MUL-3736)
- Cross-workspace unread summary badge

---

## Files touched

**Mobile:**
- `apps/mobile/data/api.ts` — `subscribeToIssue`, `unsubscribeFromIssue`, `registerPushToken`
- `apps/mobile/data/queries/subscribers.ts` — new key factory + queryOptions
- `apps/mobile/data/mutations/subscribers.ts` — `useToggleSubscription`
- `apps/mobile/data/realtime/use-issue-realtime.ts` — handle `subscriber:added/removed`
- `apps/mobile/lib/push-notifications.ts` — new `registerForPushNotifications()`
- `apps/mobile/app/(app)/[workspace]/_layout.tsx` — call `registerForPushNotifications` + POST token
- `apps/mobile/app/(app)/[workspace]/issue/[id].tsx` — bell icon in `headerRight`, load subscribers
- `apps/mobile/app/_layout.tsx` — `addNotificationResponseReceivedListener` for deep link
- `apps/mobile/app.json` — `expo-notifications` plugin

**Server:**
- `server/internal/db/migrations/XXXX_add_device_push_tokens.sql` — table
- `server/internal/db/migrations/XXXX_device_push_tokens_user_idx.sql` — user index (separate file)
- `server/internal/db/queries/push_tokens.sql` — sqlc queries (upsert, list by user)
- `server/internal/handler/push_token.go` — `RegisterPushToken` handler
- `server/cmd/server/notification_listeners.go` — call `sendPushNotifications` after fanout
- `server/cmd/server/push_send.go` — new `sendPushNotification` helper (Expo HTTP call)
- `server/internal/handler/router.go` — register `POST /api/push-tokens` route
