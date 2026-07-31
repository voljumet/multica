/**
 * Non-secret CLI login identity on a runtime.
 *
 * Auto identity (daemon register) lives at metadata.provider_account.
 * User-authored label lives at metadata.provider_account_description so
 * re-register cannot clobber it.
 */

export interface ProviderAccount {
  email?: string;
  display_name?: string;
  org_name?: string;
  /** e.g. claude_pro, claude_max — provider-specific. */
  org_type?: string;
  /** oauth | api_key | session | … */
  auth_mode?: string;
  /** Non-secret API key fingerprint, e.g. "···a7f3". */
  key_hint?: string;
  /** Multi-provider CLIs (OpenCode) list configured provider ids. */
  providers?: string[];
  source?: string;
}

function asString(v: unknown): string | undefined {
  return typeof v === "string" && v.trim().length > 0 ? v.trim() : undefined;
}

export function readProviderAccount(
  metadata: Record<string, unknown> | null | undefined,
): ProviderAccount | null {
  if (!metadata || typeof metadata !== "object") return null;
  const raw = metadata.provider_account;
  if (!raw || typeof raw !== "object") return null;
  const o = raw as Record<string, unknown>;
  const email = asString(o.email);
  const display_name = asString(o.display_name);
  const org_name = asString(o.org_name);
  const org_type = asString(o.org_type);
  const auth_mode = asString(o.auth_mode);
  const key_hint = asString(o.key_hint);
  const source = asString(o.source);
  let providers: string[] | undefined;
  if (Array.isArray(o.providers)) {
    providers = o.providers
      .filter((p): p is string => typeof p === "string" && p.trim().length > 0)
      .map((p) => p.trim());
    if (providers.length === 0) providers = undefined;
  }
  if (
    !email &&
    !display_name &&
    !org_name &&
    !auth_mode &&
    !key_hint &&
    !providers
  ) {
    return null;
  }
  return {
    email,
    display_name,
    org_name,
    org_type,
    auth_mode,
    key_hint,
    providers,
    source,
  };
}

/** User-authored label; empty/missing → null. */
export function readProviderAccountDescription(
  metadata: Record<string, unknown> | null | undefined,
): string | null {
  if (!metadata || typeof metadata !== "object") return null;
  return asString(metadata.provider_account_description) ?? null;
}

/**
 * Primary UI label: user description wins, then email, display name, key
 * hint, org, then first configured provider id.
 */
export function providerAccountLabel(
  account: ProviderAccount | null | undefined,
  description?: string | null,
): string | null {
  const desc = description?.trim();
  if (desc) return desc;
  if (!account) return null;
  if (account.email) return account.email;
  if (account.display_name) return account.display_name;
  if (account.key_hint) {
    const mode = account.auth_mode === "oauth" ? "oauth" : "api key";
    return `${mode} ${account.key_hint}`;
  }
  if (account.org_name) return account.org_name;
  if (account.providers && account.providers.length > 0) {
    return account.providers.join(", ");
  }
  if (account.auth_mode) return account.auth_mode;
  return null;
}

/** Secondary line under the primary label (email when description is set, etc.). */
export function providerAccountSubLabel(
  account: ProviderAccount | null | undefined,
  description?: string | null,
): string | null {
  const desc = description?.trim();
  if (!account) return null;
  if (desc) {
    // When the user set a description, show the auto identity underneath.
    return (
      account.email ??
      account.display_name ??
      (account.key_hint
        ? `${account.auth_mode === "oauth" ? "oauth" : "api key"} ${account.key_hint}`
        : null) ??
      (account.providers?.length ? account.providers.join(", ") : null)
    );
  }
  // No description: optional secondary detail under the primary.
  if (account.email && account.org_type) return account.org_type;
  if (account.email && account.org_name) return account.org_name;
  if (account.providers && account.providers.length > 1) {
    return account.providers.join(", ");
  }
  return null;
}
