/** Project issues use concrete status sections, like the other issue lists. */
import { useMemo } from "react";
import { Pressable, View } from "react-native";
import { Image as ExpoImage } from "expo-image";
import { useQuery } from "@tanstack/react-query";
import { router } from "expo-router";
import type { IssueStatus } from "@multica/core/types";
import { Text } from "@/components/ui/text";
import { Button } from "@/components/ui/button";
import { StatusIcon } from "@/components/ui/status-icon";
import { IssueRow } from "@/components/issue/issue-row";
import { IssuesLoading } from "@/components/issue/issues-loading";
import { projectIssuesOptions } from "@/data/queries/projects";
import { useWorkspaceStore } from "@/data/workspace-store";
import { useProjectCollapseStore } from "@/data/stores/project-collapse-store";
import { useColorScheme } from "@/lib/use-color-scheme";
import { THEME } from "@/lib/theme";
import { groupIssuesByStatus } from "@/lib/group-issues-by-status";
import { useIssueStatuses } from "@/lib/use-issue-statuses";
import { useT } from "@/lib/i18n";

interface Props {
  projectId: string;
}

export function ProjectRelatedIssues({ projectId }: Props) {
  const wsId = useWorkspaceStore((s) => s.currentWorkspaceId);
  const wsSlug = useWorkspaceStore((s) => s.currentWorkspaceSlug);
  const { t } = useT("issues");
  const { data, isLoading, error, refetch } = useQuery(
    projectIssuesOptions(wsId, projectId),
  );
  const toggle = useProjectCollapseStore((s) => s.toggle);
  const collapsedByProject = useProjectCollapseStore((s) => s.collapsedByProject);

  const catalog = useIssueStatuses();
  const sections = useMemo(() => groupIssuesByStatus(data ?? [], catalog.statuses), [data, catalog.statuses]);

  const navigateToIssue = (id: string) => {
    if (wsSlug) router.push(`/${wsSlug}/issue/${id}`);
  };

  if (isLoading) return <IssuesLoading />;

  if (error) {
    return (
      <View className="px-4 py-6 gap-3">
        <Text className="text-sm text-destructive">
          {t("errors.load_failed", {
            message: error instanceof Error ? error.message : "unknown",
          })}
        </Text>
        <Button variant="outline" onPress={() => refetch()}>
          <Text>{t("common:actions.retry")}</Text>
        </Button>
      </View>
    );
  }

  if ((data?.length ?? 0) === 0) {
    return (
      <View className="px-4 py-6">
        <Text className="text-sm text-muted-foreground">{t("list.empty")}</Text>
      </View>
    );
  }

  return (
    <View>
      {sections.map(({ status, data: issues }) => {
        if (issues.length === 0) return null;
        const collapsed = (collapsedByProject[projectId] ?? []).includes(status);
        return (
          <View key={status}>
            <SectionHeader
              status={status}
              count={issues.length}
              collapsed={collapsed}
              onToggle={() => toggle(projectId, status)}
            />
            {!collapsed &&
              issues.map((issue) => (
                <IssueRow
                  key={issue.id}
                  issue={issue}
                  onPress={() => navigateToIssue(issue.id)}
                />
              ))}
          </View>
        );
      })}
    </View>
  );
}

function SectionHeader({
  status,
  count,
  collapsed,
  onToggle,
}: {
  status: IssueStatus;
  count: number;
  collapsed: boolean;
  onToggle: () => void;
}) {
  const { colorScheme } = useColorScheme();
  const t = THEME[colorScheme];
  const catalog = useIssueStatuses();
  return (
    <Pressable
      onPress={onToggle}
      className="flex-row items-center gap-2 px-4 py-2 bg-background active:bg-secondary"
      accessibilityRole="button"
      accessibilityLabel={`${catalog.labelOf(status)}, ${count} issues, ${collapsed ? "collapsed" : "expanded"}`}
    >
      <StatusIcon
        status={status}
        category={catalog.categoryOf(status)}
        icon={catalog.iconOf(status)}
        color={catalog.colorOf(status)}
        size={14}
      />
      <Text className="flex-1 text-xs uppercase tracking-wider text-muted-foreground font-medium">
        {catalog.labelOf(status)}
      </Text>
      <Text className="text-xs text-muted-foreground/60 mr-1">{count}</Text>
      <ExpoImage
        source="sf:chevron.right"
        tintColor={t.mutedForeground}
        style={{
          width: 12,
          height: 12,
          transform: [{ rotate: collapsed ? "0deg" : "90deg" }],
        }}
      />
    </Pressable>
  );
}
