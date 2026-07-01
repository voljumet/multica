ALTER TABLE agent_runtime DROP CONSTRAINT IF EXISTS agent_runtime_visibility_check;
ALTER TABLE agent_runtime
    ADD CONSTRAINT agent_runtime_visibility_check
    CHECK (visibility IN ('private', 'public', 'shared'));
