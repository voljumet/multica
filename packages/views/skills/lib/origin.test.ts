import { describe, expect, it } from "vitest";
import type { SkillSummary } from "@multica/core/types";
import { canRefreshFromURL, readOrigin } from "./origin";

function skill(config: Record<string, unknown>): SkillSummary {
  return {
    id: "s1",
    workspace_id: "ws1",
    name: "demo",
    description: "",
    config,
    created_by: null,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

describe("canRefreshFromURL", () => {
  it("is false for manual skills", () => {
    expect(canRefreshFromURL(skill({}))).toBe(false);
  });

  it("is false for runtime_local skills", () => {
    expect(
      canRefreshFromURL(
        skill({
          origin: {
            type: "runtime_local",
            runtime_id: "r1",
            source_path: "~/.claude/skills/x",
          },
        }),
      ),
    ).toBe(false);
  });

  it("is true for github skills with source_url", () => {
    expect(
      canRefreshFromURL(
        skill({
          origin: {
            type: "github",
            source_url: "https://github.com/acme/skills/tree/main/foo",
          },
        }),
      ),
    ).toBe(true);
  });

  it("is false when github origin lacks source_url", () => {
    expect(
      canRefreshFromURL(skill({ origin: { type: "github" } })),
    ).toBe(false);
  });
});

describe("readOrigin", () => {
  it("surfaces source_url for URL-imported skills", () => {
    const origin = readOrigin(
      skill({
        origin: {
          type: "skills_sh",
          source_url: "https://skills.sh/acme/repo/foo",
        },
      }),
    );
    expect(origin.type).toBe("skills_sh");
    expect(origin.source_url).toBe("https://skills.sh/acme/repo/foo");
  });
});
