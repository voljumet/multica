package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseClaudeConfigFile_ReadsOAuthAccount(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")
	content := `{
  "oauthAccount": {
    "emailAddress": "maxed@example.com",
    "displayName": "Max User",
    "organizationName": "Max Org",
    "organizationType": "claude_max"
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	acct, err := parseClaudeConfigFile(path)
	if err != nil || acct == nil {
		t.Fatalf("got %+v err=%v", acct, err)
	}
	if acct.Email != "maxed@example.com" || acct.AuthMode != "oauth" {
		t.Fatalf("unexpected %+v", acct)
	}
}

func TestParseGrokAuthFile_ReadsEmail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	content := `{
  "https://auth.x.ai::client": {
    "auth_mode": "oidc",
    "email": "grok@example.com",
    "first_name": "Grok User",
    "refresh_token": "SECRET_DO_NOT_LEAK"
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	acct := parseGrokAuthFile(path)
	if acct == nil || acct.Email != "grok@example.com" {
		t.Fatalf("got %+v", acct)
	}
	if acct.DisplayName != "Grok User" {
		t.Fatalf("display = %q", acct.DisplayName)
	}
}

func TestParsePiAuthFile_ProvidersAndHint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	content := `{
  "anthropic": {
    "type": "oauth",
    "access": "aaaa",
    "refresh": "bbbb",
    "expires": 1
  },
  "openai": {
    "type": "api",
    "apiKey": "sk-proj-ABCDEFGH1234"
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	acct := parsePiAuthFile(path)
	if acct == nil {
		t.Fatal("nil")
	}
	if len(acct.Providers) != 2 {
		t.Fatalf("providers = %v", acct.Providers)
	}
	if acct.KeyHint != "···1234" {
		t.Fatalf("key hint = %q", acct.KeyHint)
	}
}

func TestParseKimiCredentialsFile_OAuthPresent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kimi-code.json")
	if err := os.WriteFile(path, []byte(`{
  "access_token": "tok",
  "refresh_token": "ref",
  "scope": "kimi-code"
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	acct := parseKimiCredentialsFile(path)
	if acct == nil || acct.AuthMode != "oauth" {
		t.Fatalf("got %+v", acct)
	}
	if acct.DisplayName != "kimi-code" {
		t.Fatalf("display = %q", acct.DisplayName)
	}
}

func TestParseOpenCodeAuthFile_Providers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(path, []byte(`{
  "moonshot": { "type": "api", "key": "msk-abcdefghij" },
  "zhipu": { "type": "api", "key": "zai-xyz9999" }
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	acct := parseOpenCodeAuthFile(path)
	if acct == nil {
		t.Fatal("nil")
	}
	if len(acct.Providers) != 2 {
		t.Fatalf("providers = %v", acct.Providers)
	}
	// Map iteration order is random; either key may supply the hint.
	if acct.KeyHint != "···ghij" && acct.KeyHint != "···9999" {
		t.Fatalf("key hint = %q, want ···ghij or ···9999", acct.KeyHint)
	}
}

func TestKeyHintFromSecret(t *testing.T) {
	// "sk-abcXYZ12" last 4 runes = "YZ12"
	if got := keyHintFromSecret("sk-abcXYZ12"); got != "···YZ12" {
		t.Fatalf("got %q", got)
	}
	if got := keyHintFromSecret(""); got != "" {
		t.Fatalf("empty → %q", got)
	}
}

func TestResolveProviderAccount_Unknown(t *testing.T) {
	if got := resolveProviderAccount("codex"); got != nil {
		t.Fatalf("codex should be nil, got %+v", got)
	}
}

func TestReadClaudeOAuthAccount_PrefersClaudeConfigDir(t *testing.T) {
	home := t.TempDir()
	cfgDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{
  "oauthAccount": {"emailAddress": "home@example.com"}
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, ".claude.json"), []byte(`{
  "oauthAccount": {"emailAddress": "override@example.com"}
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)
	acct := readClaudeOAuthAccount()
	if acct == nil || acct.Email != "override@example.com" {
		t.Fatalf("got %+v", acct)
	}
}
