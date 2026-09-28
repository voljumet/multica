// GitLab OAuth connection/user-link/merge-request-mirroring types are gone —
// that auth, connection, and MR/CI mirroring now goes through the generic VCS
// integration (packages/core/types/vcs.ts). Only label-triggered issue sync
// remains GitLab-specific; see packages/core/gitlab and
// server/internal/handler/gitlab_issue_sync.go.

export interface GitLabIssue {
  gl_issue_iid: number;
  project_path: string;
  url: string;
  gl_assignee_username: string | null;
}
