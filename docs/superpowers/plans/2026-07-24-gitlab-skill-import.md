# GitLab Skill Import Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow skills to be imported and refreshed from private self-hosted GitLab repos by reusing the workspace's existing OAuth access token from `gitlab_connections`.

**Architecture:** Add `sourceGitLab` to the import source enum; detect it by matching the URL host against `GITLAB_URL`. Extract a `gitlabAccessTokenForWorkspace` helper that decrypts (and refreshes if expired) the workspace's OAuth token. Thread that token through `ImportSkill` and `RefreshSkillFromURL` into a new `fetchFromGitLab` function that calls the GitLab repository files API.

**Tech Stack:** Go, GitLab REST API v4 (`/api/v4/projects/{id}/repository/...`), existing `h.GitLabBox` (NaCl secretbox), `h.Queries.GetFirstGitLabConnectionByWorkspace`, `h.refreshGitLabToken`.

## Global Constraints

- Only supported when `GITLAB_URL` env var is set; unknown hosts still error
- Token comes from the workspace's first `gitlab_connections` row — same logic as `LinkGitLabIssueForIssue`
- New fetch file must not import anything outside the `handler` package
- Tests use `net/http/httptest` mock servers — no new test dependencies
- Follow existing Go error wrapping style: `fmt.Errorf("gitlab skill: %w", err)`
- `origin.type` in stored config must be `"gitlab"` (used by the UI to label provenance)

---

### Task 1: Add `sourceGitLab` to detection and URL parsing

**Files:**
- Modify: `server/internal/handler/skill.go:785-822` (enum + `detectImportSource`)
- Create: `server/internal/handler/skill_gitlab.go`
- Create: `server/internal/handler/skill_gitlab_test.go`

**Interfaces:**
- Produces:
  - `sourceGitLab importSource` constant (new enum value in `skill.go`)
  - `gitlabConfiguredHost() string` in `skill_gitlab.go` — returns lowercased hostname of `GITLAB_URL`, empty string if unset/invalid
  - `gitlabSpec` struct with fields `namespace string` (full `group/subgroup/repo`), `ref string`, `skillDir string`
  - `parseGitLabURL(raw string) (gitlabSpec, error)` — parses supported URL forms
  - `gitlabProjectAPIPath(namespace string) string` — URL-encodes namespace for `/api/v4/projects/{id}` calls

- [ ] **Step 1: Write failing tests for `gitlabConfiguredHost` and `parseGitLabURL`**

Create `server/internal/handler/skill_gitlab_test.go`:

