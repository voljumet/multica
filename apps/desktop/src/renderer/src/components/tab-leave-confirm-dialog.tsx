import { useRef } from "react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@multica/ui/components/ui/alert-dialog";
import { useT } from "@multica/views/i18n";
import { useTabLeaveGuardStore } from "@/platform/tab-leave-guard";

export function TabLeaveConfirmDialog() {
  const { t } = useT("settings");
  const pending = useTabLeaveGuardStore((s) => s.pending);
  const confirm = useTabLeaveGuardStore((s) => s.confirm);
  const cancel = useTabLeaveGuardStore((s) => s.cancel);
  // Prevent onOpenChange(false) from canceling after a deliberate confirm.
  const confirmingRef = useRef(false);

  const open = pending !== null;

  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (next) return;
        if (confirmingRef.current) {
          confirmingRef.current = false;
          return;
        }
        cancel();
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {t(($) => $.desktop.tabs.leave_guard.title)}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t(($) => $.desktop.tabs.leave_guard.description)}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>
            {t(($) => $.desktop.tabs.leave_guard.cancel)}
          </AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            onClick={() => {
              confirmingRef.current = true;
              confirm();
            }}
          >
            {t(($) => $.desktop.tabs.leave_guard.confirm)}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
