import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  requestCloseTab,
  requestSetActiveTab,
  requestSwitchWorkspace,
  useTabLeaveGuardStore,
  wouldCloseActiveTab,
  wouldLeaveActiveTab,
} from "./tab-leave-guard";
import { useTabStore } from "@/stores/tab-store";

describe("tab-leave-guard", () => {
  beforeEach(() => {
    useTabLeaveGuardStore.getState().reset();
    useTabStore.getState().reset();
    useTabStore.getState().switchWorkspace("acme");
    // Second tab so close/switch can leave the first.
    useTabStore.getState().addTab("/acme/projects", "Projects");
  });

  it("wouldLeaveActiveTab is true only for a different tab", () => {
    const group = useTabStore.getState().byWorkspace.acme;
    const activeId = group.activeTabId;
    const otherId = group.tabs.find((t) => t.id !== activeId)!.id;
    expect(wouldLeaveActiveTab(activeId)).toBe(false);
    expect(wouldLeaveActiveTab(otherId)).toBe(true);
  });

  it("wouldCloseActiveTab is true only for the active tab", () => {
    const group = useTabStore.getState().byWorkspace.acme;
    const activeId = group.activeTabId;
    const otherId = group.tabs.find((t) => t.id !== activeId)!.id;
    expect(wouldCloseActiveTab(activeId)).toBe(true);
    expect(wouldCloseActiveTab(otherId)).toBe(false);
  });

  it("requestSetActiveTab opens a pending leave instead of switching immediately", () => {
    const group = useTabStore.getState().byWorkspace.acme;
    const activeId = group.activeTabId;
    const otherId = group.tabs.find((t) => t.id !== activeId)!.id;

    requestSetActiveTab(otherId);

    expect(useTabStore.getState().byWorkspace.acme.activeTabId).toBe(activeId);
    expect(useTabLeaveGuardStore.getState().pending).not.toBeNull();

    useTabLeaveGuardStore.getState().confirm();
    expect(useTabStore.getState().byWorkspace.acme.activeTabId).toBe(otherId);
    expect(useTabLeaveGuardStore.getState().pending).toBeNull();
  });

  it("cancel leaves the active tab unchanged", () => {
    const group = useTabStore.getState().byWorkspace.acme;
    const activeId = group.activeTabId;
    const otherId = group.tabs.find((t) => t.id !== activeId)!.id;

    requestSetActiveTab(otherId);
    useTabLeaveGuardStore.getState().cancel();

    expect(useTabStore.getState().byWorkspace.acme.activeTabId).toBe(activeId);
    expect(useTabLeaveGuardStore.getState().pending).toBeNull();
  });

  it("requestCloseTab closes inactive tabs without confirmation", () => {
    const group = useTabStore.getState().byWorkspace.acme;
    const activeId = group.activeTabId;
    const otherId = group.tabs.find((t) => t.id !== activeId)!.id;

    requestCloseTab(otherId);

    expect(useTabLeaveGuardStore.getState().pending).toBeNull();
    expect(
      useTabStore.getState().byWorkspace.acme.tabs.find((t) => t.id === otherId),
    ).toBeUndefined();
    expect(useTabStore.getState().byWorkspace.acme.activeTabId).toBe(activeId);
  });

  it("requestCloseTab confirms before closing the active tab", () => {
    const group = useTabStore.getState().byWorkspace.acme;
    const activeId = group.activeTabId;

    requestCloseTab(activeId);
    expect(useTabLeaveGuardStore.getState().pending).not.toBeNull();
    expect(
      useTabStore.getState().byWorkspace.acme.tabs.find((t) => t.id === activeId),
    ).toBeDefined();

    useTabLeaveGuardStore.getState().confirm();
    expect(
      useTabStore.getState().byWorkspace.acme.tabs.find((t) => t.id === activeId),
    ).toBeUndefined();
  });

  it("requestSwitchWorkspace confirms when a tab is mounted", () => {
    requestSwitchWorkspace("butter");
    expect(useTabLeaveGuardStore.getState().pending).not.toBeNull();
    expect(useTabStore.getState().activeWorkspaceSlug).toBe("acme");

    useTabLeaveGuardStore.getState().confirm();
    expect(useTabStore.getState().activeWorkspaceSlug).toBe("butter");
  });

  it("requestSetActiveTab is a no-op for the already-active tab", () => {
    const activeId = useTabStore.getState().byWorkspace.acme.activeTabId;
    const spy = vi.spyOn(useTabStore.getState(), "setActiveTab");
    requestSetActiveTab(activeId);
    expect(useTabLeaveGuardStore.getState().pending).toBeNull();
    // Implementation returns early without calling setActiveTab.
    expect(spy).not.toHaveBeenCalled();
    spy.mockRestore();
  });
});
