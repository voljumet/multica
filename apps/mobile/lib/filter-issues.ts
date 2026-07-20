/**
 * Mirrors the filter slice of `filterIssues()` at
 * packages/views/issues/utils/filter.ts. Same predicate, same
 * "empty array = show all" semantics — required by the same-N parity rule
 * in apps/mobile/CLAUDE.md.
 */
import type { Issue, IssuePriority, IssueStatus } from "@multica/core/types";

export interface IssueListFilters {
  statusFilters: IssueStatus[];
  priorityFilters: IssuePriority[];
  projectFilters: string[];
  includeNoProject: boolean;
}

export function filterIssues(
  issues: Issue[],
  filters: IssueListFilters,
): Issue[] {
  const {
    statusFilters,
    priorityFilters,
    projectFilters,
    includeNoProject,
  } = filters;
  const hasProjectFilter =
    projectFilters.length > 0 || includeNoProject;

  return issues.filter((issue) => {
    if (
      statusFilters.length > 0 &&
      !statusFilters.includes(issue.status)
    ) {
      return false;
    }
    if (
      priorityFilters.length > 0 &&
      !priorityFilters.includes(issue.priority)
    ) {
      return false;
    }
    if (hasProjectFilter) {
      if (!issue.project_id) {
        if (!includeNoProject) return false;
      } else if (projectFilters.length > 0) {
        if (!projectFilters.includes(issue.project_id)) return false;
      } else {
        return false;
      }
    }
    return true;
  });
}