import { describe, it, expect } from "vitest";
import { DEFAULT_GITLAB_ISSUE_SYNC_LABEL, deriveGitLabSettings } from "./settings";
import type { Workspace } from "../types";

function ws(settings: Record<string, unknown>): Pick<Workspace, "settings"> {
  return { settings };
}

describe("deriveGitLabSettings", () => {
  it("defaults to synced when workspace is null", () => {
    expect(deriveGitLabSettings(null)).toEqual({
      issueSync: true,
      issueSyncLabel: DEFAULT_GITLAB_ISSUE_SYNC_LABEL,
    });
  });

  it("defaults to synced on empty settings", () => {
    expect(deriveGitLabSettings(ws({}))).toEqual({
      issueSync: true,
      issueSyncLabel: DEFAULT_GITLAB_ISSUE_SYNC_LABEL,
    });
  });

  it("issue sync can be turned off", () => {
    expect(deriveGitLabSettings(ws({ gitlab_issue_sync_enabled: false }))).toMatchObject({
      issueSync: false,
    });
  });

  it("reads a custom issue sync label and falls back for blank values", () => {
    expect(deriveGitLabSettings(ws({ gitlab_issue_sync_label: "multica" })).issueSyncLabel).toBe("multica");
    expect(deriveGitLabSettings(ws({ gitlab_issue_sync_label: "  triage  " })).issueSyncLabel).toBe("triage");
    expect(deriveGitLabSettings(ws({ gitlab_issue_sync_label: "" })).issueSyncLabel).toBe(DEFAULT_GITLAB_ISSUE_SYNC_LABEL);
    expect(deriveGitLabSettings(ws({ gitlab_issue_sync_label: "   " })).issueSyncLabel).toBe(DEFAULT_GITLAB_ISSUE_SYNC_LABEL);
    expect(deriveGitLabSettings(ws({ gitlab_issue_sync_label: 42 })).issueSyncLabel).toBe(DEFAULT_GITLAB_ISSUE_SYNC_LABEL);
  });
});
