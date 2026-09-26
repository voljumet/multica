import { useCallback, useMemo } from "react";
import {
  ActionSheetIOS,
  Alert,
  FlatList,
  View,
} from "react-native";
import { Stack, router } from "expo-router";
import { useQuery } from "@tanstack/react-query";
import { Ionicons } from "@expo/vector-icons";
import type { InboxItem } from "@multica/core/types";
import { Text } from "@/components/ui/text";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { IconButton } from "@/components/ui/icon-button";
import { SwipeableInboxRow } from "@/components/inbox/swipeable-inbox-row";
import { inboxListOptions } from "@/data/queries/inbox";
import {
  useArchiveAllInbox,
  useArchiveAllReadInbox,
  useArchiveCompletedInbox,
  useArchiveInbox,
  useMarkAllInboxRead,
  useMarkInboxRead,
} from "@/data/mutations/inbox";
import { useWorkspaceStore } from "@/data/workspace-store";
import { useColorScheme } from "@/lib/use-color-scheme";
import { THEME } from "@/lib/theme";
import { useT } from "@/lib/i18n";
import {
  deduplicateInboxItems,
  getInboxNavigationTarget,
} from "@/lib/inbox-display";

export default function InboxPage() {
  const { t } = useT("inbox");
  const wsId = useWorkspaceStore((s) => s.currentWorkspaceId);
  const wsSlug = useWorkspaceStore((s) => s.currentWorkspaceSlug);
  const { colorScheme } = useColorScheme();
  const { data: rawItems, isLoading, error, refetch, isRefetching } = useQuery(
    inboxListOptions(wsId),
  );
  // Dedup + drop archived to match web/desktop. See CLAUDE.md
  // "Behavioral parity" → inbox dedup incident.
  const data = useMemo(
    () => deduplicateInboxItems(rawItems ?? []),
    [rawItems],
  );
  const markRead = useMarkInboxRead();
  const markAllRead = useMarkAllInboxRead();
  const archive = useArchiveInbox();
  const archiveAll = useArchiveAllInbox();
  const archiveAllRead = useArchiveAllReadInbox();
  const archiveCompleted = useArchiveCompletedInbox();

  const onPressItem = (item: InboxItem) => {
    if (!item.read) {
      markRead.mutate(item.id);
    }
    const target = getInboxNavigationTarget(item, wsSlug, String(Date.now()));
    if (target) router.push(target);
  };

  const onPressMenu = useCallback(() => {
    const options = [
      t("common:actions.cancel"),
      t("actions.mark_all_read"),
      t("actions.archive_all_read"),
      t("actions.archive_completed"),
      t("actions.archive_all"),
    ];
    ActionSheetIOS.showActionSheetWithOptions(
      {
        options,
        cancelButtonIndex: 0,
        destructiveButtonIndex: 4,
        title: t("title"),
      },
      (i) => {
        if (i === 1) markAllRead.mutate();
        else if (i === 2) archiveAllRead.mutate();
        else if (i === 3) archiveCompleted.mutate();
        else if (i === 4) {
          Alert.alert(
            t("archive.confirm_title"),
            t("archive.confirm_body"),
            [
              { text: t("common:actions.cancel"), style: "cancel" },
              {
                text: t("actions.archive_all"),
                style: "destructive",
                onPress: () => archiveAll.mutate(),
              },
            ],
          );
        }
      },
    );
  }, [markAllRead, archiveAllRead, archiveCompleted, archiveAll]);

  const inboxHeaderRight = useCallback(
    () => (
      <IconButton
        name="ellipsis-horizontal"
        onPress={onPressMenu}
        accessibilityLabel={t("actions.inbox_actions")}
      />
    ),
    [onPressMenu, t],
  );

  return (
    <View className="flex-1 bg-background">
      <Stack.Screen options={{ headerRight: inboxHeaderRight }} />
      {isLoading ? (
        <InboxLoading />
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
      ) : !data || data.length === 0 ? (
        <InboxEmpty iconColor={THEME[colorScheme].mutedForeground} />
      ) : (
        <FlatList
          data={data}
          keyExtractor={(item) => item.id}
          ItemSeparatorComponent={() => (
            <View className="h-px bg-border ml-16" />
          )}
          contentContainerClassName="pb-6"
          renderItem={({ item }) => (
            <SwipeableInboxRow
              item={item}
              onPress={() => onPressItem(item)}
              onArchive={() => archive.mutate(item.id)}
            />
          )}
          refreshing={isRefetching}
          onRefresh={refetch}
        />
      )}
    </View>
  );
}

function InboxLoading() {
  return (
    <View className="px-4 pt-4 gap-4">
      {Array.from({ length: 6 }).map((_, i) => (
        <View key={i} className="flex-row gap-3">
          <Skeleton className="size-9 rounded-full" />
          <View className="flex-1 gap-2 pt-1">
            <Skeleton className="h-3.5 w-3/4" />
            <Skeleton className="h-3 w-1/2" />
          </View>
        </View>
      ))}
    </View>
  );
}

function InboxEmpty({ iconColor }: { iconColor: string }) {
  const { t } = useT("inbox");

  return (
    <View className="flex-1 items-center justify-center px-8 gap-3">
      <Ionicons name="mail-open-outline" size={42} color={iconColor} />
      <Text className="text-base font-medium text-foreground text-center">
        {t("empty.title")}
      </Text>
      <Text className="text-sm text-muted-foreground text-center">
        {t("empty.body")}
      </Text>
    </View>
  );
}
