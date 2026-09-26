CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_gitlab_user_link_username ON gitlab_user_link(workspace_id, gitlab_username);
