/**
 * Workspace-wide Issues tab. Moved from more/issues.tsx; header is now
 * the in-body <Header> component since tab roots have headerShown: false.
 *
 * Mirrors web `packages/views/issues/components/issues-page.tsx:32-94`:
 * fetch every issue in the workspace, expose `all / members / agents`
 * scope tabs, group by status, allow status + priority filtering.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ActivityIndicator, Pressable, SectionList, View } from "react-native";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import type { Issue, IssuePriority, IssueStatus } from "@multica/core/types";
import { Text } from "@/components/ui/text";
import { Button } from "@/components/ui/button";
import { Header } from "@/components/ui/header";
import { HeaderActions } from "@/components/ui/app-header-actions";
import { StatusIcon } from "@/components/ui/status-icon";
import { IssueRow } from "@/components/issue/issue-row";
import { IssuesLoading } from "@/components/issue/issues-loading";
import { issueListOptions, issueKeys } from "@/data/queries/issues";
import { api } from "@/data/api";
import { projectListOptions } from "@/data/queries/projects";
import { useWorkspaceStore } from "@/data/workspace-store";
import {
  useIssuesViewStore,
  type IssuesScope,
} from "@/data/stores/issues-view-store";
import { useClearFiltersOnWorkspaceChange } from "@/lib/use-clear-filters-on-workspace-change";
import { PRIORITY_LABEL } from "@/lib/issue-status";
import { useT } from "@/lib/i18n";
import { useIssueStatuses } from "@/lib/use-issue-statuses";
import { groupIssuesByStatus } from "@/lib/group-issues-by-status";
import { filterIssues } from "@/lib/filter-issues";
import { useColorScheme } from "@/lib/use-color-scheme";
import { THEME } from "@/lib/theme";

// Scope tab definitions. Mirrors web `issuesScopeStore`. Counts are NOT
// rendered on the pill labels — web's `IssuesHeader` doesn't show them
// either, and on SE3 (375pt) "(123)" appended to each label pushes the
// row past the safe width when filter icon shares the row. Per-status
// counts still appear on the SectionList headers below.
const SCOPES: IssuesScope[] = ["all", "members", "agents"];

const PAGE_SIZE = 100;

export default function IssuesTab() {
  const wsId = useWorkspaceStore((s) => s.currentWorkspaceId);
  const wsSlug = useWorkspaceStore((s) => s.currentWorkspaceSlug);
  const qc = useQueryClient();
  const { t } = useT("issues");

  const scope = useIssuesViewStore((s) => s.scope);
  const setScope = useIssuesViewStore((s) => s.setScope);
  const statusFilters = useIssuesViewStore((s) => s.statusFilters);
  const priorityFilters = useIssuesViewStore((s) => s.priorityFilters);
  const sortByLastEdited = useIssuesViewStore((s) => s.sortByLastEdited);
  const collapsedStatuses = useIssuesViewStore((s) => s.collapsedStatuses);
  const toggleStatusCollapse = useIssuesViewStore((s) => s.toggleStatusCollapse);

  const openFilter = () => {
    if (!wsSlug) return;
    router.push({
      pathname: "/[workspace]/issues-filter",
      params: { workspace: wsSlug, scope: "all" },
    });
  };

  useClearFiltersOnWorkspaceChange(
    useIssuesViewStore.getState().clearFilters,
    wsId,
  );

  const { data, isLoading, error, refetch, isRefetching } = useQuery(
    issueListOptions(wsId),
  );

  // Only the active-filter chips need the catalog — sections group on the
  // category the server already resolved onto each issue. (MUL-6243)
  const catalog = useIssueStatuses();

  const [isFetchingMore, setIsFetchingMore] = useState(false);
  // true until a page comes back with fewer than PAGE_SIZE rows
  const hasMore = useRef(true);

  // Reset hasMore when the workspace changes or data is refreshed from scratch
  useEffect(() => {
    hasMore.current = true;
  }, [wsId]);

  const fetchMore = useCallback(async () => {
    if (!wsId || !hasMore.current || isFetchingMore) return;
    const current = qc.getQueryData<Issue[]>(issueKeys.list(wsId));
    const offset = current?.length ?? 0;
    setIsFetchingMore(true);
    try {
      const res = await api.listIssues({ offset });
      if (res.issues.length < PAGE_SIZE) hasMore.current = false;
      if (res.issues.length > 0) {
        qc.setQueryData<Issue[]>(issueKeys.list(wsId), (old) =>
          old ? [...old, ...res.issues] : res.issues,
        );
      }
    } finally {
      setIsFetchingMore(false);
    }
  }, [wsId, isFetchingMore, qc]);

  const scopedIssues = useMemo(() => {
    const allIssues = data ?? [];
    if (scope === "members") {
      return allIssues.filter((i) => i.assignee_type === "member");
    }
    if (scope === "agents") {
      return allIssues.filter(
        (i) => i.assignee_type === "agent" || i.assignee_type === "squad",
      );
    }
    return allIssues;
  }, [data, scope]);

  const filtered = useMemo(() => {
    const f = filterIssues(scopedIssues, statusFilters, priorityFilters);
    if (!sortByLastEdited) return f;
    return [...f].sort((a, b) => b.updated_at.localeCompare(a.updated_at));
  }, [scopedIssues, statusFilters, priorityFilters, sortByLastEdited]);

  const sections = useMemo(
    () => groupIssuesByStatus(filtered, catalog.statuses),
    [filtered, catalog.statuses],
  );

  const displaySections = useMemo(
    () =>
      sections.map((s) => ({
        ...s,
        data: collapsedStatuses.includes(s.status) ? ([] as Issue[]) : s.data,
      })),
    [sections, collapsedStatuses],
  );

  const hasActiveFilters =
    statusFilters.length > 0 || priorityFilters.length > 0 || sortByLastEdited;
  const scopeItems = SCOPES.map((value) => ({
    value,
    label: t(`tabs.${value}`),
  }));

  const showEmptyState = !isLoading && !error && filtered.length === 0;

  return (
    <View className="flex-1 bg-background">
      <Header title="Issues" right={<HeaderActions />} />
      <ScopeToolbar
        scopes={scopeItems}
        scope={scope}
        onChange={(v) => setScope(v)}
        onOpenFilter={openFilter}
        hasActiveFilters={hasActiveFilters}
      />
      {hasActiveFilters ? (
        <ActiveFilterChips
          statusFilters={statusFilters}
          priorityFilters={priorityFilters}
          statusLabelOf={catalog.labelOf}
          onClearStatus={(s) =>
            useIssuesViewStore.getState().toggleStatusFilter(s)
          }
          onClearPriority={(p) =>
            useIssuesViewStore.getState().togglePriorityFilter(p)
          }
        />
      ) : null}
      {isLoading ? (
        <IssuesLoading />
      ) : error ? (
        <View className="px-4 gap-3 pt-4">
          <Text className="text-sm text-destructive">
            {t("errors.load_failed", {
              message: error instanceof Error ? error.message : "unknown",
            })}
          </Text>
          <Button variant="outline" onPress={() => refetch()}>
            <Text>{t("common:actions.retry")}</Text>
          </Button>
        </View>
      ) : showEmptyState ? (
        <EmptyState
          message={
            hasActiveFilters
              ? t("empty.filtered")
              : t(`empty.${scope}`)
          }
        />
      ) : (
        <SectionList
          sections={displaySections}
          keyExtractor={(item) => item.id}
          stickySectionHeadersEnabled={false}
          ItemSeparatorComponent={() => (
            <View className="h-px bg-border ml-4" />
          )}
          renderSectionHeader={({ section }) => (
            <SectionHeader
              status={section.status}
              count={section.data.length}
              collapsed={collapsedStatuses.includes(section.status)}
              onToggle={() => toggleStatusCollapse(section.status)}
            />
          )}
          contentContainerClassName="pb-6"
          renderItem={({ item }) => (
            <IssueRow
              issue={item}
              onPress={() => {
                if (wsSlug) router.push(`/${wsSlug}/issue/${item.id}`);
              }}
            />
          )}
          refreshing={isRefetching}
          onRefresh={() => {
            hasMore.current = true;
            refetch();
          }}
          onEndReached={fetchMore}
          onEndReachedThreshold={0.3}
          ListFooterComponent={
            isFetchingMore ? (
              <View className="py-4 items-center">
                <ActivityIndicator />
              </View>
            ) : null
          }
        />
      )}
    </View>
  );
}

function FilterButton({
  onPress,
  hasActiveFilters,
}: {
  onPress: () => void;
  hasActiveFilters: boolean;
}) {
  const { t } = useT("issues");
  const { colorScheme } = useColorScheme();
  return (
    <View style={{ position: "relative" }} className="ml-2">
      <Button
        variant="outline"
        size="sm"
        onPress={onPress}
        accessibilityLabel={t("filters.title")}
        className="w-9 px-0"
      >
        <Ionicons
          name="options-outline"
          size={16}
          color={THEME[colorScheme].mutedForeground}
        />
      </Button>
      {hasActiveFilters ? (
        <View
          pointerEvents="none"
          className="absolute top-1 right-1 size-1.5 rounded-full bg-brand"
        />
      ) : null}
    </View>
  );
}

function ScopeToolbar<S extends string>({
  scopes,
  scope,
  onChange,
  onOpenFilter,
  hasActiveFilters,
}: {
  scopes: { value: S; label: string }[];
  scope: S;
  onChange: (value: S) => void;
  onOpenFilter: () => void;
  hasActiveFilters: boolean;
}) {
  return (
    <View className="flex-row items-center justify-between px-4 pt-2 pb-2">
      <View className="flex-row items-center gap-1 flex-shrink min-w-0">
        {scopes.map((s) => {
          const active = scope === s.value;
          return (
            <Button
              key={s.value}
              variant="outline"
              size="sm"
              onPress={() => onChange(s.value)}
              className={active ? "bg-accent" : ""}
              accessibilityState={{ selected: active }}
            >
              <Text
                numberOfLines={1}
                className={active ? "text-accent-foreground" : "text-muted-foreground"}
              >
                {s.label}
              </Text>
            </Button>
          );
        })}
      </View>
      <FilterButton
        onPress={onOpenFilter}
        hasActiveFilters={hasActiveFilters}
      />
    </View>
  );
}

function ActiveFilterChips({
  statusFilters,
  priorityFilters,
  statusLabelOf,
  onClearStatus,
  onClearPriority,
}: {
  statusFilters: IssueStatus[];
  priorityFilters: IssuePriority[];
  /** Resolves a status KEY — which can be a custom one — to its label. */
  statusLabelOf: (statusKey: string) => string;
  onClearStatus: (s: IssueStatus) => void;
  onClearPriority: (p: IssuePriority) => void;
}) {
  const { t } = useT("issues");
  return (
    <View className="flex-row flex-wrap gap-1.5 px-4 pb-2">
      {statusFilters.map((s) => (
        <Chip
          key={`s-${s}`}
          label={statusLabelOf(s)}
          onClear={() => onClearStatus(s)}
        />
      ))}
      {priorityFilters.map((p) => (
        <Chip
          key={`p-${p}`}
          label={t(PRIORITY_LABEL[p])}
          onClear={() => onClearPriority(p)}
        />
      ))}
    </View>
  );
}

function Chip({ label, onClear }: { label: string; onClear: () => void }) {
  const { colorScheme } = useColorScheme();
  return (
    <Pressable
      onPress={onClear}
      className="flex-row items-center gap-1 pl-2.5 pr-2 py-1 rounded-full border border-border bg-secondary/40 active:bg-secondary"
    >
      <Text className="text-xs text-foreground">{label}</Text>
      <Ionicons
        name="close"
        size={12}
        color={THEME[colorScheme].mutedForeground}
      />
    </Pressable>
  );
}

// The section header names its concrete built-in or custom status.
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
      {/* Category keys resolve to their canonical lifecycle glyph. */}
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
      <Ionicons
        name={collapsed ? "chevron-forward" : "chevron-down"}
        size={12}
        color={t.mutedForeground}
      />
    </Pressable>
  );
}

function EmptyState({ message }: { message: string }) {
  return (
    <View className="flex-1 items-center justify-center px-6">
      <Text className="text-sm text-muted-foreground text-center">
        {message}
      </Text>
    </View>
  );
}