```go
package handler

import (
	"os"
	"testing"
)

func TestGitlabConfiguredHost(t *testing.T) {
	t.Setenv("GITLAB_URL", "https://gitlab.company.com")
	if got := gitlabConfiguredHost(); got != "gitlab.company.com" {
		t.Fatalf("got %q, want %q", got, "gitlab.company.com")
	}

	t.Setenv("GITLAB_URL", "")
	if got := gitlabConfiguredHost(); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestParseGitLabURL(t *testing.T) {
	cases := []struct {
		name     string
		url      string
		wantNS   string
		wantRef  string
		wantDir  string
		wantErr  bool
	}{
		{
			name:    "root no path",
			url:     "https://gitlab.company.com/group/repo",
			wantNS:  "group/repo",
			wantRef: "",
			wantDir: "",
		},
		{
			name:    "tree with dash",
			url:     "https://gitlab.company.com/group/repo/-/tree/main/my-skill",
			wantNS:  "group/repo",
			wantRef: "main",
			wantDir: "my-skill",
		},
		{
			name:    "tree without dash",
			url:     "https://gitlab.company.com/group/repo/tree/main/my-skill",
			wantNS:  "group/repo",
			wantRef: "main",
			wantDir: "my-skill",
		},
		{
			name:    "nested subgroup",
			url:     "https://gitlab.company.com/group/sub/repo/-/tree/main/skill-a",
			wantNS:  "group/sub/repo",
			wantRef: "main",
			wantDir: "skill-a",
		},
		{
			name:    "nested skill dir",
			url:     "https://gitlab.company.com/group/repo/-/tree/main/skills/foo",
			wantNS:  "group/repo",
			wantRef: "main",
			wantDir: "skills/foo",
		},
		{
			name:    "root with no segments after host",
			url:     "https://gitlab.company.com/",
			wantErr: true,
		},
		{
			name:    "only owner no repo",
			url:     "https://gitlab.company.com/group",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := parseGitLabURL(tc.url)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if spec.namespace != tc.wantNS {
				t.Errorf("namespace = %q, want %q", spec.namespace, tc.wantNS)
			}
			if spec.ref != tc.wantRef {
				t.Errorf("ref = %q, want %q", spec.ref, tc.wantRef)
			}
			if spec.skillDir != tc.wantDir {
				t.Errorf("skillDir = %q, want %q", spec.skillDir, tc.wantDir)
			}
		})
	}
}

func TestDetectImportSource_GitLab(t *testing.T) {
	t.Setenv("GITLAB_URL", "https://gitlab.company.com")

	source, normalized, err := detectImportSource("https://gitlab.company.com/group/repo/-/tree/main/skill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != sourceGitLab {
		t.Fatalf("source = %v, want sourceGitLab", source)
	}
	if normalized == "" {
		t.Fatal("normalized URL empty")
	}
}

func TestDetectImportSource_GitLabNotConfigured(t *testing.T) {
	t.Setenv("GITLAB_URL", "")

	_, _, err := detectImportSource("https://gitlab.company.com/group/repo")
	if err == nil {
		t.Fatal("expected error when GITLAB_URL not set")
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd server && go test ./internal/handler/... -run "TestGitlabConfigured|TestParseGitLab|TestDetectImportSource_GitLab" -v 2>&1 | tail -20
```

Expected: FAIL — `gitlabConfiguredHost`, `parseGitLabURL`, `sourceGitLab` undefined.

- [ ] **Step 3: Create `skill_gitlab.go` with parsing logic**

Create `server/internal/handler/skill_gitlab.go`:

```go
package handler

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

type gitlabSpec struct {
	namespace string // full "group[/subgroup]/repo"
	ref       string // branch/tag/commit, empty → resolve default
	skillDir  string // relative path within repo, "" for root
}

// gitlabConfiguredHost returns the lowercased hostname from GITLAB_URL,
// or "" when GITLAB_URL is unset or unparseable.
func gitlabConfiguredHost() string {
	base := strings.TrimRight(os.Getenv("GITLAB_URL"), "/")
	if base == "" {
		return ""
	}
	u, err := url.Parse(base)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// parseGitLabURL extracts namespace, ref, and skill directory from a
// self-hosted GitLab URL. Supported forms:
//
//	gitlab.host/group/repo                          → root, default branch
//	gitlab.host/group/repo/-/tree/{ref}/{path...}   → with dash-separator
//	gitlab.host/group/repo/tree/{ref}/{path...}      → without dash-separator
//	gitlab.host/group/sub/repo/-/tree/{ref}/{path}  → nested namespace
func parseGitLabURL(raw string) (gitlabSpec, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return gitlabSpec{}, fmt.Errorf("invalid URL: %w", err)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return gitlabSpec{}, fmt.Errorf("expected gitlab.host/{namespace}/{repo}[/-/tree/{ref}/{path}], got: %s", parsed.Path)
	}

	// Find "tree" or "-" (dash separator) to split namespace from ref/path.
	// GitLab uses /-/tree/ but also accepts /tree/ on some versions.
	dashIdx := -1
	treeIdx := -1
	for i, p := range parts {
		if p == "-" && i >= 2 {
			dashIdx = i
			break
		}
		if p == "tree" && i >= 2 {
			treeIdx = i
			break
		}
	}

	// Bare repo URL (no tree segment): namespace is everything up to end.
	if dashIdx < 0 && treeIdx < 0 {
		// Must be at least group/repo — already checked len >= 2.
		ns := strings.Join(parts, "/")
		ns = strings.TrimSuffix(ns, ".git")
		return gitlabSpec{namespace: ns}, nil
	}

	// Determine where the namespace ends and the tree marker begins.
	markerIdx := dashIdx
	treeStart := dashIdx + 2 // skip "-" and "tree"
	if dashIdx < 0 {
		markerIdx = treeIdx
		treeStart = treeIdx + 1 // skip "tree"
	} else {
		// Confirm "tree" follows "-"
		if dashIdx+1 >= len(parts) || parts[dashIdx+1] != "tree" {
			return gitlabSpec{}, fmt.Errorf("expected /-/tree/ after dash in URL: %s", raw)
		}
	}

	ns := strings.Join(parts[:markerIdx], "/")
	ns = strings.TrimSuffix(ns, ".git")
	if ns == "" {
		return gitlabSpec{}, fmt.Errorf("empty namespace before tree in URL: %s", raw)
	}

	if treeStart >= len(parts) || parts[treeStart] == "" {
		return gitlabSpec{}, fmt.Errorf("missing ref after /tree/ in URL: %s", raw)
	}

	ref := parts[treeStart]
	skillDir := ""
	if treeStart+1 < len(parts) {
		dir := strings.Join(parts[treeStart+1:], "/")
		if dir != "" {
			decoded, err := url.PathUnescape(dir)
			if err != nil {
				return gitlabSpec{}, fmt.Errorf("invalid path in URL: %w", err)
			}
			skillDir = decoded
		}
	}

	return gitlabSpec{namespace: ns, ref: ref, skillDir: skillDir}, nil
}

// gitlabProjectAPIPath URL-encodes a namespace for use in GitLab API paths:
// /api/v4/projects/{encoded_namespace}/...
func gitlabProjectAPIPath(namespace string) string {
	return url.PathEscape(namespace)
}
```

