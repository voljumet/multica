-- Per-agent queue pause: when set, the claim path stops dispatching this
-- agent's queued tasks and the queued-TTL sweeper leaves them alone, so
-- operators can hold a large queue while changing agent settings.
ALTER TABLE agent ADD COLUMN paused_at TIMESTAMPTZ;
