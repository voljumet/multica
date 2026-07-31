package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// ProviderAccount is non-secret identity for the CLI login on this machine.
// Written into agent_runtime.metadata.provider_account at register time so the
// UI can tell multi-account setups apart. Never include tokens or secrets.
//
// User-authored labels live separately at metadata.provider_account_description
// so daemon re-register cannot clobber them.
type ProviderAccount struct {
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	OrgName     string `json:"org_name,omitempty"`
	// OrgType is the subscription class Claude reports (e.g. claude_pro).
	OrgType string `json:"org_type,omitempty"`
	// AuthMode is a coarse credential shape: "oauth", "api_key", "session".
	AuthMode string `json:"auth_mode,omitempty"`
	// KeyHint is a non-secret fingerprint of an API key (e.g. "···a7f3").
	KeyHint string `json:"key_hint,omitempty"`
	// Providers lists configured provider ids for multi-provider CLIs (OpenCode).
	Providers []string `json:"providers,omitempty"`
	// Source names where we read the identity (e.g. "claude_config").
	Source string `json:"source,omitempty"`
}

// resolveProviderAccount returns best-effort login identity for a Multica
// provider (built-in protocol name such as "claude"). Returns nil when the
// provider has no known identity source or nothing is configured.
func resolveProviderAccount(provider string) *ProviderAccount {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "claude":
		return readClaudeOAuthAccount()
	case "grok":
		return readGrokAuthAccount()
	case "pi":
		return readPiAuthAccount()
	case "kimi":
		return readKimiAuthAccount()
	case "opencode":
		return readOpenCodeAuthAccount()
	default:
		return nil
	}
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// keyHintFromSecret returns a short non-secret fingerprint of a credential.
// Never log the full secret. Empty input → empty hint.
func keyHintFromSecret(secret string) string {
	s := strings.TrimSpace(secret)
	if s == "" {
		return ""
	}
	// Strip common "Bearer " prefix if present.
	if len(s) > 7 && strings.EqualFold(s[:7], "bearer ") {
		s = strings.TrimSpace(s[7:])
	}
	runes := []rune(s)
	if len(runes) <= 4 {
		return "···"
	}
	return "···" + string(runes[len(runes)-4:])
}

// applyProviderAccountToRegister copies non-empty identity fields onto a
// daemon Register runtime map. Keep field names in sync with
// DaemonRegisterRequest on the server.
func applyProviderAccountToRegister(rt map[string]any, acct *ProviderAccount) {
	if acct == nil || rt == nil {
		return
	}
	if acct.Email != "" {
		rt["account_email"] = acct.Email
	}
	if acct.DisplayName != "" {
		rt["account_display_name"] = acct.DisplayName
	}
	if acct.OrgName != "" {
		rt["account_org_name"] = acct.OrgName
	}
	if acct.OrgType != "" {
		rt["account_org_type"] = acct.OrgType
	}
	if acct.AuthMode != "" {
		rt["account_auth_mode"] = acct.AuthMode
	}
	if acct.KeyHint != "" {
		rt["account_key_hint"] = acct.KeyHint
	}
	if len(acct.Providers) > 0 {
		rt["account_providers"] = strings.Join(acct.Providers, ",")
	}
	if acct.Source != "" {
		rt["account_source"] = acct.Source
	}
}

func accountOrNil(a *ProviderAccount) *ProviderAccount {
	if a == nil {
		return nil
	}
	if a.Email == "" && a.DisplayName == "" && a.OrgName == "" &&
		a.KeyHint == "" && a.AuthMode == "" && len(a.Providers) == 0 {
		return nil
	}
	return a
}

// ---------------------------------------------------------------------------
// Claude — ~/.claude.json oauthAccount
// ---------------------------------------------------------------------------

