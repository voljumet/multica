CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_gitlab_user_link_member ON gitlab_user_link(workspace_id, member_id);
