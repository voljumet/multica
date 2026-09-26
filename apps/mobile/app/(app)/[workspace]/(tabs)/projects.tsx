/**
 * Projects tab. Moved from more/projects.tsx; header is now the in-body
 * <Header> component since tab roots have headerShown: false.
 */
import { useCallback, useMemo } from "react";
import {
  ActivityIndicator,
  FlatList,
  RefreshControl,
  View,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useQuery } from "@tanstack/react-query";
import { router } from "expo-router";
import { Text } from "@/components/ui/text";
import { Button } from "@/components/ui/button";
import { IconButton } from "@/components/ui/icon-button";
import { Header } from "@/components/ui/header";
import { HeaderActions } from "@/components/ui/app-header-actions";
import { ProjectRow } from "@/components/project/project-row";
import { projectListOptions } from "@/data/queries/projects";
import { useWorkspaceStore } from "@/data/workspace-store";
import { useT } from "@/lib/i18n";

export default function ProjectsTab() {
  const wsId = useWorkspaceStore((s) => s.currentWorkspaceId);
  const wsSlug = useWorkspaceStore((s) => s.currentWorkspaceSlug);
  const { t } = useT("projects");

  const { data, isLoading, error, refetch, isRefetching } = useQuery(
    projectListOptions(wsId),
  );

  const sorted = useMemo(() => {
    if (!data) return [];
    return [...data].sort(
      (a, b) =>
        new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime(),
    );
  }, [data]);

  const goCreate = useCallback(() => {
    if (wsSlug) router.push(`/${wsSlug}/project/new`);
  }, [wsSlug]);

  return (
    <SafeAreaView className="flex-1 bg-background" edges={[]}>
      <Header
        title="Projects"
        right={
          <>
            <PlusButton onPress={goCreate} />
            <HeaderActions />
          </>
        }
      />
      {isLoading ? (
        <View className="flex-1 items-center justify-center">
          <ActivityIndicator />
        </View>
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
      ) : sorted.length === 0 ? (
        <EmptyState onCreate={goCreate} />
      ) : (
        <FlatList
          data={sorted}
          keyExtractor={(item) => item.id}
          ItemSeparatorComponent={() => (
            <View className="h-px bg-border ml-4" />
          )}
          renderItem={({ item }) => (
            <ProjectRow
              project={item}
              onPress={() => {
                if (wsSlug) router.push(`/${wsSlug}/project/${item.id}`);
              }}
            />
          )}
          refreshControl={
            <RefreshControl refreshing={isRefetching} onRefresh={refetch} />
          }
          contentContainerClassName="pb-6"
        />
      )}
    </SafeAreaView>
  );
}

function PlusButton({ onPress }: { onPress: () => void }) {
  const { t } = useT("projects");
  return (
    <IconButton
      name="add"
      onPress={onPress}
      accessibilityLabel={t("navigation:routes.new_project")}
    />
  );
}

function EmptyState({ onCreate }: { onCreate: () => void }) {
  const { t } = useT("projects");
  return (
    <View className="flex-1 items-center justify-center px-6 gap-4">
      <Text className="text-base font-medium text-foreground">
        {t("list.empty")}
      </Text>
      <Button variant="default" onPress={onCreate}>
        <Text>{t("common:actions.create")}</Text>
      </Button>
    </View>
  );
}