func claudeConfigPaths() []string {
	var paths []string
	if dir := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); dir != "" {
		paths = append(paths, filepath.Join(dir, ".claude.json"))
		paths = append(paths, filepath.Join(dir, "claude.json"))
	}
	if home := homeDir(); home != "" {
		paths = append(paths, filepath.Join(home, ".claude.json"))
	}
	return paths
}

func readClaudeOAuthAccount() *ProviderAccount {
	var last *ProviderAccount
	for _, path := range claudeConfigPaths() {
		acct, err := parseClaudeConfigFile(path)
		if err != nil || acct == nil {
			continue
		}
		if acct.Email != "" {
			return acct
		}
		last = acct
	}
	return last
}

func parseClaudeConfigFile(path string) (*ProviderAccount, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root struct {
		OAuthAccount *struct {
			EmailAddress     string `json:"emailAddress"`
			DisplayName      string `json:"displayName"`
			OrganizationName string `json:"organizationName"`
			OrganizationType string `json:"organizationType"`
		} `json:"oauthAccount"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if root.OAuthAccount == nil {
		return nil, nil
	}
	email := strings.TrimSpace(root.OAuthAccount.EmailAddress)
	display := strings.TrimSpace(root.OAuthAccount.DisplayName)
	orgName := strings.TrimSpace(root.OAuthAccount.OrganizationName)
	orgType := strings.TrimSpace(root.OAuthAccount.OrganizationType)
	if email == "" && display == "" && orgName == "" {
		return nil, nil
	}
	return &ProviderAccount{
		Email:       email,
		DisplayName: display,
		OrgName:     orgName,
		OrgType:     orgType,
		AuthMode:    "oauth",
		Source:      "claude_config",
	}, nil
}

// ---------------------------------------------------------------------------
// Grok — ~/.grok/auth.json (OIDC profile fields; tokens ignored)
// ---------------------------------------------------------------------------

func readGrokAuthAccount() *ProviderAccount {
	home := homeDir()
	if home == "" {
		return nil
	}
	return parseGrokAuthFile(filepath.Join(home, ".grok", "auth.json"))
}

func parseGrokAuthFile(path string) *ProviderAccount {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	// Map of "issuer::client_id" → profile. Prefer any entry with email.
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil
	}
	var fallback *ProviderAccount
	for _, raw := range root {
		var entry struct {
			AuthMode  string `json:"auth_mode"`
			Email     string `json:"email"`
			FirstName string `json:"first_name"`
			UserID    string `json:"user_id"`
			TeamID    string `json:"team_id"`
		}
		if err := json.Unmarshal(raw, &entry); err != nil {
			continue
		}
		email := strings.TrimSpace(entry.Email)
		display := strings.TrimSpace(entry.FirstName)
		authMode := strings.TrimSpace(entry.AuthMode)
		if authMode == "" {
			authMode = "oauth"
		}
		if email == "" && display == "" {
			continue
		}
		acct := &ProviderAccount{
			Email:       email,
			DisplayName: display,
			AuthMode:    authMode,
			Source:      "grok_auth",
		}
		if email != "" {
			return acct
		}
		if fallback == nil {
			fallback = acct
		}
	}
	return fallback
}

// ---------------------------------------------------------------------------
// Pi — ~/.pi/agent/auth.json (provider → oauth/api tokens; no email)
// ---------------------------------------------------------------------------

func readPiAuthAccount() *ProviderAccount {
	home := homeDir()
	if home == "" {
		return nil
	}
	return parsePiAuthFile(filepath.Join(home, ".pi", "agent", "auth.json"))
}

func parsePiAuthFile(path string) *ProviderAccount {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil
	}
	if len(root) == 0 {
		return nil
	}
	providers := make([]string, 0, len(root))
	authMode := ""
	var keyHint string
	for name, raw := range root {
		providers = append(providers, name)
		var entry struct {
			Type    string `json:"type"`
			Access  string `json:"access"`
			APIKey  string `json:"apiKey"`
			Key     string `json:"key"`
			Refresh string `json:"refresh"`
		}
		_ = json.Unmarshal(raw, &entry)
		mode := strings.ToLower(strings.TrimSpace(entry.Type))
		if mode == "" {
			if entry.APIKey != "" || entry.Key != "" {
				mode = "api_key"
			} else if entry.Access != "" || entry.Refresh != "" {
				mode = "oauth"
			}
		}
		if authMode == "" && mode != "" {
			authMode = mode
		}
		if keyHint == "" {
			keyHint = firstNonEmpty(
				keyHintFromSecret(entry.APIKey),
				keyHintFromSecret(entry.Key),
			)
		}
	}
	return accountOrNil(&ProviderAccount{
		AuthMode:  authMode,
		KeyHint:   keyHint,
		Providers: providers,
		Source:    "pi_auth",
	})
}

// ---------------------------------------------------------------------------
// Kimi — ~/.kimi-code/credentials (tokens only)
// ---------------------------------------------------------------------------

func readKimiAuthAccount() *ProviderAccount {
	home := homeDir()
	if home == "" {
		return nil
	}
	// credentials/kimi-code.json is the managed OAuth store.
	path := filepath.Join(home, ".kimi-code", "credentials", "kimi-code.json")
	return parseKimiCredentialsFile(path)
}

func parseKimiCredentialsFile(path string) *ProviderAccount {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var root struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
		TokenType    string `json:"token_type"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil
	}
	if strings.TrimSpace(root.AccessToken) == "" && strings.TrimSpace(root.RefreshToken) == "" {
		return nil
	}
	// No email in this store — surface that OAuth is present + scope.
	return &ProviderAccount{
		AuthMode:    "oauth",
		DisplayName: firstNonEmpty(strings.TrimSpace(root.Scope), "kimi oauth"),
		Source:      "kimi_credentials",
	}
}

