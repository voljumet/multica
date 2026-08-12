-- Partial index for the issue retention sweeper: quickly finds closed issues
-- older than the retention cutoff without scanning open issues.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_issue_closed_updated_at
    ON issue(updated_at)
    WHERE status IN ('done', 'cancelled');
