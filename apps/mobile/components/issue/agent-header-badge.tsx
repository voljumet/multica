/**
 * Ambient status / runs entry for the issue detail Stack header (right
 * side). Two states:
 *
 *   ≥1 active task   → [agent avatars] (pulse)  — "something is working"
 *   0 active, ≥1 past → scroll-text icon        — open historical runs
 *   never run        → null
 *
 * Why this exists: the in-card `<AgentActivityRow>` is the first-time-
 * discovery surface (full "Working" / "Runs · N" text + larger avatars),
 * but it scrolls away with the timeline. Agent tasks run for minutes to
 * tens of minutes; users actively scroll during that window to read past
 * comments. After a run finishes the pulse badge used to disappear, which
 * left no ambient way to re-open the log on completed issues — the header
 * entry must stay available whenever any runs exist.
 *
 * Tap always pushes the `issue/[id]/runs` formSheet route — the in-card
 * AgentActivityRow does the same. One route, two entry points, no
 * duplicate sheet state.
 */
import { useMemo } from "react";
import { Pressable } from "react-native";
import { router } from "expo-router";
import { useQuery } from "@tanstack/react-query";
import { Ionicons } from "@expo/vector-icons";
import { AvatarStack, type StackActor } from "@/components/ui/avatar-stack";
import { PulseDot } from "@/components/ui/pulse-dot";
import {
  issueActiveTasksOptions,
  issueTasksOptions,
} from "@/data/queries/issues";
import { useWorkspaceStore } from "@/data/workspace-store";
import { useColorScheme } from "@/lib/use-color-scheme";
import { THEME } from "@/lib/theme";

interface Props {
  issueId: string;
}

export function AgentHeaderBadge({ issueId }: Props) {
  const wsId = useWorkspaceStore((s) => s.currentWorkspaceId);
  const wsSlug = useWorkspaceStore((s) => s.currentWorkspaceSlug);
  const { colorScheme } = useColorScheme();
  const mutedFg = THEME[colorScheme].mutedForeground;

  const { data: active = [] } = useQuery(
    issueActiveTasksOptions(wsId, issueId),
  );
  const { data: allTasks = [] } = useQuery(issueTasksOptions(wsId, issueId));

  const pastCount = useMemo(
    () =>
      allTasks.filter(
        (t) =>
          t.status === "completed" ||
          t.status === "failed" ||
          t.status === "cancelled",
      ).length,
    [allTasks],
  );

  if (active.length === 0 && pastCount === 0) return null;

  const openRuns = () => {
    if (!wsSlug) return;
    router.push({
      pathname: "/[workspace]/issue/[id]/runs",
      params: { workspace: wsSlug, id: issueId },
    });
  };

  if (active.length > 0) {
    const actors = active.map<StackActor>((t) => ({
      type: "agent",
      id: t.agent_id,
    }));

    return (
      <Pressable
        onPress={openRuns}
        hitSlop={8}
        accessibilityLabel="Agent working — open runs"
        className="flex-row items-center gap-1.5 px-2 py-1 active:opacity-60"
      >
        <AvatarStack actors={actors} max={2} size={20} />
        <PulseDot size={6} />
      </Pressable>
    );
  }

  return (
    <Pressable
      onPress={openRuns}
      hitSlop={8}
      accessibilityLabel={`Open agent runs · ${pastCount}`}
      className="px-2 py-1 active:opacity-60"
    >
      <Ionicons name="document-text-outline" size={20} color={mutedFg} />
    </Pressable>
  );
}