- [ ] **Step 4: Add `sourceGitLab` to the enum and `detectImportSource` in `skill.go`**

In `skill.go`, the enum at line ~785:
```go
// Before:
const (
	sourceClawHub importSource = iota
	sourceSkillsSh
	sourceGitHub
)

// After:
const (
	sourceClawHub importSource = iota
	sourceSkillsSh
	sourceGitHub
	sourceGitLab
)
```

In `detectImportSource` at line ~808, add a case before `default`:
```go
// Before the default case, add:
case host == gitlabConfiguredHost() && gitlabConfiguredHost() != "":
    return sourceGitLab, normalized, nil
```

Also update the error message in the `default` case:
```go
// Before:
return 0, "", fmt.Errorf("unsupported source: %s (supported: clawhub.ai, skills.sh, github.com)", host)
// After:
return 0, "", fmt.Errorf("unsupported source: %s (supported: clawhub.ai, skills.sh, github.com, or your configured GitLab instance)", host)
```

- [ ] **Step 5: Run tests to confirm they pass**

```bash
cd server && go test ./internal/handler/... -run "TestGitlabConfigured|TestParseGitLab|TestDetectImportSource_GitLab" -v 2>&1 | tail -20
```

Expected: PASS all 4 test functions.

- [ ] **Step 6: Commit**

```bash
cd server && git add internal/handler/skill_gitlab.go internal/handler/skill_gitlab_test.go internal/handler/skill.go
git commit -m "feat(skills): add sourceGitLab detection and URL parsing"
```

---

### Task 2: Implement `fetchFromGitLab`

**Files:**
- Modify: `server/internal/handler/skill_gitlab.go` (add fetch function and helpers)
- Modify: `server/internal/handler/skill_gitlab_test.go` (add fetch test with mock server)

**Interfaces:**
- Consumes: `gitlabSpec`, `gitlabProjectAPIPath`, `importedSkill`, `gitlabAPIURL()` (from `gitlab.go`), `fetchRawFile` (from `skill.go`)
- Produces: `fetchFromGitLab(httpClient *http.Client, token, rawURL string) (*importedSkill, error)`

- [ ] **Step 1: Write the failing test**

Add to `skill_gitlab_test.go`:

