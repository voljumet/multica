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
