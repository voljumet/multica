// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { SkillSummary } from "@multica/core/types";
import { canRefreshFromURL, originSourceUrl, readOrigin, type OriginInfo } from "./origin";

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

// `origin` is persisted JSONB writable verbatim through the skill update API,
// so originSourceUrl is the only thing standing between a hand-edited
// source_url and a live href. These tests pin the fail-closed behaviour.
describe("originSourceUrl", () => {
  const github = (source_url?: string): OriginInfo => ({
    type: "github",
    source_url,
  });

  it("accepts a source_url on the declared origin's own host", () => {
    const url = "https://github.com/anthropics/skills/tree/main/animations";
    expect(originSourceUrl(github(url))).toBe(url);
    expect(
      originSourceUrl({ type: "skills_sh", source_url: "https://skills.sh/a/b/c" }),
    ).toBe("https://skills.sh/a/b/c");
    expect(
      originSourceUrl({ type: "clawhub", source_url: "https://clawhub.ai/owner/slug" }),
    ).toBe("https://clawhub.ai/owner/slug");
  });

  it("accepts http and www variants, matching the server allowlist", () => {
    expect(originSourceUrl(github("http://www.github.com/a/b"))).toBe(
      "http://www.github.com/a/b",
    );
    expect(originSourceUrl(github("https://WWW.GITHUB.COM/a/b"))).toBe(
      "https://WWW.GITHUB.COM/a/b",
    );
  });

  it("rejects a host that does not match the declared origin type", () => {
    expect(originSourceUrl(github("https://evil.example/anthropics/skills"))).toBeNull();
    expect(originSourceUrl(github("https://skills.sh/a/b/c"))).toBeNull();
    expect(
      originSourceUrl({ type: "clawhub", source_url: "https://github.com/a/b" }),
    ).toBeNull();
    // Lookalike hosts must not pass a substring/suffix check.
    expect(originSourceUrl(github("https://github.com.evil.example/a/b"))).toBeNull();
    expect(originSourceUrl(github("https://notgithub.com/a/b"))).toBeNull();
  });

  it("rejects non-http(s) schemes", () => {
    expect(originSourceUrl(github("data:text/html,<script>x</script>"))).toBeNull();
    expect(originSourceUrl(github("javascript:alert(1)"))).toBeNull();
    expect(originSourceUrl(github("ftp://github.com/a/b"))).toBeNull();
  });

  it("rejects malformed and scheme-less values", () => {
    expect(originSourceUrl(github("not a url"))).toBeNull();
    expect(originSourceUrl(github("github.com/a/b"))).toBeNull();
    // A bare ClawHub slug is server-refreshable but not linkable.
    expect(originSourceUrl({ type: "clawhub", source_url: "my-skill" })).toBeNull();
  });

  it("rejects non-import origins even with a well-formed URL", () => {
    expect(
      originSourceUrl({ type: "manual", source_url: "https://github.com/a/b" }),
    ).toBeNull();
    expect(
      originSourceUrl({ type: "runtime_local", source_url: "https://github.com/a/b" }),
    ).toBeNull();
  });

  it("returns null for a missing or empty source_url", () => {
    expect(originSourceUrl(null)).toBeNull();
    expect(originSourceUrl(github(undefined))).toBeNull();
    expect(originSourceUrl(github(""))).toBeNull();
  });
});