// ---------------------------------------------------------------------------
// OpenCode — ~/.local/share/opencode/auth.json (provider API keys)
// Also used when agents run Kimi/z.ai models through OpenCode.
// ---------------------------------------------------------------------------

func openCodeAuthPaths() []string {
	var paths []string
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		paths = append(paths, filepath.Join(xdg, "opencode", "auth.json"))
	}
	if home := homeDir(); home != "" {
		paths = append(paths, filepath.Join(home, ".local", "share", "opencode", "auth.json"))
		// Some installs put config under Application Support / .config.
		paths = append(paths, filepath.Join(home, ".config", "opencode", "auth.json"))
		paths = append(paths, filepath.Join(home, "Library", "Application Support", "opencode", "auth.json"))
	}
	return paths
}

func readOpenCodeAuthAccount() *ProviderAccount {
	for _, path := range openCodeAuthPaths() {
		if acct := parseOpenCodeAuthFile(path); acct != nil {
			return acct
		}
	}
	return nil
}

func parseOpenCodeAuthFile(path string) *ProviderAccount {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	// OpenCode stores a map of provider id → credential object.
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil
	}
	if len(root) == 0 {
		return nil
	}
	providers := make([]string, 0, len(root))
	var keyHint string
	authMode := "api_key"
	for name, raw := range root {
		providers = append(providers, name)
		var entry struct {
			Type   string `json:"type"`
			Key    string `json:"key"`
			APIKey string `json:"apiKey"`
			Token  string `json:"token"`
		}
		_ = json.Unmarshal(raw, &entry)
		if keyHint == "" {
			keyHint = firstNonEmpty(
				keyHintFromSecret(entry.Key),
				keyHintFromSecret(entry.APIKey),
				keyHintFromSecret(entry.Token),
			)
		}
		if t := strings.ToLower(strings.TrimSpace(entry.Type)); t == "oauth" {
			authMode = "oauth"
		}
	}
	return accountOrNil(&ProviderAccount{
		AuthMode:  authMode,
		KeyHint:   keyHint,
		Providers: providers,
		Source:    "opencode_auth",
	})
}
