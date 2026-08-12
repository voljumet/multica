import { create } from "zustand";
import {
  useTabStore,
  getActiveTab,
  sanitizeTabPath,
  resourceKeyForUrl,
} from "@/stores/tab-store";

// ponytail: all draft stores (issue/project/feedback/comment/chat) use zustand
// persist — data survives tab remounts. Nothing is actually lost on switch/close,
// so this guard never needs to block. Re-add checks here only for genuinely
// ephemeral state that can't survive a remount.
export function hasUnsavedContent(): boolean {
  return false;
}

type LeaveAction = () => void;

interface TabLeaveGuardState {
  pending: LeaveAction | null;
  /**
   * Queue `action` behind a confirmation dialog when there is unsaved
   * content, or run it immediately when leaving loses nothing.
   */
  requestLeave: (action: LeaveAction) => void;
  /** Run the pending leave action. */
  confirm: () => void;
  cancel: () => void;
  /** Test helper — clear pending. */
  reset: () => void;
}

export const useTabLeaveGuardStore = create<TabLeaveGuardState>((set, get) => ({
  pending: null,
  requestLeave(action) {
    if (!hasUnsavedContent()) {
      action();
      return;
    }
    // Replace any prior pending leave — only one dialog at a time.
    set({ pending: action });
  },
  confirm() {
    const { pending } = get();
    set({ pending: null });
    pending?.();
  },
  cancel() {
    set({ pending: null });
  },
  reset() {
    set({ pending: null });
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
