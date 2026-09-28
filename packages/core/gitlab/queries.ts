import { queryOptions } from "@tanstack/react-query";
import { api, ApiError } from "../api";
import type { GitLabIssue } from "../types/gitlab";

export const gitlabKeys = {
  gitlabIssue: (issueId: string) => ["gitlab", "issue", issueId] as const,
};

export const issueGitLabIssueOptions = (issueId: string) =>
  queryOptions<GitLabIssue | null>({
    queryKey: gitlabKeys.gitlabIssue(issueId),
    queryFn: async () => {
      try {
        return await api.getGitLabIssue(issueId);
      } catch (e) {
        if (e instanceof ApiError && e.status === 404) return null;
        throw e;
      }
    },
    enabled: !!issueId,
  });