```go
import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestFetchFromGitLab(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Authorization header is present
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/v4/projects/group%2Frepo":
			// Return default branch
			_ = json.NewEncoder(w).Encode(map[string]any{
				"default_branch": "main",
			})
		case "/api/v4/projects/group%2Frepo/repository/files/SKILL.md/raw":
			w.Write([]byte("---\nname: My Skill\ndescription: A test skill\n---\n# Content"))
		case "/api/v4/projects/group%2Frepo/repository/tree":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"type": "blob", "path": "SKILL.md", "name": "SKILL.md"},
				{"type": "blob", "path": "notes.md", "name": "notes.md"},
			})
		case "/api/v4/projects/group%2Frepo/repository/files/notes.md/raw":
			w.Write([]byte("extra notes"))
		default:
			t.Logf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	t.Setenv("GITLAB_URL", srv.URL)

	httpClient := srv.Client()
	skill, err := fetchFromGitLab(httpClient, "test-token", srv.URL+"/group/repo")
	if err != nil {
		t.Fatalf("fetchFromGitLab: %v", err)
	}
	if skill.name != "My Skill" {
		t.Errorf("name = %q, want %q", skill.name, "My Skill")
	}
	if skill.description != "A test skill" {
		t.Errorf("description = %q", skill.description)
	}
	if skill.origin["type"] != "gitlab" {
		t.Errorf("origin.type = %v, want %q", skill.origin["type"], "gitlab")
	}
	if skill.origin["source_url"] != srv.URL+"/group/repo" {
		t.Errorf("origin.source_url = %v", skill.origin["source_url"])
	}
	foundNotes := false
	for _, f := range skill.files {
		if f.path == "notes.md" && f.content == "extra notes" {
			foundNotes = true
		}
	}
	if !foundNotes {
		t.Errorf("expected notes.md in files, got %v", skill.files)
	}
}

func TestFetchFromGitLab_SkillDir(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/v4/projects/group%2Frepo/repository/files/skills%2Ffoo%2FSKILL.md/raw":
			w.Write([]byte("---\nname: Foo Skill\n---\n# Foo"))
		case "/api/v4/projects/group%2Frepo/repository/tree":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"type": "blob", "path": "skills/foo/SKILL.md", "name": "SKILL.md"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	t.Setenv("GITLAB_URL", srv.URL)

	skill, err := fetchFromGitLab(srv.Client(), "tok", srv.URL+"/group/repo/-/tree/main/skills/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skill.name != "Foo Skill" {
		t.Errorf("name = %q, want %q", skill.name, "Foo Skill")
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd server && go test ./internal/handler/... -run "TestFetchFromGitLab" -v 2>&1 | tail -10
```

Expected: FAIL — `fetchFromGitLab` undefined.

- [ ] **Step 3: Implement `fetchFromGitLab` in `skill_gitlab.go`**

Add to `server/internal/handler/skill_gitlab.go`:

