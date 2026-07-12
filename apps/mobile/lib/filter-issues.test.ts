import { describe, expect, it } from "vitest";
import type { Issue } from "@multica/core/types";
import { filterIssues } from "./filter-issues";

function issue(overrides: Partial<Issue> = {}): Issue {
  return {
    id: "1",
    workspace_id: "ws-1",
    title: "Test",
    status: "todo",
    priority: "medium",
    assignee_type: "member",
    assignee_id: "u-1",
    creator_type: "member",
    creator_id: "u-1",
    project_id: null,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  } as Issue;
}

describe("filterIssues", () => {
  const issues = [
    issue({ id: "a", project_id: null }),
    issue({ id: "b", project_id: "p-1" }),
    issue({ id: "c", project_id: "p-2" }),
  ];

  it("returns all issues when no filters are active", () => {
    expect(
      filterIssues(issues, {
        statusFilters: [],
        priorityFilters: [],
        projectFilters: [],
        includeNoProject: false,
      }),
    ).toHaveLength(3);
  });

  it("filters by selected projects with OR semantics", () => {
    const result = filterIssues(issues, {
      statusFilters: [],
      priorityFilters: [],
      projectFilters: ["p-1"],
      includeNoProject: false,
    });
    expect(result.map((i) => i.id)).toEqual(["b"]);
  });

  it("includes unassigned issues when includeNoProject is on", () => {
    const result = filterIssues(issues, {
      statusFilters: [],
      priorityFilters: [],
      projectFilters: [],
      includeNoProject: true,
    });
    expect(result.map((i) => i.id)).toEqual(["a"]);
  });

  it("combines project selection with includeNoProject", () => {
    const result = filterIssues(issues, {
      statusFilters: [],
      priorityFilters: [],
      projectFilters: ["p-1"],
      includeNoProject: true,
    });
    expect(result.map((i) => i.id)).toEqual(["a", "b"]);
  });
});