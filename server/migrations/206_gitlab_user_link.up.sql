CREATE TABLE IF NOT EXISTS gitlab_user_link (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID        NOT NULL,
    member_id       UUID        NOT NULL,
    gitlab_username TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
