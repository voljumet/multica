/**
 * Compose the absolute GitLab project webhook URL for a workspace.
 *
 * Resolution order (same idea as autopilot webhooks):
 *  1. apiBaseUrl — desktop and split-origin clients always know the API host.
 *  2. currentOrigin — browser same-origin fallback (Next.js web).
 *
 * Non-http origins (Electron `file://`, `app://`, opaque `"null"`) are ignored
 * so the UI never shows `file:///api/webhooks/gitlab/...`.
 */
export function buildGitLabWebhookUrl(params: {
  workspaceId: string;
  apiBaseUrl?: string;
  currentOrigin?: string;
}): string {
  const path = `/api/webhooks/gitlab/${params.workspaceId}`;
  const base =
    stripTrailingSlash(params.apiBaseUrl) || usableHttpOrigin(params.currentOrigin);
  return base ? `${base}${path}` : path;
}

function stripTrailingSlash(s: string | undefined): string {
  if (!s) return "";
  return s.endsWith("/") ? s.slice(0, -1) : s;
}

/** Accept only real public web origins as a webhook host. */
function usableHttpOrigin(origin: string | undefined): string {
  if (!origin) return "";
  if (origin === "null") return "";
  // Electron desktop / custom protocol shells are not a public API host.
  if (/^(file|app|chrome-extension):/i.test(origin)) return "";
  return stripTrailingSlash(origin);
}
