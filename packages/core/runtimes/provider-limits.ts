/**
 * Provider plan-limit snapshot stored at agent_runtime.metadata.provider_limits.
 *
 * Written by the server when a task fails with a recognized Claude subscription
 * plan-limit message (session / weekly / Opus). A future daemon probe will
 * reuse the same shape with source "probe" and non-null used_pct.
 *
 * Parse defensively: older backends omit the key; malformed JSON on the wire
 * is treated as absent (API compatibility rules).
 */

export type ProviderLimitsStatus =
  | "ok"
  | "exhausted"
  | "unknown"
  | "unsupported";

export type ProviderLimitsSource = "task_error" | "probe";

export type ProviderLimitWindowKind =
  | "five_hour"
  | "seven_day"
  | "opus"
  | "unknown";

export interface ProviderLimitWindow {
  kind: ProviderLimitWindowKind;
  /** 0–100 when known (probe path); null on task_error captures. */
  used_pct: number | null;
  /** Absolute reset instant (ISO) when known. */
  resets_at: string | null;
  /** Raw reset phrase from the provider message, e.g. "3:45pm". */
  resets_label?: string | null;
  /** Short human label, e.g. "session limit". */
  label?: string;
}

export interface ProviderLimitsSnapshot {
  provider: string;
  status: ProviderLimitsStatus;
  source: ProviderLimitsSource;
  /** ISO timestamp of the observation. */
  observed_at: string;
  windows: ProviderLimitWindow[];
  message?: string;
}

const STATUSES = new Set<string>([
  "ok",
  "exhausted",
  "unknown",
  "unsupported",
]);
const SOURCES = new Set<string>(["task_error", "probe"]);
const KINDS = new Set<string>([
  "five_hour",
  "seven_day",
  "opus",
  "unknown",
]);

function asString(v: unknown): string | null {
  return typeof v === "string" && v.length > 0 ? v : null;
}

function asNumberOrNull(v: unknown): number | null {
  if (v === null || v === undefined) return null;
  if (typeof v !== "number" || !Number.isFinite(v)) return null;
  return v;
}

function parseWindow(raw: unknown): ProviderLimitWindow | null {
  if (!raw || typeof raw !== "object") return null;
  const o = raw as Record<string, unknown>;
  const kindRaw = asString(o.kind) ?? "unknown";
  const kind = (KINDS.has(kindRaw) ? kindRaw : "unknown") as ProviderLimitWindowKind;
  return {
    kind,
    used_pct: asNumberOrNull(o.used_pct),
    resets_at: asString(o.resets_at),
    resets_label: asString(o.resets_label),
    label: asString(o.label) ?? undefined,
  };
}

/**
 * Extract a ProviderLimitsSnapshot from runtime.metadata, or null when
 * absent / unusable.
 */
export function readProviderLimits(
  metadata: Record<string, unknown> | null | undefined,
): ProviderLimitsSnapshot | null {
  if (!metadata || typeof metadata !== "object") return null;
  const raw = metadata.provider_limits;
  if (!raw || typeof raw !== "object") return null;
  const o = raw as Record<string, unknown>;

  const provider = asString(o.provider) ?? "unknown";
  const statusRaw = asString(o.status) ?? "unknown";
  const status = (STATUSES.has(statusRaw)
    ? statusRaw
    : "unknown") as ProviderLimitsStatus;
  const sourceRaw = asString(o.source) ?? "task_error";
  const source = (SOURCES.has(sourceRaw)
    ? sourceRaw
    : "task_error") as ProviderLimitsSource;
  const observed_at = asString(o.observed_at);
  if (!observed_at) return null;

  const windowsRaw = Array.isArray(o.windows) ? o.windows : [];
  const windows: ProviderLimitWindow[] = [];
  for (const w of windowsRaw) {
    const parsed = parseWindow(w);
    if (parsed) windows.push(parsed);
  }

  return {
    provider,
    status,
    source,
    observed_at,
    windows,
    message: asString(o.message) ?? undefined,
  };
}
