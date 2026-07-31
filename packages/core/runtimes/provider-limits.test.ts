import { describe, expect, it } from "vitest";
import { readProviderLimits } from "./provider-limits";

describe("readProviderLimits", () => {
  it("returns null for missing or empty metadata", () => {
    expect(readProviderLimits(null)).toBeNull();
    expect(readProviderLimits(undefined)).toBeNull();
    expect(readProviderLimits({})).toBeNull();
    expect(readProviderLimits({ provider_limits: null as unknown as object })).toBeNull();
  });

  it("parses a full exhausted snapshot from task_error", () => {
    const snap = readProviderLimits({
      provider_limits: {
        provider: "claude",
        status: "exhausted",
        source: "task_error",
        observed_at: "2026-07-29T18:00:00Z",
        windows: [
          {
            kind: "five_hour",
            used_pct: null,
            resets_at: null,
            resets_label: "3:45pm",
            label: "session limit",
          },
        ],
        message: "You've hit your session limit · resets 3:45pm",
      },
    });
    expect(snap).toEqual({
      provider: "claude",
      status: "exhausted",
      source: "task_error",
      observed_at: "2026-07-29T18:00:00Z",
      windows: [
        {
          kind: "five_hour",
          used_pct: null,
          resets_at: null,
          resets_label: "3:45pm",
          label: "session limit",
        },
      ],
      message: "You've hit your session limit · resets 3:45pm",
    });
  });

  it("defaults unknown status/kind and drops windows that are not objects", () => {
    const snap = readProviderLimits({
      provider_limits: {
        provider: "claude",
        status: "not-a-real-status",
        source: "weird",
        observed_at: "2026-07-29T18:00:00Z",
        windows: ["nope", { kind: "mystery", used_pct: 12, resets_at: null }],
      },
    });
    expect(snap?.status).toBe("unknown");
    expect(snap?.source).toBe("task_error");
    expect(snap?.windows).toHaveLength(1);
    expect(snap?.windows[0]?.kind).toBe("unknown");
    expect(snap?.windows[0]?.used_pct).toBe(12);
  });

  it("requires observed_at", () => {
    expect(
      readProviderLimits({
        provider_limits: {
          provider: "claude",
          status: "exhausted",
          source: "task_error",
          windows: [],
        },
      }),
    ).toBeNull();
  });
});
