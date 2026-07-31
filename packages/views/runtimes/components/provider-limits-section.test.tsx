// @vitest-environment jsdom

import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { I18nProvider } from "@multica/core/i18n/react";
import type { AgentRuntime } from "@multica/core/types";
import enCommon from "../../locales/en/common.json";
import enRuntimes from "../../locales/en/runtimes.json";
import { ProviderLimitsSection } from "./provider-limits-section";

const TEST_RESOURCES = { en: { common: enCommon, runtimes: enRuntimes } };

vi.mock("./provider-logo", () => ({
  ProviderLogo: () => <span data-testid="provider-logo" />,
}));

function wrap(ui: ReactNode) {
  return (
    <I18nProvider locale="en" resources={TEST_RESOURCES}>
      {ui}
    </I18nProvider>
  );
}

function makeRuntime(overrides: Partial<AgentRuntime> = {}): AgentRuntime {
  return {
    id: "rt-1",
    workspace_id: "ws-1",
    daemon_id: "d-1",
    name: "claude",
    runtime_mode: "local",
    provider: "claude",
    launch_header: "",
    status: "online",
    device_info: "host",
    metadata: {},
    owner_id: "u-1",
    visibility: "private",
    last_seen_at: new Date().toISOString(),
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  };
}

describe("ProviderLimitsSection", () => {
  it("hides for cloud runtimes", () => {
    const { container } = render(
      wrap(
        <ProviderLimitsSection
          runtime={makeRuntime({ runtime_mode: "cloud" })}
        />,
      ),
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("shows empty state when no snapshot", () => {
    render(wrap(<ProviderLimitsSection runtime={makeRuntime()} />));
    expect(
      screen.getByText(/Limit status is not reported for this runtime yet/i),
    ).toBeTruthy();
    expect(screen.getByText(/Provider limits/i)).toBeTruthy();
  });

  it("renders exhausted session limit with reset label", () => {
    render(
      wrap(
        <ProviderLimitsSection
          runtime={makeRuntime({
            metadata: {
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
              },
            },
          })}
        />,
      ),
    );
    expect(screen.getByText(/Plan limit hit/i)).toBeTruthy();
    expect(screen.getByText(/session limit/i)).toBeTruthy();
    expect(screen.getByText(/Resets 3:45pm/i)).toBeTruthy();
    expect(screen.getByText(/from a failed task/i)).toBeTruthy();
  });
});
