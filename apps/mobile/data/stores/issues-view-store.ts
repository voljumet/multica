/**
 * View store for the workspace-wide Issues page (`(tabs)/issues.tsx`).
 *
 * Shape mirrors `useMyIssuesViewStore` plus a `scope` field — workspace
 * Issues has `all / members / agents` scope tabs (see web
 * `packages/views/issues/components/issues-page.tsx:32-94`), while
 * My Issues has its own `assigned / created / agents` scopes.
 *
 * The `scope` filter is **client-side** on `assignee_type` — see
 * `(tabs)/issues.tsx`'s `scopedIssues` derivation. Server param stays unset
 * so the cache key (`issueKeys.list(wsId)`) and WS realtime invalidation
 * (`useIssuesRealtime`) don't have to know about scope.
 *
 * `IssuesScope` is defined locally rather than imported from
 * `@multica/core/issues/stores/issues-scope-store` — mobile only
 * `import type` from `@multica/core/types/*` per Sharing Principles, and
 * the union is small enough that a duplicated literal is preferable to a
 * cross-package type import hop.
 *
 * Empty filter array = "show all" (matches web's predicate semantics in
 * packages/views/issues/utils/filter.ts).
 *
 * Filter/collapse preferences persist per workspace slug via SecureStore.
 * `clearFilters` deliberately does NOT reset `scope` or `collapsedStatuses`.
 */
import { create } from "zustand";
import type { IssuePriority, IssueStatus } from "@multica/core/types";
import {
  loadIssuesViewState,
  saveIssuesViewState,
} from "@/lib/workspace-scoped-storage";

export type IssuesScope = "all" | "members" | "agents";

export interface PersistedIssuesViewState {
  statusFilters: IssueStatus[];
  priorityFilters: IssuePriority[];
  projectFilters: string[];
  includeNoProject: boolean;
  collapsedStatuses: IssueStatus[];
}

interface IssuesViewState extends PersistedIssuesViewState {
  scope: IssuesScope;
  sortByLastEdited: boolean;
  setScope: (scope: IssuesScope) => void;
  toggleStatusFilter: (status: IssueStatus) => void;
  togglePriorityFilter: (priority: IssuePriority) => void;
  toggleProjectFilter: (projectId: string) => void;
  toggleNoProject: () => void;
  toggleSortByLastEdited: () => void;
  toggleStatusCollapse: (status: IssueStatus) => void;
  clearFilters: () => void;
  hydrateForWorkspace: (slug: string) => Promise<void>;
}

const defaultPersistedState = (): PersistedIssuesViewState => ({
  statusFilters: [],
  priorityFilters: [],
  projectFilters: [],
  includeNoProject: false,
  collapsedStatuses: [],
});

let persistSlug: string | null = null;
let persistTimer: ReturnType<typeof setTimeout> | null = null;

function schedulePersist(slug: string, state: PersistedIssuesViewState) {
  if (persistTimer) clearTimeout(persistTimer);
  persistTimer = setTimeout(() => {
    void saveIssuesViewState(slug, state);
  }, 200);
}

function pickPersisted(state: IssuesViewState): PersistedIssuesViewState {
  return {
    statusFilters: state.statusFilters,
    priorityFilters: state.priorityFilters,
    projectFilters: state.projectFilters,
    includeNoProject: state.includeNoProject,
    collapsedStatuses: state.collapsedStatuses,
  };
}

export const useIssuesViewStore = create<IssuesViewState>((set, get) => ({
  scope: "all",
  ...defaultPersistedState(),
  sortByLastEdited: false,
  setScope: (scope) => set({ scope }),
  toggleStatusFilter: (status) =>
    set((state) => ({
      statusFilters: state.statusFilters.includes(status)
        ? state.statusFilters.filter((s) => s !== status)
        : [...state.statusFilters, status],
    })),
  togglePriorityFilter: (priority) =>
    set((state) => ({
      priorityFilters: state.priorityFilters.includes(priority)
        ? state.priorityFilters.filter((p) => p !== priority)
        : [...state.priorityFilters, priority],
    })),
  toggleProjectFilter: (projectId) =>
    set((state) => ({
      projectFilters: state.projectFilters.includes(projectId)
        ? state.projectFilters.filter((id) => id !== projectId)
        : [...state.projectFilters, projectId],
    })),
  toggleNoProject: () =>
    set((state) => ({ includeNoProject: !state.includeNoProject })),
  toggleSortByLastEdited: () =>
    set((state) => ({ sortByLastEdited: !state.sortByLastEdited })),
  toggleStatusCollapse: (status) =>
    set((state) => ({
      collapsedStatuses: state.collapsedStatuses.includes(status)
        ? state.collapsedStatuses.filter((s) => s !== status)
        : [...state.collapsedStatuses, status],
    })),
  clearFilters: () =>
    set({
      statusFilters: [],
      priorityFilters: [],
      projectFilters: [],
      includeNoProject: false,
    }),
  hydrateForWorkspace: async (slug) => {
    persistSlug = slug;
    const saved = await loadIssuesViewState<PersistedIssuesViewState>(slug);
    set({
      ...defaultPersistedState(),
      ...saved,
    });
  },
}));

useIssuesViewStore.subscribe((state) => {
  if (!persistSlug) return;
  schedulePersist(persistSlug, pickPersisted(state));
});