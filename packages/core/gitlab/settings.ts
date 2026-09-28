import { useQueryClient, useMutation } from "@tanstack/react-query";
import type { Workspace } from "../types";
import { api } from "../api";
import { gitlabKeys } from "./queries";

/** Default GitLab label that triggers Multica issue creation. */
export const DEFAULT_GITLAB_ISSUE_SYNC_LABEL = "agent";

export interface GitLabSettings {
  issueSync: boolean;
  /**
   * GitLab label title that creates/syncs Multica issues. Defaults to "agent"
   * so workspaces predating this setting keep historical behavior.
   */
  issueSyncLabel: string;
}

export function deriveGitLabSettings(
  workspace: Pick<Workspace, "settings"> | null | undefined,
): GitLabSettings {
  const s = (workspace?.settings ?? {}) as Record<string, unknown>;
  const rawLabel = s.gitlab_issue_sync_label;
  const issueSyncLabel =
    typeof rawLabel === "string" && rawLabel.trim() !== ""
      ? rawLabel.trim()
      : DEFAULT_GITLAB_ISSUE_SYNC_LABEL;
  return {
    issueSync: s.gitlab_issue_sync_enabled !== false,
    issueSyncLabel,
  };
}

export function useLinkGitLabIssue(issueId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ projectPath, glIssueIid }: { projectPath: string; glIssueIid: number }) =>
      api.linkGitLabIssue(issueId, projectPath, glIssueIid),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: gitlabKeys.gitlabIssue(issueId) });
    },
  });
}

export function useUnlinkGitLabIssue(issueId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.unlinkGitLabIssue(issueId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: gitlabKeys.gitlabIssue(issueId) });
    },
  });
}