```go
import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
)

// fetchFromGitLab fetches a skill from a self-hosted GitLab repo using the
// provided OAuth bearer token. Mirrors fetchFromGitHub in structure.
func fetchFromGitLab(httpClient *http.Client, token, rawURL string) (*importedSkill, error) {
	spec, err := parseGitLabURL(rawURL)
	if err != nil {
		return nil, err
	}

	apiBase := strings.TrimRight(os.Getenv("GITLAB_URL"), "/") + "/api/v4"
	encodedNS := gitlabProjectAPIPath(spec.namespace)

	// Resolve default branch when ref is absent.
	ref := spec.ref
	if ref == "" {
		ref, err = gitlabDefaultBranch(httpClient, token, apiBase, encodedNS)
		if err != nil {
			return nil, fmt.Errorf("gitlab skill: resolve default branch for %s: %w", spec.namespace, err)
		}
	}

	// Fetch SKILL.md.
	skillMdFilePath := "SKILL.md"
	if spec.skillDir != "" {
		skillMdFilePath = spec.skillDir + "/SKILL.md"
	}
	skillMdBody, err := gitlabFetchFile(httpClient, token, apiBase, encodedNS, skillMdFilePath, ref)
	if err != nil {
		return nil, fmt.Errorf("gitlab skill: SKILL.md not found at %s in %s@%s: %w",
			skillMdFilePath, spec.namespace, ref, err)
	}

	name, description := skillpkg.ParseSkillFrontmatter(string(skillMdBody))
	if name == "" {
		if spec.skillDir != "" {
			name = path.Base(spec.skillDir)
		} else {
			parts := strings.Split(spec.namespace, "/")
			name = parts[len(parts)-1]
		}
	}

	result := &importedSkill{
		name:        name,
		description: description,
		content:     string(skillMdBody),
		origin: map[string]any{
			"type":       "gitlab",
			"source_url": rawURL,
			"namespace":  spec.namespace,
			"ref":        ref,
			"path":       spec.skillDir,
		},
	}

	// List directory to collect supporting files.
	entries, err := gitlabListTree(httpClient, token, apiBase, encodedNS, spec.skillDir, ref)
	if err != nil {
		slog.Warn("gitlab skill: failed to list tree, returning SKILL.md only",
			"namespace", spec.namespace, "ref", ref, "err", err)
		return result, nil
	}

	basePrefix := ""
	if spec.skillDir != "" {
		basePrefix = spec.skillDir + "/"
	}
	for _, entry := range entries {
		if entry.Type != "blob" {
			continue
		}
		relPath := strings.TrimPrefix(entry.Path, basePrefix)
		if skillpkg.IsReservedContentPath(relPath) {
			continue
		}
		content, err := gitlabFetchFile(httpClient, token, apiBase, encodedNS, entry.Path, ref)
		if err != nil {
			slog.Warn("gitlab skill: skipping file fetch error", "path", entry.Path, "err", err)
			continue
		}
		if err := result.addFile(relPath, string(content)); err != nil {
			if isCapError(err) {
				return nil, err
			}
			slog.Warn("gitlab skill: addFile skipped", "path", relPath, "err", err)
		}
	}

	return result, nil
}

type gitlabTreeEntry struct {
	Type string `json:"type"` // "blob" or "tree"
	Path string `json:"path"`
	Name string `json:"name"`
}

// gitlabDefaultBranch fetches the project's default branch from the GitLab API.
func gitlabDefaultBranch(httpClient *http.Client, token, apiBase, encodedNS string) (string, error) {
	u := apiBase + "/projects/" + encodedNS
	body, err := gitlabAPIGet(httpClient, token, u)
	if err != nil {
		return "", err
	}
	defer body.Close()
	var proj struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(body).Decode(&proj); err != nil {
		return "", fmt.Errorf("decode project: %w", err)
	}
	if proj.DefaultBranch == "" {
		return "main", nil // sensible fallback
	}
	return proj.DefaultBranch, nil
}

// gitlabListTree lists all blobs under a directory path (recursive).
func gitlabListTree(httpClient *http.Client, token, apiBase, encodedNS, dirPath, ref string) ([]gitlabTreeEntry, error) {
	q := url.Values{
		"ref":       {ref},
		"recursive": {"true"},
		"per_page":  {"100"},
	}
	if dirPath != "" {
		q.Set("path", dirPath)
	}
	u := apiBase + "/projects/" + encodedNS + "/repository/tree?" + q.Encode()
	body, err := gitlabAPIGet(httpClient, token, u)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	var entries []gitlabTreeEntry
	if err := json.NewDecoder(body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode tree: %w", err)
	}
	return entries, nil
}

// gitlabFetchFile fetches a single file's raw content via the repository files API.
func gitlabFetchFile(httpClient *http.Client, token, apiBase, encodedNS, filePath, ref string) ([]byte, error) {
	encodedPath := url.PathEscape(filePath)
	u := apiBase + "/projects/" + encodedNS + "/repository/files/" + encodedPath + "/raw?ref=" + url.QueryEscape(ref)
	body, err := gitlabAPIGet(httpClient, token, u)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return io.ReadAll(io.LimitReader(body, maxImportFileSize))
}

// gitlabAPIGet issues an authenticated GET and returns the body on 2xx.
func gitlabAPIGet(httpClient *http.Client, token, rawURL string) (io.ReadCloser, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, fmt.Errorf("not found (404)")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return resp.Body, nil
}
```

