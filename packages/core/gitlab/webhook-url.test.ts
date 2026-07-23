import { describe, it, expect } from "vitest";
import { buildGitLabWebhookUrl } from "./webhook-url";

const wsId = "53b994ad-73fd-439a-8807-166aa1badaea";

describe("buildGitLabWebhookUrl", () => {
  it("prefers apiBaseUrl (desktop / split-origin)", () => {
    expect(
      buildGitLabWebhookUrl({
        workspaceId: wsId,
        apiBaseUrl: "https://multica.paral.no",
        currentOrigin: "file://",
      }),
    ).toBe(`https://multica.paral.no/api/webhooks/gitlab/${wsId}`);
  });

  it("strips trailing slash on apiBaseUrl", () => {
    expect(
      buildGitLabWebhookUrl({
        workspaceId: wsId,
        apiBaseUrl: "https://multica.paral.no/",
      }),
    ).toBe(`https://multica.paral.no/api/webhooks/gitlab/${wsId}`);
  });

  it("falls back to browser origin when apiBaseUrl is empty", () => {
    expect(
      buildGitLabWebhookUrl({
        workspaceId: wsId,
        apiBaseUrl: "",
        currentOrigin: "https://multica.paral.no",
      }),
    ).toBe(`https://multica.paral.no/api/webhooks/gitlab/${wsId}`);
  });

  it("ignores Electron file:// origin so the UI never shows file:///api/...", () => {
    expect(
      buildGitLabWebhookUrl({
        workspaceId: wsId,
        apiBaseUrl: "",
        currentOrigin: "file://",
      }),
    ).toBe(`/api/webhooks/gitlab/${wsId}`);
  });

  it("ignores app:// and opaque null origins", () => {
    expect(
      buildGitLabWebhookUrl({
        workspaceId: wsId,
        currentOrigin: "app://-",
      }),
    ).toBe(`/api/webhooks/gitlab/${wsId}`);
    expect(
      buildGitLabWebhookUrl({
        workspaceId: wsId,
        currentOrigin: "null",
      }),
    ).toBe(`/api/webhooks/gitlab/${wsId}`);
  });
});
