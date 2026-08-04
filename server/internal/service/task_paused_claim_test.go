package service

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// TestClaimTaskPausedAgentHoldsQueue verifies the per-agent pause gate:
// while paused_at is set, ClaimTask returns no task even with queued work
// and capacity; clearing it makes the same queue claimable again.
func TestClaimTaskPausedAgentHoldsQueue(t *testing.T) {
	ctx := context.Background()
	pool := newTaskClaimRacePool(t)
	queries := db.New(pool)

	agentID := createClaimCapacityFixture(t, ctx, pool)
	agentUUID := util.MustParseUUID(agentID)
	svc := NewTaskService(queries, pool, nil, events.New())

	if _, err := pool.Exec(ctx, `UPDATE agent SET paused_at = now() WHERE id = $1`, agentID); err != nil {
		t.Fatalf("pause agent: %v", err)
	}

	task, err := svc.ClaimTask(ctx, agentUUID)
	if err != nil {
		t.Fatalf("claim task while paused: %v", err)
	}
	if task != nil {
		t.Fatalf("expected no task claimed while paused, got %s", util.UUIDToString(task.ID))
	}

	if _, err := pool.Exec(ctx, `UPDATE agent SET paused_at = NULL WHERE id = $1`, agentID); err != nil {
		t.Fatalf("resume agent: %v", err)
	}

	task, err = svc.ClaimTask(ctx, agentUUID)
	if err != nil {
		t.Fatalf("claim task after resume: %v", err)
	}
	if task == nil {
		t.Fatal("expected a task claimed after resume, got none")
	}
}

// TestExpireStaleQueuedTasksSkipsPausedAgents verifies the queued-TTL
// sweeper leaves a paused agent's queue alone: pausing exists precisely to
// hold a large queue while settings change, so expiry must not drain it.
func TestExpireStaleQueuedTasksSkipsPausedAgents(t *testing.T) {
	ctx := context.Background()
	pool := newTaskClaimRacePool(t)
	queries := db.New(pool)

	agentID := createClaimCapacityFixture(t, ctx, pool)

	// Age both queued fixture tasks past a 1-second TTL.
	if _, err := pool.Exec(ctx, `
		UPDATE agent_task_queue SET created_at = now() - interval '1 hour'
		WHERE agent_id = $1 AND status = 'queued'
	`, agentID); err != nil {
		t.Fatalf("age queued tasks: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE agent SET paused_at = now() WHERE id = $1`, agentID); err != nil {
		t.Fatalf("pause agent: %v", err)
	}

	expired, err := queries.ExpireStaleQueuedTasks(ctx, db.ExpireStaleQueuedTasksParams{
		TtlSecs:    1,
		MaxPerTick: 100,
	})
	if err != nil {
		t.Fatalf("expire stale queued tasks: %v", err)
	}
	for _, task := range expired {
		if util.UUIDToString(task.AgentID) == agentID {
			t.Fatalf("paused agent's queued task %s was expired", util.UUIDToString(task.ID))
		}
	}

	var stillQueued int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM agent_task_queue WHERE agent_id = $1 AND status = 'queued'
	`, agentID).Scan(&stillQueued); err != nil {
		t.Fatalf("count queued: %v", err)
	}
	if stillQueued != 2 {
		t.Fatalf("expected 2 queued tasks to survive expiry while paused, got %d", stillQueued)
	}

	if _, err := pool.Exec(ctx, `UPDATE agent SET paused_at = NULL WHERE id = $1`, agentID); err != nil {
		t.Fatalf("resume agent: %v", err)
	}
	expired, err = queries.ExpireStaleQueuedTasks(ctx, db.ExpireStaleQueuedTasksParams{
		TtlSecs:    1,
		MaxPerTick: 100,
	})
	if err != nil {
		t.Fatalf("expire stale queued tasks after resume: %v", err)
	}
	var expiredForAgent int
	for _, task := range expired {
		if util.UUIDToString(task.AgentID) == agentID {
			expiredForAgent++
		}
	}
	if expiredForAgent != 2 {
		t.Fatalf("expected 2 tasks expired after resume, got %d", expiredForAgent)
	}
}
