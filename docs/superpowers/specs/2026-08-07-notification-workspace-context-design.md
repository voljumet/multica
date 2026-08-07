# Notification Workspace Context & Mute Action

**Date:** 2026-08-07
**Status:** Approved

## Summary

Three improvements to push notifications on iOS (mobile) and desktop (Electron):

1. Show the source workspace name in the notification so users know which workspace it came from.
2. Tapping a notification switches to the correct workspace automatically.
3. Long-pressing a notification on iOS (or clicking the action button on macOS) unsubscribes the user from that issue's notifications.

## Current State

| Aspect | Current behavior |
|---|---|
| Push payload (server → Expo) | `{ title, sound, data: { workspace_slug, issue_id } }` — no body, no category |
| Desktop notification payload | `{ slug, itemId, issueKey, title, body }` — no workspace name field |
| Workspace name in notification | Not shown on either platform |
| Workspace switch on tap | Desktop: already works (`push()` delegates to `switchWorkspace` for cross-workspace paths). Mobile: `router.push(/${slug}/issue/${id})` routes through the workspace layout which calls `setCurrentWorkspace`. Both already correct. |
| Mute action | Not available on either platform |

## Feature 1 — Workspace Name

### Server (`server/cmd/server/push_send.go`)

- Add `Body string \`json:"body,omitempty"\`` to `expoPushMessage` struct.
- Add `workspaceNameByID(ctx, queries, workspaceID) string` helper (same shape as `workspaceSlugByID`, returns `ws.Name`).
- Add `body string` parameter to `sendPushDirect` and `sendPushNotifications`.
- Set `Body: body` in every `expoPushMessage`. Callers pass the workspace name.

### Server (`server/cmd/server/notification_listeners.go`)

- Two call sites invoke `sendPushNotifications` (in `notifySubscribers` and `notifyDirect`). At each, also compute `wsName := workspaceNameByID(...)` and pass it through.

### Desktop (`packages/core/realtime/use-realtime-sync.ts`)

- In `handleInboxNew()`, resolve workspace name from query cache. The workspace list is warm by the time inbox events arrive; look up by `sourceWsId`.
- Add `workspaceName?: string` to `SystemNotificationPayload` (defined in `packages/core/platform/system-notification.ts`).
- Pass the resolved name in the payload sent via `desktopAPI.showNotification(payload)`.

### Desktop (`apps/desktop/src/main/index.ts`)

- When constructing the `Notification`, if `payload.workspaceName` is present, prepend it to the notification body:
  `body: [payload.workspaceName, payload.body].filter(Boolean).join('\n')`

### Display result

- iOS: workspace name appears as the notification subtitle (the `body` field), below the title line.
- macOS: workspace name appears as the first line of the notification body text.

## Feature 2 — Workspace Switch on Tap

No code changes needed.

- **Desktop**: `push(inboxPath)` in `desktop-layout.tsx` line 204 already delegates to `switchWorkspace` for cross-workspace paths (verified in `navigation.test.tsx` line 140).
- **Mobile**: `router.push(/${workspace_slug}/issue/${id})` loads the `(app)/[workspace]/_layout.tsx` which calls `setCurrentWorkspace`. If the user is in a different workspace, Expo Router transitions to the new workspace layout first.

## Feature 3 — Mute Issue Action

### Server (`server/cmd/server/push_send.go`)

Add `"category": "issue_notification"` to the `data` map in `sendPushDirect` (alongside `workspace_slug` and `issue_id`). Expo Cloud forwards this as the iOS `categoryIdentifier`, which iOS uses to look up registered notification action categories.

### Mobile — category registration (`apps/mobile/lib/push-notifications.ts`)

After requesting permissions, register the category:

```ts
await Notifications.setNotificationCategoryAsync('issue_notification', [
  {
    identifier: 'mute_issue',
    buttonTitle: 'Turn Off Notifications',
    options: { isDestructive: false, isAuthenticationRequired: false },
  },
]);
```

