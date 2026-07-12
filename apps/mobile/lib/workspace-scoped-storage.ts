/**
 * Workspace-scoped SecureStore helpers for mobile view preferences.
 * Keys are namespaced by workspace slug so each workspace keeps its own
 * Issues filter/collapse state across app restarts.
 */
import * as SecureStore from "expo-secure-store";

const KEY_PREFIX = "multica_issues_view";

export function issuesViewStorageKey(slug: string): string {
  return `${KEY_PREFIX}:${slug}`;
}

export async function loadIssuesViewState<T>(slug: string): Promise<T | null> {
  const raw = await SecureStore.getItemAsync(issuesViewStorageKey(slug));
  if (!raw) return null;
  try {
    return JSON.parse(raw) as T;
  } catch {
    return null;
  }
}

export async function saveIssuesViewState<T>(
  slug: string,
  state: T,
): Promise<void> {
  await SecureStore.setItemAsync(
    issuesViewStorageKey(slug),
    JSON.stringify(state),
  );
}