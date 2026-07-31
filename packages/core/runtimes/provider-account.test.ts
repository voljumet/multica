import { describe, expect, it } from "vitest";
import {
  providerAccountLabel,
  providerAccountSubLabel,
  readProviderAccount,
  readProviderAccountDescription,
} from "./provider-account";

describe("readProviderAccount", () => {
  it("returns null when absent", () => {
    expect(readProviderAccount(null)).toBeNull();
    expect(readProviderAccount({})).toBeNull();
  });

  it("parses expanded identity fields", () => {
    expect(
      readProviderAccount({
        provider_account: {
          email: "maxed@example.com",
          auth_mode: "oauth",
          key_hint: "···a7f3",
          providers: ["moonshot", "zhipu"],
          source: "opencode_auth",
        },
      }),
    ).toMatchObject({
      email: "maxed@example.com",
      auth_mode: "oauth",
      key_hint: "···a7f3",
      providers: ["moonshot", "zhipu"],
    });
  });
});

describe("readProviderAccountDescription", () => {
  it("reads user label", () => {
    expect(
      readProviderAccountDescription({
        provider_account_description: " Work Max ",
      }),
    ).toBe("Work Max");
    expect(readProviderAccountDescription({})).toBeNull();
  });
});

describe("providerAccountLabel", () => {
  it("prefers user description over email", () => {
    expect(
      providerAccountLabel(
        { email: "a@b.co", display_name: "A" },
        "Team Max box",
      ),
    ).toBe("Team Max box");
  });

  it("falls back through email → key hint → providers", () => {
    expect(providerAccountLabel({ email: "a@b.co" })).toBe("a@b.co");
    expect(providerAccountLabel({ key_hint: "···x9", auth_mode: "api_key" })).toBe(
      "api key ···x9",
    );
    expect(providerAccountLabel({ providers: ["moonshot", "zhipu"] })).toBe(
      "moonshot, zhipu",
    );
  });
});

describe("providerAccountSubLabel", () => {
  it("shows auto identity under a user description", () => {
    expect(
      providerAccountSubLabel({ email: "a@b.co" }, "Work Max"),
    ).toBe("a@b.co");
  });
});
