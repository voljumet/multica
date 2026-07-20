/**
 * Project filter sheet for the workspace-wide Issues tab.
 * Multi-select projects plus an optional "No project" row.
 */
import { useMemo } from "react";
import { Pressable, ScrollView, View } from "react-native";
import { useQuery } from "@tanstack/react-query";
import { Ionicons } from "@expo/vector-icons";
import { Text } from "@/components/ui/text";
import { ProjectIcon } from "@/components/ui/project-icon";
import { projectListOptions } from "@/data/queries/projects";
import { useIssuesViewStore } from "@/data/stores/issues-view-store";
import { useWorkspaceStore } from "@/data/workspace-store";
import { cn } from "@/lib/utils";

export default function IssuesProjectFilterRoute() {
  const wsId = useWorkspaceStore((s) => s.currentWorkspaceId);
  const projectFilters = useIssuesViewStore((s) => s.projectFilters);
  const includeNoProject = useIssuesViewStore((s) => s.includeNoProject);
  const { data: projects = [] } = useQuery(projectListOptions(wsId));

  const sortedProjects = useMemo(
    () => [...projects].sort((a, b) => a.title.localeCompare(b.title)),
    [projects],
  );

  const hasActive = projectFilters.length > 0 || includeNoProject;

  const onClear = () => {
    useIssuesViewStore.setState({
      projectFilters: [],
      includeNoProject: false,
    });
  };

  return (
    <View className="flex-1">
      <View className="flex-row items-center justify-between px-4 pt-4 pb-3">
        <Text className="text-base font-semibold text-foreground">Projects</Text>
        {hasActive ? (
          <Pressable
            onPress={onClear}
            hitSlop={8}
            className="px-2 py-1 active:opacity-60"
          >
            <Text className="text-sm text-primary font-medium">Reset</Text>
          </Pressable>
        ) : null}
      </View>
      <ScrollView className="flex-1" showsVerticalScrollIndicator={false}>
        <Pressable
          onPress={() => useIssuesViewStore.getState().toggleNoProject()}
          className={cn(
            "flex-row items-center gap-3 px-4 py-2.5 active:bg-secondary",
            includeNoProject && "bg-secondary/60",
          )}
        >
          <Ionicons name="folder-outline" size={16} color="#888" />
          <Text className="flex-1 text-sm text-foreground">No project</Text>
          <CheckMark checked={includeNoProject} />
        </Pressable>
        {sortedProjects.map((project) => {
          const checked = projectFilters.includes(project.id);
          return (
            <Pressable
              key={project.id}
              onPress={() =>
                useIssuesViewStore.getState().toggleProjectFilter(project.id)
              }
              className={cn(
                "flex-row items-center gap-3 px-4 py-2.5 active:bg-secondary",
                checked && "bg-secondary/60",
              )}
            >
              <ProjectIcon icon={project.icon} size="sm" />
              <Text className="flex-1 text-sm text-foreground" numberOfLines={1}>
                {project.title}
              </Text>
              <CheckMark checked={checked} />
            </Pressable>
          );
        })}
      </ScrollView>
    </View>
  );
}

function CheckMark({ checked }: { checked: boolean }) {
  if (!checked) return null;
  return <Text className="text-sm text-primary font-semibold">✓</Text>;
}