On iOS, this causes a "Turn Off Notifications" button to appear when the user long-presses (3D Touch / press-and-hold) the notification banner.

### Mobile — response handler (`apps/mobile/app/_layout.tsx`)

Extend the `addNotificationResponseReceivedListener` callback:

```ts
if (response.actionIdentifier === 'mute_issue') {
  // Don't navigate — just mute and return.
  if (data?.workspace_slug && data?.issue_id) {
    await api.unsubscribeFromIssue(data.workspace_slug, data.issue_id);
  }
  return;
}
// Existing DEFAULT_ACTION_IDENTIFIER navigation logic unchanged.
```

### Mobile — API method (`apps/mobile/data/api.ts`)

Add `unsubscribeFromIssue(workspaceSlug: string, issueId: string): Promise<void>`:

- Calls `POST /api/issues/${issueId}/unsubscribe`
- Sets `X-Workspace-Slug: workspaceSlug` header (same pattern as other workspace-scoped calls)
- Uses `this.fetch<void>(...)` — response body is not consumed

### Desktop — notification action (`apps/desktop/src/main/index.ts`)

Add action to the Electron `Notification` before `.show()`:

```ts
notification.actions = [{ type: 'button', text: 'Turn Off Notifications' }];
notification.on('action', (_, actionIndex) => {
  if (actionIndex !== 0) return;
  dispatchToMainRenderer('notification:mute-issue', {
    slug: payload.slug,
    issueId: payload.issueKey,
  });
});
```

`issueKey` equals `item.issue_id` for issue-attached notifications (the only type that can be muted).

### Desktop — preload bridge (`apps/desktop/src/preload/index.ts`)

Expose:

```ts
onNotificationMuteIssue: (
  callback: (payload: { slug: string; issueId: string }) => void
) => subscribeToMainRendererChannel('notification:mute-issue', callback),
```

Add `notification:mute-issue` to `main-renderer-messages.ts` channel list.

### Desktop — renderer handler (`apps/desktop/src/renderer/src/components/desktop-layout.tsx`)

In the `useEffect` block alongside `onInboxOpen`, subscribe:

```ts
return window.desktopAPI.onNotificationMuteIssue(({ slug, issueId }) => {
  if (!slug || !issueId) return;
  // POST /api/issues/{issueId}/unsubscribe with workspace header
  mutateUnsubscribe({ slug, issueId });
});
```

Use the existing pattern for workspace-scoped mutations; the slug sets the workspace header.

## Files Changed

| File | Change |
|---|---|
| `server/cmd/server/push_send.go` | Add `Body` to struct; add `workspaceNameByID`; add `body` param |
| `server/cmd/server/notification_listeners.go` | Compute `wsName`, pass to `sendPushNotifications` (3 call sites); add `"category"` to push data |
| `packages/core/platform/system-notification.ts` | Add `workspaceName?: string` to `SystemNotificationPayload` |
| `packages/core/realtime/use-realtime-sync.ts` | Resolve workspace name from cache, populate `payload.workspaceName` |
| `apps/desktop/src/main/index.ts` | Prepend workspace name to body; add notification action + handler |
| `apps/desktop/src/shared/main-renderer-messages.ts` | Add `notification:mute-issue` channel |
| `apps/desktop/src/preload/index.ts` | Add `onNotificationMuteIssue` IPC bridge |
| `apps/desktop/src/renderer/src/components/desktop-layout.tsx` | Subscribe to mute-issue, call unsubscribe mutation |
| `apps/mobile/lib/push-notifications.ts` | Register `issue_notification` category with `mute_issue` action |
| `apps/mobile/app/_layout.tsx` | Handle `mute_issue` actionIdentifier in response listener |
| `apps/mobile/data/api.ts` | Add `unsubscribeFromIssue(workspaceSlug, issueId)` method |

## Out of Scope

- Android notification action support (falls back gracefully — action simply doesn't appear)
- Per-issue re-subscribe flow (mute is one-way from notification; re-subscribe via issue detail screen)
- Cold-start notification handling gap (existing behavior, separate issue)
