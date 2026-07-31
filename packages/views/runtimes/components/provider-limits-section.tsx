"use client";

import { useEffect, useState } from "react";
import { Gauge, Pencil } from "lucide-react";
import { toast } from "sonner";
import type { AgentRuntime } from "@multica/core/types";
import { useWorkspaceId } from "@multica/core/hooks";
import { useUpdateRuntime } from "@multica/core/runtimes/mutations";
import {
  providerAccountLabel,
  providerAccountSubLabel,
  readProviderAccount,
  readProviderAccountDescription,
  readProviderLimits,
  type ProviderLimitWindow,
  type ProviderLimitsSnapshot,
} from "@multica/core/runtimes";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { ProviderLogo } from "./provider-logo";
import { useT, useTimeAgo } from "../../i18n";

function useNowTick(intervalMs = 30_000): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}

/**
 * Per-runtime provider plan limits + account identity.
 *
 * Auto identity comes from the daemon (email / key fingerprint / providers).
 * User can set a free-text description so API-key runtimes stay distinguishable.
 */
export function ProviderLimitsSection({
  runtime,
  canEdit = false,
}: {
  runtime: AgentRuntime;
  canEdit?: boolean;
}) {
  const { t } = useT("runtimes");
  const timeAgo = useTimeAgo();
  const wsId = useWorkspaceId();
  const updateRuntime = useUpdateRuntime(wsId);
  useNowTick(30_000);

  const [editOpen, setEditOpen] = useState(false);
  const [draft, setDraft] = useState("");

  // Cloud workers don't share a local subscription login; hide the card.
  if (runtime.runtime_mode !== "local") {
    return null;
  }

  const snapshot = readProviderLimits(runtime.metadata);
  const account = readProviderAccount(runtime.metadata);
  const description = readProviderAccountDescription(runtime.metadata);
  const primary = providerAccountLabel(account, description);
  const secondary = providerAccountSubLabel(account, description);

  const openEdit = () => {
    setDraft(description ?? "");
    setEditOpen(true);
  };

  const saveDescription = () => {
    const next = draft.trim();
    updateRuntime.mutate(
      {
        runtimeId: runtime.id,
        patch: { provider_account_description: next },
      },
      {
        onSuccess: () => {
          setEditOpen(false);
          toast.success(t(($) => $.provider_limits.description_saved));
        },
        onError: () => {
          toast.error(t(($) => $.provider_limits.description_save_failed));
        },
      },
    );
  };

  return (
    <section className="rounded-lg border bg-card">
      <div className="flex items-center gap-2 border-b px-4 py-3">
        <Gauge className="h-4 w-4 text-muted-foreground" aria-hidden />
        <h3 className="text-sm font-medium">
          {t(($) => $.provider_limits.title)}
        </h3>
        <span className="ml-auto inline-flex min-w-0 max-w-[65%] flex-col items-end gap-0.5 text-xs text-muted-foreground">
          <span className="inline-flex items-center gap-1.5">
            <ProviderLogo
              provider={snapshot?.provider ?? runtime.provider}
              className="h-3.5 w-3.5"
            />
            <span className="capitalize">
              {snapshot?.provider ?? runtime.provider}
            </span>
            {canEdit ? (
              <button
                type="button"
                onClick={openEdit}
                className="rounded p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground"
                aria-label={t(($) => $.provider_limits.edit_description_aria)}
                title={t(($) => $.provider_limits.edit_description_aria)}
              >
                <Pencil className="h-3 w-3" />
              </button>
            ) : null}
          </span>
          {primary ? (
            <span
              className="max-w-full truncate font-mono text-[11px] text-muted-foreground"
              title={secondary ? `${primary} · ${secondary}` : primary}
            >
              {primary}
            </span>
          ) : null}
          {secondary ? (
            <span className="max-w-full truncate text-[10px] text-muted-foreground/80">
              {secondary}
            </span>
          ) : null}
        </span>
      </div>
      <div className="px-4 py-3">
        {snapshot ? (
          <SnapshotBody snapshot={snapshot} timeAgo={timeAgo} />
        ) : (
          <p className="text-sm text-muted-foreground">
            {primary
              ? t(($) => $.provider_limits.empty_with_account, {
                  account: primary,
                })
              : t(($) => $.provider_limits.empty)}
            {canEdit && !description ? (
              <>
                {" "}
                <button
                  type="button"
                  onClick={openEdit}
                  className="text-foreground underline-offset-2 hover:underline"
                >
                  {t(($) => $.provider_limits.add_description)}
                </button>
              </>
            ) : null}
          </p>
        )}
      </div>

      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>
              {t(($) => $.provider_limits.description_dialog_title)}
            </DialogTitle>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">
            {t(($) => $.provider_limits.description_dialog_hint)}
          </p>
          <Input
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            maxLength={120}
            placeholder={t(($) => $.provider_limits.description_placeholder)}
            autoFocus
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                saveDescription();
              }
            }}
          />
          <DialogFooter className="gap-2 sm:gap-0">
            {description ? (
              <Button
                type="button"
                variant="ghost"
                className="mr-auto"
                disabled={updateRuntime.isPending}
                onClick={() => {
                  setDraft("");
                  updateRuntime.mutate(
                    {
                      runtimeId: runtime.id,
                      patch: { provider_account_description: "" },
                    },
                    {
                      onSuccess: () => {
                        setEditOpen(false);
                        toast.success(
                          t(($) => $.provider_limits.description_cleared),
                        );
                      },
                    },
                  );
                }}
              >
                {t(($) => $.provider_limits.clear_description)}
              </Button>
            ) : null}
            <Button
              type="button"
              variant="outline"
              onClick={() => setEditOpen(false)}
            >
              {t(($) => $.provider_limits.cancel)}
            </Button>
            <Button
              type="button"
              disabled={updateRuntime.isPending}
              onClick={saveDescription}
            >
              {t(($) => $.provider_limits.save)}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}

