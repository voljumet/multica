import { create } from "zustand";
import {
  useTabStore,
  getActiveTab,
  sanitizeTabPath,
  resourceKeyForUrl,
} from "@/stores/tab-store";

/**
 * Leave-tab confirmation for the desktop single-router architecture.
 *
 * Only the active tab is mounted. Switching workspace/tabs or closing the
 * active tab remounts the page tree and drops local React state that is not
 * in a draft store / server save path. This guard asks before that unmount
 * so users are not surprised mid-edit.
 *
 * "Don't ask again this session" is process-local (not persisted) so a new
 * launch re-enables the warning.
 */

type LeaveAction = () => void;

interface TabLeaveGuardState {
  pending: LeaveAction | null;
  skipForSession: boolean;
  /**
   * Queue `action` behind a confirmation dialog, or run it immediately when
   * the user opted out for this session.
   */
  requestLeave: (action: LeaveAction) => void;
  /** Run the pending leave action. Optionally suppress further prompts. */
  confirm: (dontAskAgain?: boolean) => void;
  cancel: () => void;
  /** Test helper — clear session skip + pending. */
  reset: () => void;
}

export const useTabLeaveGuardStore = create<TabLeaveGuardState>((set, get) => ({
  pending: null,
  skipForSession: false,
  requestLeave(action) {
    if (get().skipForSession) {
      action();
      return;
    }
    // Replace any prior pending leave — only one dialog at a time.
    set({ pending: action });
  },
  confirm(dontAskAgain = false) {
    const { pending } = get();
    set({
      pending: null,
      skipForSession: dontAskAgain ? true : get().skipForSession,
    });
    pending?.();
  },
  cancel() {
    set({ pending: null });
  },
  reset() {
    set({ pending: null, skipForSession: false });
  },
}));

/** True when activating `tabId` would unmount the currently active tab. */
export function wouldLeaveActiveTab(tabId: string): boolean {
  const active = getActiveTab(useTabStore.getState());
  return !!active && active.id !== tabId;
}

/** True when closing `tabId` would unmount the currently active tab. */
export function wouldCloseActiveTab(tabId: string): boolean {
  const active = getActiveTab(useTabStore.getState());
  return !!active && active.id === tabId;
}

/**
 * Whether `openTab(path, …, { activate })` would change the active tab
 * (and therefore unmount the current host). Mirrors openTab's dedupe +
 * activate rules without mutating the store.
 */
function wouldOpenTabLeaveActive(path: string, activate?: boolean): boolean {
  const state = useTabStore.getState();
  const active = getActiveTab(state);
  if (!active || !state.activeWorkspaceSlug) return false;
  const group = state.byWorkspace[state.activeWorkspaceSlug];
  if (!group) return false;

  const clean = sanitizeTabPath(path);
  if (!clean) return false;

  const key = resourceKeyForUrl(clean);
  const existing = group.tabs.find((t) => t.resourceKey === key);
  if (existing) {
    // openTab always focuses an existing match.
    return existing.id !== group.activeTabId;
  }
  // New tab only steals focus when activate: true.
  return activate === true;
}

/**
 * Activate a tab, confirming first when that leaves the current mount.
 * No-op when the tab is already active.
 */
export function requestSetActiveTab(tabId: string): void {
  if (!wouldLeaveActiveTab(tabId)) {
    const active = getActiveTab(useTabStore.getState());
    if (active?.id === tabId) return;
    useTabStore.getState().setActiveTab(tabId);
    return;
  }
  useTabLeaveGuardStore.getState().requestLeave(() => {
    useTabStore.getState().setActiveTab(tabId);
  });
}

/**
 * Close a tab. Confirms only when closing the active tab (the only mount
 * that still holds ephemeral React state). Inactive tabs are already pure
 * session state — closing them is free.
 */
export function requestCloseTab(tabId: string): void {
  if (!wouldCloseActiveTab(tabId)) {
    useTabStore.getState().closeTab(tabId);
    return;
  }
  useTabLeaveGuardStore.getState().requestLeave(() => {
    useTabStore.getState().closeTab(tabId);
  });
}

/** Close the active tab (Cmd/Ctrl+W path), with confirmation. */
export function requestCloseActiveTab(): void {
  const active = getActiveTab(useTabStore.getState());
  if (!active) return;
  useTabLeaveGuardStore.getState().requestLeave(() => {
    useTabStore.getState().closeActiveTab();
  });
}

/**
 * Switch workspace (and optionally open a path). Always remounts the host,
 * so always confirm when there is a currently mounted active tab.
 */
export function requestSwitchWorkspace(slug: string, openPath?: string): void {
  const active = getActiveTab(useTabStore.getState());
  if (!active) {
    useTabStore.getState().switchWorkspace(slug, openPath);
    return;
  }
  useTabLeaveGuardStore.getState().requestLeave(() => {
    useTabStore.getState().switchWorkspace(slug, openPath);
  });
}

/**
 * Open (or focus) a tab and optionally activate it. Confirms when the
 * open would unmount the current host. Defers the store mutation until
 * confirm so Cancel never creates a background tab either.
 *
 * Returns the tab id only when the open runs synchronously (no dialog).
 * Callers that activate after a confirm must not rely on the return value.
 */
export function requestOpenTab(
  path: string,
  title: string,
  icon: string,
  opts?: { activate?: boolean },
): string {
  const store = useTabStore.getState();
  if (!wouldOpenTabLeaveActive(path, opts?.activate)) {
    return store.openTab(path, title, icon, opts);
  }

  useTabLeaveGuardStore.getState().requestLeave(() => {
    useTabStore.getState().openTab(path, title, icon, opts);
  });
  return "";
}

/**
 * Cmd/Ctrl+W handler: confirm before closing the active tab, or before
 * closing the window when it is the last tab (same unmount risk).
 */
export function requestCmdWClose(): void {
  const store = useTabStore.getState();
  const { activeWorkspaceSlug, byWorkspace } = store;
  if (!activeWorkspaceSlug) {
    window.desktopAPI.closeWindow();
    return;
  }
  const group = byWorkspace[activeWorkspaceSlug];
  if (!group || group.tabs.length <= 1) {
    useTabLeaveGuardStore.getState().requestLeave(() => {
      window.desktopAPI.closeWindow();
    });
    return;
  }
  requestCloseActiveTab();
}
