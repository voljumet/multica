CREATE TABLE device_push_tokens (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL,
    token      TEXT        NOT NULL,
    platform   TEXT        NOT NULL DEFAULT 'expo',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