function SnapshotBody({
  snapshot,
  timeAgo,
}: {
  snapshot: ProviderLimitsSnapshot;
  timeAgo: (dateStr: string) => string;
}) {
  const { t } = useT("runtimes");
  const seen = timeAgo(snapshot.observed_at);
  const sourceHint =
    snapshot.source === "task_error"
      ? t(($) => $.provider_limits.source_task_error)
      : t(($) => $.provider_limits.source_probe);

  if (snapshot.status === "exhausted") {
    return (
      <div className="space-y-3">
        <p className="text-sm text-warning">
          {t(($) => $.provider_limits.status_exhausted)}
        </p>
        {snapshot.windows.map((w, i) => (
          <WindowRow
            key={`${w.kind}-${w.resets_label ?? w.resets_at ?? i}`}
            window={w}
          />
        ))}
        <p className="text-xs text-muted-foreground">
          {t(($) => $.provider_limits.seen_hint, {
            when: seen,
            source: sourceHint,
          })}
        </p>
      </div>
    );
  }

  if (snapshot.status === "ok" && snapshot.windows.length > 0) {
    return (
      <div className="space-y-3">
        {snapshot.windows.map((w, i) => (
          <WindowRow
            key={`${w.kind}-${w.resets_label ?? w.resets_at ?? i}`}
            window={w}
          />
        ))}
        <p className="text-xs text-muted-foreground">
          {t(($) => $.provider_limits.seen_hint, {
            when: seen,
            source: sourceHint,
          })}
        </p>
      </div>
    );
  }

  return (
    <p className="text-sm text-muted-foreground">
      {t(($) => $.provider_limits.status_unknown)}
    </p>
  );
}

function WindowRow({ window }: { window: ProviderLimitWindow }) {
  const { t } = useT("runtimes");

  let title = window.label?.trim() ?? "";
  if (!title) {
    switch (window.kind) {
      case "five_hour":
        title = t(($) => $.provider_limits.window_five_hour);
        break;
      case "seven_day":
        title = t(($) => $.provider_limits.window_seven_day);
        break;
      case "opus":
        title = t(($) => $.provider_limits.window_opus);
        break;
      default:
        title = t(($) => $.provider_limits.window_unknown);
    }
  }

  let reset: string | null = null;
  if (window.resets_at) {
    const ms = Date.parse(window.resets_at);
    if (Number.isFinite(ms)) {
      const delta = ms - Date.now();
      if (delta <= 0) {
        reset = t(($) => $.provider_limits.resets_soon);
      } else {
        const mins = Math.round(delta / 60_000);
        if (mins < 60) {
          reset = t(($) => $.provider_limits.resets_in_minutes, {
            count: mins,
          });
        } else {
          const hours = Math.floor(mins / 60);
          const rem = mins % 60;
          reset =
            rem === 0
              ? t(($) => $.provider_limits.resets_in_hours, { count: hours })
              : t(($) => $.provider_limits.resets_in_hours_mins, {
                  hours,
                  minutes: rem,
                });
        }
      }
    }
  } else if (window.resets_label) {
    reset = t(($) => $.provider_limits.resets_at_label, {
      when: window.resets_label,
    });
  }

  const pct =
    window.used_pct != null && Number.isFinite(window.used_pct)
      ? Math.max(0, Math.min(100, Math.round(window.used_pct)))
      : null;

  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-baseline justify-between gap-3">
        <span className="text-sm font-medium text-foreground">{title}</span>
        {pct != null ? (
          <span className="tabular-nums text-sm text-muted-foreground">
            {t(($) => $.provider_limits.used_pct, { pct })}
          </span>
        ) : null}
      </div>
      {pct != null ? (
        <div
          className="h-1.5 overflow-hidden rounded-full bg-muted"
          role="progressbar"
          aria-valuenow={pct}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-label={t(($) => $.provider_limits.used_pct, { pct })}
        >
          <div
            className={`h-full rounded-full transition-[width] ${
              pct >= 90
                ? "bg-destructive"
                : pct >= 70
                  ? "bg-warning"
                  : "bg-foreground/70"
            }`}
            style={{ width: `${pct}%` }}
          />
        </div>
      ) : null}
      {reset ? (
        <p className="text-xs text-muted-foreground">{reset}</p>
      ) : null}
    </div>
  );
}
