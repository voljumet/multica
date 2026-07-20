/**
 * Workspace switcher — presented as a formSheet by the parent Stack.
 *
 * Reached from the More popover's WorkspaceCard (collapsed single-row entry).
 * Lists every workspace the user belongs to, current one disabled with a
 * checkmark. Tapping a non-current row triggers an iOS-native `Alert.alert`
 * confirm — only after the user confirms do we dismiss the sheet and navigate
 * to the target workspace.
 */
import {
  ActivityIndicator,
  Alert,
  InteractionManager,
  Pressable,
  ScrollView,
  View,
} from "react-native";
import { Image as ExpoImage } from "expo-image";
import { router } from "expo-router";
import { useQuery } from "@tanstack/react-query";
import type { Workspace } from "@multica/core/types";
import { Text } from "@/components/ui/text";
import { WorkspaceAvatar } from "@/components/workspace/workspace-avatar";
import { workspaceListOptions } from "@/data/queries/workspaces";
import { useWorkspaceStore } from "@/data/workspace-store";
import { useColorScheme } from "@/lib/use-color-scheme";
import { THEME } from "@/lib/theme";
import { cn } from "@/lib/utils";

export default function SwitchWorkspaceRoute() {
  const activeSlug = useWorkspaceStore((s) => s.currentWorkspaceSlug);
  const setCurrentWorkspace = useWorkspaceStore((s) => s.setCurrentWorkspace);
  const { colorScheme } = useColorScheme();
  const t = THEME[colorScheme];
  const { data, isLoading } = useQuery(workspaceListOptions());

  const onSelect = (ws: Workspace) => {
    if (ws.slug === activeSlug) return;
    Alert.alert(
      "Switch workspace",
      `Switch to "${ws.name}"?`,
      [
        { text: "Cancel", style: "cancel" },
        {
          text: "Switch",
          onPress: () => {
            void (async () => {
              await setCurrentWorkspace(ws.id, ws.slug);
              router.dismiss();
              InteractionManager.runAfterInteractions(() => {
                router.replace(`/${ws.slug}/issues`);
              });
            })();
          },
        },
      ],
    );
  };

  return (
    <View className="flex-1">
      <View className="px-4 pt-4 pb-3">
        <Text className="text-base font-semibold text-foreground">
          Switch workspace
        </Text>
      </View>
      {isLoading ? (
        <View className="py-6 items-center">
          <ActivityIndicator />
        </View>
      ) : (
        <ScrollView className="flex-1" showsVerticalScrollIndicator={false}>
          {(data ?? []).map((ws) => (
            <WorkspaceRow
              key={ws.id}
              workspace={ws}
              active={ws.slug === activeSlug}
              onPress={() => onSelect(ws)}
              iconTint={t.foreground}
            />
          ))}
        </ScrollView>
      )}
    </View>
  );
}

function WorkspaceRow({
  workspace,
  active,
  onPress,
  iconTint,
}: {
  workspace: Workspace;
  active: boolean;
  onPress: () => void;
  iconTint: string;
}) {
  return (
    <Pressable
      onPress={onPress}
      disabled={active}
      accessibilityLabel={
        active
          ? `${workspace.name}, current workspace`
          : `Switch to ${workspace.name}`
      }
      className={cn(
        "flex-row items-center gap-3 px-4 py-3 active:bg-secondary",
        active && "opacity-100",
      )}
    >
      <WorkspaceAvatar
        name={workspace.name}
        avatarUrl={workspace.avatar_url}
        size={24}
      />
      <Text
        className={cn(
          "flex-1 text-sm text-foreground",
          active && "font-semibold",
        )}
        numberOfLines={1}
      >
        {workspace.name}
      </Text>
      {active ? (
        <ExpoImage
          source="sf:checkmark"
          tintColor={iconTint}
          style={{ width: 16, height: 16 }}
        />
      ) : null}
    </Pressable>
  );
}