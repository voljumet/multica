/**
 * Agent run transcript — formSheet pushed from the Agent Runs list.
 *
 * Shows the full execution log for one task (thinking / tools / agent
 * text / errors). While the task is active, `task:message` WS events
 * (subscribed on the parent issue via `useIssueRealtime`) append into
 * the shared `["task-messages", taskId]` cache so this screen grows live.
 *
 * On open we force a REST backfill and merge-by-seq so any gap from a
 * reconnect (or messages emitted before this screen mounted) is healed
 * without blind-replacing rows the WS already delivered. Mirrors web's
 * LiveTranscriptDialog in packages/views/common/task-transcript/.
 */
import { useEffect, useMemo, useRef } from "react";
import {
  ActivityIndicator,
  Alert,
  Pressable,
  ScrollView,
  View,
} from "react-native";
import { useLocalSearchParams } from "expo-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { AgentTask, TaskMessagePayload } from "@multica/core/types";
import { Text } from "@/components/ui/text";
import { ActorAvatar } from "@/components/ui/actor-avatar";
import { PulseDot } from "@/components/ui/pulse-dot";
import { ChatTimeline } from "@/components/chat/chat-timeline";
import { useCancelTask } from "@/data/mutations/issues";
import {
  issueActiveTasksOptions,
  issueTasksOptions,
} from "@/data/queries/issues";
import {
  chatKeys,
  isTaskMessageTaskId,
  mergeTaskMessagesBySeq,
  taskMessagesOptions,
} from "@/data/queries/chat";
import { api } from "@/data/api";
import { useActorLookup } from "@/data/use-actor-name";
import { useWorkspaceStore } from "@/data/workspace-store";
import { timeAgo } from "@/lib/time-ago";

const LIVE_STATUSES: readonly AgentTask["status"][] = [
  "queued",
  "dispatched",
  "waiting_local_directory",
  "running",
];

const STATUS_LABEL: Record<AgentTask["status"], string> = {
  queued: "Queued",
  dispatched: "Starting",
  waiting_local_directory: "Waiting for directory",
  running: "Working",
  completed: "Done",
  failed: "Failed",
  cancelled: "Cancelled",
};

export default function IssueRunTranscriptRoute() {
  const { id: issueId, taskId } = useLocalSearchParams<{
    id: string;
    taskId: string;
  }>();
  const wsId = useWorkspaceStore((s) => s.currentWorkspaceId);
  const qc = useQueryClient();
  const { getName } = useActorLookup();

  const { data: allTasks = [] } = useQuery(issueTasksOptions(wsId, issueId));
  const { data: activeTasks = [] } = useQuery(
    issueActiveTasksOptions(wsId, issueId),
  );

  const task = useMemo(() => {
    const fromAll = allTasks.find((t) => t.id === taskId);
    if (fromAll) return fromAll;
    return activeTasks.find((t) => t.id === taskId);
  }, [allTasks, activeTasks, taskId]);

  const isLive = task ? LIVE_STATUSES.includes(task.status) : false;
  // Latch live for this open session so a running→terminal flip mid-view
  // doesn't drop the feed before the final backfill lands (web does the
  // same in TranscriptButton).
  const wasLiveRef = useRef(isLive);
  if (isLive) wasLiveRef.current = true;
  const showAsLive = isLive || wasLiveRef.current;

  const { data: messages = [], isLoading, isFetched } = useQuery(
    taskMessagesOptions(taskId),
  );

  // Force backfill on open + again when the task reaches a terminal state.
  // `taskMessagesOptions` is staleTime: Infinity, so without this a reconnect
  // gap (or the final tail never re-broadcast for completed issue tasks)
  // would leave holes. Merge-by-seq keeps concurrent WS appends.
  useEffect(() => {
    if (!isTaskMessageTaskId(taskId)) return;
    let cancelled = false;
    api
      .listTaskMessages(taskId)
      .then((msgs) => {
        if (cancelled) return;
        qc.setQueryData<TaskMessagePayload[]>(
          chatKeys.taskMessages(taskId),
          (old = []) => mergeTaskMessagesBySeq(old, msgs),
        );
      })
      .catch((err) => {
        console.error("task messages backfill failed", err);
      });
    return () => {
      cancelled = true;
    };
  }, [taskId, isLive, qc]);

  const scrollRef = useRef<ScrollView>(null);
  const prevCountRef = useRef(0);
  useEffect(() => {
    if (messages.length > prevCountRef.current && showAsLive) {
      // Keep the live tail in view as new events stream in.
      requestAnimationFrame(() => {
        scrollRef.current?.scrollToEnd({ animated: true });
      });
    }
    prevCountRef.current = messages.length;
  }, [messages.length, showAsLive]);

  const agentName = task
    ? getName("agent", task.agent_id)
    : "Agent";
  const statusLabel = task
    ? (STATUS_LABEL[task.status] ?? task.status)
    : "";
  const timestamp = task
    ? task.completed_at || task.started_at || task.created_at
    : null;

  return (
    <View className="flex-1">
      <View className="px-4 pt-4 pb-3 gap-2 border-b border-border">
        <Text className="text-base font-semibold text-foreground">
          Transcript
        </Text>
        {task ? (
          <View className="flex-row items-center gap-2">
            <ActorAvatar
              type="agent"
              id={task.agent_id}
              size={28}
              showPresence
            />
            <View className="flex-1">
              <Text className="text-sm font-medium text-foreground">
                {agentName}
              </Text>
              <View className="flex-row items-center gap-1.5">
                {isLive ? <PulseDot /> : null}
                <Text className="text-xs text-muted-foreground">
                  {statusLabel}
                  {timestamp ? ` · ${timeAgo(timestamp)}` : ""}
                </Text>
              </View>
            </View>
            {isLive ? (
              <CancelButton taskId={task.id} issueId={issueId} />
            ) : null}
          </View>
        ) : null}
      </View>

      <ScrollView
        ref={scrollRef}
        className="flex-1"
        contentContainerClassName="px-4 py-3 pb-8"
        showsVerticalScrollIndicator={false}
      >
        {isLoading && !isFetched ? (
          <View className="py-12 items-center">
            <ActivityIndicator />
          </View>
        ) : messages.length === 0 ? (
          <EmptyState isLive={isLive} />
        ) : (
          <ChatTimeline
            items={messages}
            isStreaming={isLive}
            variant="full"
          />
        )}
      </ScrollView>
    </View>
  );
}

function EmptyState({ isLive }: { isLive: boolean }) {
  return (
    <View className="py-12 items-center gap-2">
      {isLive ? <PulseDot /> : null}
      <Text className="text-sm text-muted-foreground text-center">
        {isLive
          ? "Waiting for events…"
          : "No execution events for this run."}
      </Text>
    </View>
  );
}

function CancelButton({
  taskId,
  issueId,
}: {
  taskId: string;
  issueId: string;
}) {
  const mutation = useCancelTask(issueId);

  const onPress = () => {
    Alert.alert(
      "Cancel task?",
      "The agent will stop after the current step.",
      [
        { text: "Keep running", style: "cancel" },
        {
          text: "Cancel task",
          style: "destructive",
          onPress: () => mutation.mutate(taskId),
        },
      ],
    );
  };

  return (
    <Pressable
      onPress={onPress}
      disabled={mutation.isPending}
      className="px-3 py-1.5 rounded-md bg-secondary active:opacity-70"
    >
      <Text className="text-xs font-medium text-foreground">Cancel</Text>
    </Pressable>
  );
}