Also add the missing import for `skillpkg` and `maxImportFileSize`. Check `skill.go` for the constant name:

```go
// In skill.go search for the file size cap constant name; it is used in fetchRawFile.
// Add to skill_gitlab.go imports:
import (
    skillpkg "github.com/multica-ai/multica/server/internal/skill"
)
// maxImportFileSize is defined in skill.go — already in scope (same package).
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd server && go test ./internal/handler/... -run "TestFetchFromGitLab" -v 2>&1 | tail -15
```

Expected: PASS both test functions.

- [ ] **Step 5: Run full handler tests to check for regressions**

```bash
cd server && go test ./internal/handler/... -count=1 2>&1 | tail -10
```

Expected: no new failures.

- [ ] **Step 6: Commit**

```bash
cd server && git add internal/handler/skill_gitlab.go internal/handler/skill_gitlab_test.go
git commit -m "feat(skills): implement fetchFromGitLab using repository files API"
```

---

### Task 3: Thread the token through import and refresh handlers

**Files:**
- Modify: `server/internal/handler/skill_gitlab.go` (add `gitlabAccessTokenForWorkspace`)
- Modify: `server/internal/handler/skill.go:1963-1985` (`ImportSkill` switch)
- Modify: `server/internal/handler/skill_refresh.go:42-64` (`RefreshSkillFromURL` switch)

**Interfaces:**
- Consumes: `h.Queries.GetFirstGitLabConnectionByWorkspace`, `h.GitLabBox.Open`, `h.refreshGitLabToken`, `base64.StdEncoding.DecodeString`
- Produces: `(h *Handler) gitlabAccessTokenForWorkspace(ctx, workspaceUUID) (string, error)` — returns decrypted, refreshed-if-expired plain token

- [ ] **Step 1: Add `gitlabAccessTokenForWorkspace` helper to `skill_gitlab.go`**

Add to `server/internal/handler/skill_gitlab.go` (add `"context"`, `"encoding/base64"`, `"time"` to imports):

```go
// gitlabAccessTokenForWorkspace retrieves and decrypts the workspace's GitLab
// OAuth access token, refreshing it first if it has expired.
// Returns an error when no connection exists or decryption fails.
func (h *Handler) gitlabAccessTokenForWorkspace(ctx context.Context, workspaceUUID pgtype.UUID) (string, error) {
	conn, err := h.Queries.GetFirstGitLabConnectionByWorkspace(ctx, workspaceUUID)
	if err != nil {
		return "", fmt.Errorf("gitlab skill: no GitLab connection for workspace: %w", err)
	}

	if conn.TokenExpiresAt.Valid && conn.TokenExpiresAt.Time.Before(time.Now()) {
		return h.refreshGitLabToken(ctx, conn)
	}

	tokenBytes, err := base64.StdEncoding.DecodeString(conn.AccessToken)
	if err != nil {
		return "", fmt.Errorf("gitlab skill: decode token: %w", err)
	}
	plain, err := h.GitLabBox.Open(tokenBytes)
	if err != nil {
		return "", fmt.Errorf("gitlab skill: decrypt token: %w", err)
	}
	return string(plain), nil
}
```

Also add these imports at the top of `skill_gitlab.go` (merge with existing):
```go
import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	skillpkg "github.com/multica-ai/multica/server/internal/skill"
)
```

- [ ] **Step 2: Update `ImportSkill` in `skill.go`**

In `skill.go`, the switch at line ~1972:

```go
// Before:
var imported *importedSkill
switch source {
case sourceClawHub:
    imported, err = fetchFromClawHub(httpClient, normalized)
case sourceSkillsSh:
    imported, err = fetchFromSkillsSh(httpClient, normalized)
case sourceGitHub:
    imported, err = fetchFromGitHub(httpClient, normalized)
}

// After:
var imported *importedSkill
switch source {
case sourceClawHub:
    imported, err = fetchFromClawHub(httpClient, normalized)
case sourceSkillsSh:
    imported, err = fetchFromSkillsSh(httpClient, normalized)
case sourceGitHub:
    imported, err = fetchFromGitHub(httpClient, normalized)
case sourceGitLab:
    var glToken string
    glToken, err = h.gitlabAccessTokenForWorkspace(r.Context(), workspaceUUID)
    if err != nil {
        writeError(w, http.StatusBadRequest, "no GitLab connection found for this workspace — connect GitLab in Settings first")
        return
    }
    imported, err = fetchFromGitLab(httpClient, glToken, normalized)
}
```

