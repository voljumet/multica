package handler

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

	skillpkg "github.com/multica-ai/multica/server/internal/skill"
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
//	gitlab.host/group/repo                         → root, default branch
//	gitlab.host/group/repo/-/tree/{ref}/{path...}  → with dash-separator
//	gitlab.host/group/repo/tree/{ref}/{path...}    → without dash-separator
//	gitlab.host/group/sub/repo/-/tree/{ref}/{path} → nested namespace
func parseGitLabURL(raw string) (gitlabSpec, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return gitlabSpec{}, fmt.Errorf("invalid URL: %w", err)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return gitlabSpec{}, fmt.Errorf("expected gitlab.host/{namespace}/{repo}[/-/tree/{ref}/{path}], got: %s", parsed.Path)
	}

	// Find "-" (GitLab dash separator /-/tree/) or bare "tree" to split
	// namespace from ref/path. GitLab canonical form uses /-/tree/; older
	// versions and some mirror hosts use /tree/ directly.
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

	// Bare repo URL (no tree segment): namespace is the whole path.
	if dashIdx < 0 && treeIdx < 0 {
		ns := strings.Join(parts, "/")
		ns = strings.TrimSuffix(ns, ".git")
		return gitlabSpec{namespace: ns}, nil
	}

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

// fetchFromGitLab fetches a skill from a self-hosted GitLab repo using the
// provided OAuth bearer token. Mirrors fetchFromGitHub in structure.
func fetchFromGitLab(httpClient *http.Client, token, rawURL string) (*importedSkill, error) {
	spec, err := parseGitLabURL(rawURL)
	if err != nil {
		return nil, err
	}

	apiBase := strings.TrimRight(os.Getenv("GITLAB_URL"), "/") + "/api/v4"
	encodedNS := gitlabProjectAPIPath(spec.namespace)

	ref := spec.ref
	if ref == "" {
		ref, err = gitlabDefaultBranch(httpClient, token, apiBase, encodedNS)
		if err != nil {
			return nil, fmt.Errorf("gitlab skill: resolve default branch for %s: %w", spec.namespace, err)
		}
	}

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
	Type string `json:"type"`
	Path string `json:"path"`
	Name string `json:"name"`
}

// gitlabDefaultBranch fetches the project's default branch from the GitLab API.
func gitlabDefaultBranch(httpClient *http.Client, token, apiBase, encodedNS string) (string, error) {
	body, err := gitlabAPIGet(httpClient, token, apiBase+"/projects/"+encodedNS)
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
		return "main", nil
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
	body, err := gitlabAPIGet(httpClient, token, apiBase+"/projects/"+encodedNS+"/repository/tree?"+q.Encode())
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
	u := apiBase + "/projects/" + encodedNS + "/repository/files/" + url.PathEscape(filePath) + "/raw?ref=" + url.QueryEscape(ref)
	body, err := gitlabAPIGet(httpClient, token, u)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return io.ReadAll(io.LimitReader(body, maxImportFileSize))
}

// gitlabAPIGet issues an authenticated GET and returns the response body on 2xx.
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
