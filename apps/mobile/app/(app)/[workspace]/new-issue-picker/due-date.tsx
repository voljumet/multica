/**
 * Due-date picker route for the in-progress new-issue draft. See ./status.tsx.
 *
 * Same Done / Clear pattern as the issue-detail variant
 * (`issue/[id]/picker/due-date.tsx`) — UIDatePicker doesn't auto-commit, so
 * the route renders a slim action row below the native nav header.
 */
import { useRef } from "react";
import { Pressable, View } from "react-native";
import { router } from "expo-router";
import { Text } from "@/components/ui/text";
import {
  DueDatePickerBody,
  type DueDatePickerBodyHandle,
} from "@/components/issue/pickers/due-date-picker-body";
import { useNewIssueDraftStore } from "@/data/stores/new-issue-draft-store";

export default function NewIssueDueDatePickerRoute() {
  const dueDate = useNewIssueDraftStore((s) => s.dueDate);
  const setDueDate = useNewIssueDraftStore((s) => s.setDueDate);
  const ref = useRef<DueDatePickerBodyHandle>(null);

  return (
    <View className="flex-1">
      <View className="flex-row items-center justify-end px-4 pt-4 pb-2">
        <View className="flex-row items-center gap-1">
          {dueDate ? (
            <Pressable
              onPress={() => {
                setDueDate(null);
                router.back();
              }}
              hitSlop={6}
              className="px-2 py-1 rounded-md active:bg-secondary"
            >
              <Text className="text-sm text-destructive">Clear</Text>
            </Pressable>
          ) : null}
          <Pressable
            onPress={() => {
              const iso = ref.current?.getIso();
              if (iso) setDueDate(iso);
              router.back();
            }}
            hitSlop={6}
            className="px-2 py-1 rounded-md active:bg-secondary"
          >
            <Text className="text-sm font-medium text-primary">Done</Text>
          </Pressable>
        </View>
      </View>
      <DueDatePickerBody ref={ref} value={dueDate} />
    </View>
  );
}