- [ ] **Step 3: Update `RefreshSkillFromURL` in `skill_refresh.go`**

Update the error message at line ~38 and add the `sourceGitLab` case in the switch:

```go
// Update the error message (line ~38):
writeError(w, http.StatusBadRequest, "this skill has no source URL to refresh from; only skills imported from GitHub, ClawHub, Skills.sh, or a connected GitLab instance can be updated from URL")

// In the switch at line ~50, add after sourceGitHub case:
case sourceGitLab:
    var glToken string
    glToken, err = h.gitlabAccessTokenForWorkspace(r.Context(), skill.WorkspaceID)
    if err != nil {
        writeError(w, http.StatusBadRequest, "no GitLab connection found for this workspace — connect GitLab in Settings first")
        return
    }
    imported, err = fetchFromGitLab(httpClient, glToken, normalized)
```

- [ ] **Step 4: Verify it compiles**

```bash
cd server && go build ./internal/handler/... 2>&1
```

Expected: no errors.

- [ ] **Step 5: Run all handler tests**

```bash
cd server && go test ./internal/handler/... -count=1 2>&1 | tail -10
```

Expected: all pass (GitLab integration tests skip without DB or `GITLAB_URL`).

- [ ] **Step 6: Commit**

```bash
cd server && git add internal/handler/skill_gitlab.go internal/handler/skill.go internal/handler/skill_refresh.go
git commit -m "feat(skills): thread GitLab OAuth token through import and refresh handlers"
```

---

### Task 4: Update the builtin skill-importing doc

**Files:**
- Modify: `server/internal/service/builtin_skills/multica-skill-importing/SKILL.md`
- Modify: `server/internal/service/builtin_skills/multica-skill-importing/references/skill-importing-source-map.md`

Per CLAUDE.md: when changing product behavior documented by built-in skills, update the relevant `SKILL.md` and `references/*-source-map.md` in the same PR.

- [ ] **Step 1: Read the existing skill-importing docs**

```bash
cat server/internal/service/builtin_skills/multica-skill-importing/SKILL.md
cat server/internal/service/builtin_skills/multica-skill-importing/references/skill-importing-source-map.md
```

- [ ] **Step 2: Add GitLab self-hosted source to `SKILL.md`**

Find the section listing supported import sources (ClawHub, Skills.sh, GitHub). Add:

```markdown
- **Self-hosted GitLab** (`https://your-gitlab-host/group/repo/-/tree/{ref}/{skill-dir}`): requires an active GitLab connection in workspace Settings. Uses the workspace's OAuth token — no separate credentials needed. The URL must match the `GITLAB_URL` configured on the server.
```

Also add a note that refresh works the same way for GitLab-sourced skills.

- [ ] **Step 3: Update `references/skill-importing-source-map.md`**

Add an entry pointing to the relevant handler code:

```markdown
## GitLab self-hosted

- URL detection: `server/internal/handler/skill.go` → `detectImportSource` (`sourceGitLab` case matches `gitlabConfiguredHost()`)
- URL parsing: `server/internal/handler/skill_gitlab.go` → `parseGitLabURL`
- Fetch: `server/internal/handler/skill_gitlab.go` → `fetchFromGitLab`
- Token: `server/internal/handler/skill_gitlab.go` → `gitlabAccessTokenForWorkspace` (reuses workspace OAuth connection)
- Refresh: `server/internal/handler/skill_refresh.go` → `RefreshSkillFromURL` (`sourceGitLab` case)
```

- [ ] **Step 4: Commit**

```bash
git add server/internal/service/builtin_skills/multica-skill-importing/
git commit -m "docs(skills): document self-hosted GitLab as a skill import source"
```
