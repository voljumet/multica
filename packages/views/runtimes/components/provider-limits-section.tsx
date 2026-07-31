"use client";

import { useEffect, useState } from "react";
import { Gauge } from "lucide-react";
import type { AgentRuntime } from "@multica/core/types";
import {
  readProviderLimits,
  type ProviderLimitWindow,
  type ProviderLimitsSnapshot,
} from "@multica/core/runtimes";
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
 * Per-runtime provider plan limits (Claude 5h / weekly / Opus).
 *
 * Slice 1 only shows data when a task failure stamped metadata.provider_limits.
 * Empty state is intentional — we do not invent limits from Multica token usage.
 */
export function ProviderLimitsSection({ runtime }: { runtime: AgentRuntime }) {
  const { t } = useT("runtimes");
  const timeAgo = useTimeAgo();
  // Tick so relative "seen" / countdown labels stay honest while the page is open.
  useNowTick(30_000);

  // Cloud workers don't share a local subscription login; hide the card.
  if (runtime.runtime_mode !== "local") {
    return null;
  }

  const snapshot = readProviderLimits(runtime.metadata);

  return (
    <section className="rounded-lg border bg-card">
      <div className="flex items-center gap-2 border-b px-4 py-3">
        <Gauge className="h-4 w-4 text-muted-foreground" aria-hidden />
        <h3 className="text-sm font-medium">
          {t(($) => $.provider_limits.title)}
        </h3>
        <span className="ml-auto inline-flex items-center gap-1.5 text-xs text-muted-foreground">
          <ProviderLogo
            provider={snapshot?.provider ?? runtime.provider}
            className="h-3.5 w-3.5"
          />
          <span className="capitalize">
            {snapshot?.provider ?? runtime.provider}
          </span>
        </span>
      </div>
      <div className="px-4 py-3">
        {snapshot ? (
          <SnapshotBody snapshot={snapshot} timeAgo={timeAgo} />
        ) : (
          <p className="text-sm text-muted-foreground">
            {t(($) => $.provider_limits.empty)}
          </p>
        )}
      </div>
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
