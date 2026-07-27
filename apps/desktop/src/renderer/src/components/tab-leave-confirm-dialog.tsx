import { useRef, useState } from "react";
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
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Label } from "@multica/ui/components/ui/label";
import { useT } from "@multica/views/i18n";
import { useTabLeaveGuardStore } from "@/platform/tab-leave-guard";

/**
 * Shell-level confirm when leaving the active desktop tab. Mounted once in
 * DesktopShell so every leave path (tab bar, Cmd+W, workspace switch,
 * navigation that activates another tab) shares one dialog.
 */
export function TabLeaveConfirmDialog() {
  const { t } = useT("settings");
  const pending = useTabLeaveGuardStore((s) => s.pending);
  const confirm = useTabLeaveGuardStore((s) => s.confirm);
  const cancel = useTabLeaveGuardStore((s) => s.cancel);
  const [dontAskAgain, setDontAskAgain] = useState(false);
  // Prevent onOpenChange(false) from canceling after a deliberate confirm —
  // Base UI closes the dialog after the action click, which also fires
  // onOpenChange(false). Without this guard, cancel would clear `pending`
  // before confirm runs (or no-op after), depending on event order.
  const confirmingRef = useRef(false);

  const open = pending !== null;

  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (next) return;
        setDontAskAgain(false);
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
        <div className="flex items-center gap-2 px-1">
          <Checkbox
            id="tab-leave-dont-ask"
            checked={dontAskAgain}
            onCheckedChange={(v) => setDontAskAgain(v === true)}
          />
          <Label
            htmlFor="tab-leave-dont-ask"
            className="text-sm font-normal text-muted-foreground"
          >
            {t(($) => $.desktop.tabs.leave_guard.dont_ask_again)}
          </Label>
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel>
            {t(($) => $.desktop.tabs.leave_guard.cancel)}
          </AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            onClick={() => {
              confirmingRef.current = true;
              const skip = dontAskAgain;
              setDontAskAgain(false);
              confirm(skip);
            }}
          >
            {t(($) => $.desktop.tabs.leave_guard.confirm)}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
