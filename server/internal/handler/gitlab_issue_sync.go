package handler

// GitLab label-triggered issue sync (GitLab → Multica), kept in its own file
// so future upstream merges to vcs.go / vcs_webhook.go / gitlab.go (the
// generic PR/CI mirroring) don't collide with this Multica-specific addon.
// Auth and transport are shared with the generic VCS connection
// (vcs_connection, provider="gitlab") — there is no separate GitLab OAuth
// app, group scope, or dedicated webhook for this feature anymore. Adding a
// workspace's GitLab instance under Settings → Integrations → Git providers
// (server/internal/handler/vcs.go) is enough; the Issue Hook is delivered on
// the same webhook URL used for merge-request/pipeline mirroring, dispatched
// from vcs_webhook.go into handleGitLabIssueSyncWebhook below.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// ── Payload shapes (GitLab Issue Hook) ──────────────────────────────────────

type gitlabIssueLabel struct {
	Title string `json:"title"`
	Color string `json:"color"`
}

type gitlabIssueLabelChange struct {
	Previous []gitlabIssueLabel `json:"previous"`
	Current  []gitlabIssueLabel `json:"current"`
}

type gitlabIssueStateChange struct {
	Previous string `json:"previous"`
	Current  string `json:"current"`
}

// gitlabIssuePayload is the subset of GitLab's Issue Hook webhook we consume.
type gitlabIssuePayload struct {
	ObjectKind       string `json:"object_kind"`
	ObjectAttributes struct {
		IID         int32              `json:"iid"`
		Title       string             `json:"title"`
		Description string             `json:"description"`
		Action      string             `json:"action"`
		State       string             `json:"state"`
		Labels      []gitlabIssueLabel `json:"labels"`
	} `json:"object_attributes"`
	Project   gitlabWebhookProject `json:"project"`
	Labels    []gitlabIssueLabel   `json:"labels"`
	Assignees []struct {
		Username string `json:"username"`
	} `json:"assignees"`
	Changes struct {
		Labels *gitlabIssueLabelChange `json:"labels"`
		State  *gitlabIssueStateChange `json:"state"`
	} `json:"changes"`
}

// gitlabWebhookProject is the project object nested in GitLab webhooks. URL
// fields vary slightly by GitLab version; we accept the common aliases.
type gitlabWebhookProject struct {
	ID                int64  `json:"id"`
	PathWithNamespace string `json:"path_with_namespace"`
	Namespace         string `json:"namespace"`
	WebURL            string `json:"web_url"`
	GitHTTPURL        string `json:"git_http_url"`
	GitSSHURL         string `json:"git_ssh_url"`
	HTTPURL           string `json:"http_url"`
	SSHURL            string `json:"ssh_url"`
	URL               string `json:"url"`
}

// GitLabIssueResponse is the linked-GitLab-issue payload shown in the issue
// sidebar badge.
type GitLabIssueResponse struct {
	GlIssueIID         int32   `json:"gl_issue_iid"`
	ProjectPath        string  `json:"project_path"`
	URL                string  `json:"url"`
	GlAssigneeUsername *string `json:"gl_assignee_username"`
}

// ── Settings ─────────────────────────────────────────────────────────────────

// defaultGitLabIssueSyncLabel is used when workspace settings omit or blank
// gitlab_issue_sync_label. Historical installs always matched the "agent" label.
const defaultGitLabIssueSyncLabel = "agent"

// workspaceGitLabIssueSyncLabel returns the GitLab label title that triggers
// Multica issue creation. Defaults to "agent" when unset or empty.
func workspaceGitLabIssueSyncLabel(settings []byte) string {
	if len(settings) == 0 {
		return defaultGitLabIssueSyncLabel
	}
	var s struct {
		Label *string `json:"gitlab_issue_sync_label"`
	}
	if err := json.Unmarshal(settings, &s); err != nil || s.Label == nil {
		return defaultGitLabIssueSyncLabel
	}
	label := strings.TrimSpace(*s.Label)
	if label == "" {
		return defaultGitLabIssueSyncLabel
	}
	return label
}

// gitlabImportedIssueTitle builds the Multica title for a GitLab-synced issue:
// "{gitlab title} #{iid}" (e.g. "Fix login #42").
func gitlabImportedIssueTitle(title string, iid int32) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Sprintf("#%d", iid)
	}
	return fmt.Sprintf("%s #%d", title, iid)
}

// ── Label matching ───────────────────────────────────────────────────────────

func containsLabel(labels []gitlabIssueLabel, title string) bool {
	for _, l := range labels {
		if l.Title == title {
			return true
		}
	}
	return false
}

// hasGitLabIssueSyncTrigger reports whether the issue should be imported into
// Multica. True when the configured sync label is present exactly, or when any
// label uses the "{syncLabel}::{agentName}" form (e.g. "agent::Implementer"
// with sync label "agent"). A prefixed label alone is enough to create the
// Multica issue; agent assignment is best-effort and may leave it unassigned.
func hasGitLabIssueSyncTrigger(labels []gitlabIssueLabel, syncLabel string) bool {
	if syncLabel == "" {
		return false
	}
	if containsLabel(labels, syncLabel) {
		return true
	}
	prefix := syncLabel + "::"
	for _, l := range labels {
		title := strings.TrimSpace(l.Title)
		if strings.HasPrefix(title, prefix) && len(title) > len(prefix) {
			return true
		}
	}
	return false
}

// gitlabIssueLabels returns the effective label list from an Issue Hook payload.
// Prefer top-level labels (standard); fall back to object_attributes.labels when
// some GitLab versions only nest them there.
func gitlabIssueLabels(p gitlabIssuePayload) []gitlabIssueLabel {
	if len(p.Labels) > 0 {
		return p.Labels
	}
	return p.ObjectAttributes.Labels
}

// isGitLabIssueClosed reports whether this hook closes a GitLab issue.
func isGitLabIssueClosed(action, state string, stateChange *gitlabIssueStateChange) bool {
	if action == "close" {
		return true
	}
	if stateChange == nil {
		return false
	}
	prev := strings.ToLower(strings.TrimSpace(stateChange.Previous))
	curr := strings.ToLower(strings.TrimSpace(stateChange.Current))
	if curr == "" {
		curr = strings.ToLower(strings.TrimSpace(state))
	}
	prevOpen := prev == "opened" || prev == "open"
	currClosed := curr == "closed" || curr == "close"
	return prevOpen && currClosed
}

// isGitLabIssueReopened reports whether this hook reopens a closed GitLab issue.
func isGitLabIssueReopened(action, state string, stateChange *gitlabIssueStateChange) bool {
	if action == "reopen" {
		return true
	}
	if stateChange == nil {
		return false
	}
	prev := strings.ToLower(strings.TrimSpace(stateChange.Previous))
	curr := strings.ToLower(strings.TrimSpace(stateChange.Current))
	if curr == "" {
		curr = strings.ToLower(strings.TrimSpace(state))
	}
	prevClosed := prev == "closed" || prev == "close"
	currOpen := curr == "opened" || curr == "open" || curr == "reopened"
	return prevClosed && currOpen
}

// gitlabSyncLabelRemoved reports whether the configured sync trigger was
// explicitly removed in this webhook. Requires changes.labels so ordinary
// description/title updates never cancel a Multica issue.
func gitlabSyncLabelRemoved(changes *gitlabIssueLabelChange, syncLabel string) bool {
	if changes == nil || syncLabel == "" {
		return false
	}
	had := hasGitLabIssueSyncTrigger(changes.Previous, syncLabel)
	has := hasGitLabIssueSyncTrigger(changes.Current, syncLabel)
	return had && !has
}

// gitlabLabelAgentNameCandidates returns possible Multica agent names encoded
// in GitLab labels.
func gitlabLabelAgentNameCandidates(labels []gitlabIssueLabel, syncLabel string) []string {
	prefix := ""
	if syncLabel != "" {
		prefix = syncLabel + "::"
	}
	var out []string
	seen := map[string]struct{}{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	for _, l := range labels {
		title := strings.TrimSpace(l.Title)
		if title == "" {
			continue
		}
		add(title)
		if prefix != "" && strings.HasPrefix(title, prefix) {
			add(strings.TrimPrefix(title, prefix))
		}
	}
	return out
}

// matchAgentByGitLabLabels picks a workspace user-agent whose name uniquely
// matches a GitLab label (case-insensitive). Returns ok=false when zero or
// multiple distinct agents match — create still proceeds unassigned.
func matchAgentByGitLabLabels(agents []db.Agent, labels []gitlabIssueLabel, syncLabel string) (db.Agent, bool) {
	candidates := gitlabLabelAgentNameCandidates(labels, syncLabel)
	if len(candidates) == 0 {
		return db.Agent{}, false
	}

	byLower := make(map[string][]db.Agent)
	for _, a := range agents {
		if a.Kind != "" && a.Kind != "user" {
			continue
		}
		if a.ArchivedAt.Valid {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(a.Name))
		if key == "" {
			continue
		}
		byLower[key] = append(byLower[key], a)
	}

	matched := make(map[string]db.Agent)
	for _, cand := range candidates {
		group := byLower[strings.ToLower(cand)]
		if len(group) == 0 {
			continue
		}
		var pick db.Agent
		if len(group) == 1 {
			pick = group[0]
		} else {
			exact := false
			for _, a := range group {
				if a.Name == cand {
					pick = a
					exact = true
					break
				}
			}
			if !exact {
				continue
			}
		}
		matched[uuidToString(pick.ID)] = pick
	}

	if len(matched) != 1 {
		return db.Agent{}, false
	}
	for _, a := range matched {
		return a, true
	}
	return db.Agent{}, false
}

// gitlabDefaultLabelColor is used when a GitLab label carries no usable color.
const gitlabDefaultLabelColor = "#6b7280"

// syncGitLabLabelsToIssue ensures a Multica issue label exists for each GitLab
// label (matched case-insensitively by name) and attaches it to the issue. The
// sync trigger label ("{syncLabel}" or "{syncLabel}::agent") is control
// metadata, not content, and is skipped.
// ponytail: add-only sync; detaching removed labels needs changes.labels handling.
func (h *Handler) syncGitLabLabelsToIssue(ctx context.Context, workspaceID, issueID pgtype.UUID, labels []gitlabIssueLabel, syncLabel string) {
	existing, err := h.Queries.ListLabels(ctx, db.ListLabelsParams{
		WorkspaceID:  workspaceID,
		ResourceType: "issue",
	})
	if err != nil {
		slog.Warn("gitlab: list labels for sync failed", "workspace", uuidToString(workspaceID), "err", err)
		return
	}
	byName := make(map[string]pgtype.UUID, len(existing))
	for _, l := range existing {
		byName[strings.ToLower(l.Name)] = l.ID
	}

	syncPrefix := strings.ToLower(syncLabel) + "::"
	for _, gl := range labels {
		name, err := validateLabelName(gl.Title)
		if err != nil {
			slog.Warn("gitlab: skipping label with invalid name", "label", gl.Title, "err", err)
			continue
		}
		lower := strings.ToLower(name)
		if lower == strings.ToLower(syncLabel) || strings.HasPrefix(lower, syncPrefix) {
			continue
		}
		id, ok := byName[lower]
		if !ok {
			color := gitlabDefaultLabelColor
			if c, err := normalizeColor(gl.Color); err == nil {
				color = c
			}
			created, err := h.Queries.CreateLabel(ctx, db.CreateLabelParams{
				WorkspaceID:  workspaceID,
				ResourceType: "issue",
				Name:         name,
				Color:        color,
			})
			if err != nil {
				slog.Warn("gitlab: create label failed", "label", name, "err", err)
				continue
			}
			id = created.ID
			byName[lower] = id
		}
		if _, err := h.Queries.AttachLabelToIssue(ctx, db.AttachLabelToIssueParams{
			IssueID:     issueID,
			LabelID:     id,
			WorkspaceID: workspaceID,
		}); err != nil {
			slog.Warn("gitlab: attach label failed", "label", name, "issue_id", uuidToString(issueID), "err", err)
		}
	}
}

// ── Repo matching (Issue Hook project → Multica project) ────────────────────

func gitlabProjectCandidateURLs(p gitlabWebhookProject, instanceURL string) []string {
	raw := []string{
		p.GitHTTPURL,
		p.GitSSHURL,
		p.WebURL,
		p.HTTPURL,
		p.SSHURL,
		p.URL,
	}
	if base := strings.TrimRight(instanceURL, "/"); base != "" && p.PathWithNamespace != "" {
		raw = append(raw, base+"/"+strings.Trim(p.PathWithNamespace, "/"))
		raw = append(raw, base+"/"+strings.Trim(p.PathWithNamespace, "/")+".git")
	}
	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func gitRepoMatchesGitLabProject(resourceURL, pathKey string, candidateURLs []string) bool {
	rHost, rPath := splitGitRemote(resourceURL)
	if rPath == "" {
		return false
	}
	for _, c := range candidateURLs {
		cHost, cPath := splitGitRemote(c)
		if cPath == "" {
			continue
		}
		if rPath != cPath {
			continue
		}
		if cHost != "" && rHost != "" && cHost != rHost {
			continue
		}
		return true
	}
	return pathKey != "" && rPath == pathKey
}

// splitGitRemote extracts a comparable host + path from a git remote URL.
// Supports https://, http://, ssh://, git://, and scp-like git@host:path forms.
func splitGitRemote(raw string) (host, path string) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimRight(raw, "/")
	if raw == "" {
		return "", ""
	}
	if u, err := url.Parse(raw); err == nil && u.Scheme != "" && u.Host != "" {
		host = strings.ToLower(u.Hostname())
		path = normalizeGitRepoPath(u.Path)
		return host, path
	}
	s := raw
	if i := strings.Index(s, "@"); i >= 0 && !strings.Contains(s[:i], "://") {
		s = s[i+1:]
	}
	if i := strings.Index(s, ":"); i > 0 {
		host = strings.ToLower(s[:i])
		if j := strings.Index(host, "/"); j >= 0 {
			host = host[:j]
		}
		path = normalizeGitRepoPath(s[i+1:])
		return host, path
	}
	return "", normalizeGitRepoPath(raw)
}

// normalizeGitRepoPath lowercases a repo path, trims slashes, and drops a
// trailing ".git" suffix.
func normalizeGitRepoPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, "/")
	p = strings.TrimSuffix(p, ".git")
	p = strings.Trim(p, "/")
	return strings.ToLower(p)
}

// githubRepoURLFromResourceRef extracts resource_ref.url for a github_repo row.
func githubRepoURLFromResourceRef(ref []byte) (string, bool) {
	var payload struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(ref, &payload); err != nil {
		return "", false
	}
	u := strings.TrimSpace(payload.URL)
	if u == "" {
		return "", false
	}
	return u, true
}

// resolveMulticaProjectForGitLabRepo finds a Multica project in the workspace
// whose attached github_repo resource matches the GitLab project from a
// webhook. Returns an invalid UUID when no project matches (issue is created
// at workspace scope).
func (h *Handler) resolveMulticaProjectForGitLabRepo(ctx context.Context, workspaceID pgtype.UUID, instanceURL string, glProject gitlabWebhookProject) pgtype.UUID {
	resources, err := h.Queries.ListGithubRepoProjectResourcesByWorkspace(ctx, workspaceID)
	if err != nil {
		slog.Warn("gitlab: failed to list project resources for repo match",
			"err", err, "workspace", uuidToString(workspaceID))
		return pgtype.UUID{}
	}
	if len(resources) == 0 {
		return pgtype.UUID{}
	}

	candidates := gitlabProjectCandidateURLs(glProject, instanceURL)
	pathKey := normalizeGitRepoPath(glProject.PathWithNamespace)

	var matched pgtype.UUID
	for _, res := range resources {
		repoURL, ok := githubRepoURLFromResourceRef(res.ResourceRef)
		if !ok {
			continue
		}
		if !gitRepoMatchesGitLabProject(repoURL, pathKey, candidates) {
			continue
		}
		if matched.Valid {
			slog.Warn("gitlab: multiple multica projects match gitlab repo; using first",
				"workspace", uuidToString(workspaceID), "path_with_namespace", glProject.PathWithNamespace)
			continue
		}
		matched = res.ProjectID
	}
	return matched
}

// ── Status transitions ───────────────────────────────────────────────────────

// setGitLabLinkedIssueStatus updates a Multica issue linked from GitLab and
// publishes issue:updated. Used for reopen and sync-label remove/restore.
func (h *Handler) setGitLabLinkedIssueStatus(ctx context.Context, issue db.Issue, workspaceID, status, source string) {
	if issue.Status == status {
		return
	}
	updated, err := h.Queries.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{
		ID:          issue.ID,
		Status:      status,
		WorkspaceID: issue.WorkspaceID,
	})
	if err != nil {
		slog.Warn("gitlab: failed to update issue status",
			"err", err, "issue_id", uuidToString(issue.ID), "status", status, "source", source)
		return
	}
	prefix := h.getIssuePrefix(ctx, issue.WorkspaceID)
	h.publish(protocol.EventIssueUpdated, workspaceID, "system", "", map[string]any{
		"issue":          issueToResponse(updated, prefix),
		"status_changed": true,
		"prev_status":    issue.Status,
		"source":         source,
	})
}

// ── Connection + creator lookup (backed by vcs_connection) ──────────────────

// firstGitLabVCSConnection returns the workspace's gitlab-provider VCS
// connection. Multiple GitLab instances per workspace are not supported by
// this feature (nor by the generic VCS tab); the first one wins.
func (h *Handler) firstGitLabVCSConnection(ctx context.Context, workspaceID pgtype.UUID) (db.VcsConnection, error) {
	conns, err := h.Queries.ListVCSConnectionsByWorkspace(ctx, workspaceID)
	if err != nil {
		return db.VcsConnection{}, err
	}
	for _, c := range conns {
		if c.Provider == "gitlab" {
			return c, nil
		}
	}
	return db.VcsConnection{}, pgx.ErrNoRows
}

// vcsConnectionCreatorID returns the user UUID to use as creator for
// webhook-triggered issue creation. Prefers the connection's connected_by_id;
// falls back to the first workspace member.
func (h *Handler) vcsConnectionCreatorID(ctx context.Context, conn db.VcsConnection) (pgtype.UUID, bool) {
	if conn.ConnectedByID.Valid {
		return conn.ConnectedByID, true
	}
	members, err := h.Queries.ListMembers(ctx, conn.WorkspaceID)
	if err != nil || len(members) == 0 {
		return pgtype.UUID{}, false
	}
	return members[0].UserID, true
}

// gitlabAccessTokenFromVCSConnection decrypts the access token stored on a
// gitlab-provider vcs_connection.
func (h *Handler) gitlabAccessTokenFromVCSConnection(conn db.VcsConnection) (string, error) {
	return h.openVCSSecret(conn.AccessTokenEncrypted)
}

// ── Issue Hook handling ──────────────────────────────────────────────────────

// handleGitLabIssueSyncWebhook creates/updates a Multica issue from a GitLab
// Issue Hook event when the configured sync trigger label is present. See the
// package doc comment above for how this reaches here from vcs_webhook.go.
func (h *Handler) handleGitLabIssueSyncWebhook(ctx context.Context, conn db.VcsConnection, body []byte) {
	var p gitlabIssuePayload
	if err := json.Unmarshal(body, &p); err != nil {
		slog.Error("gitlab: failed to parse issue payload", "err", err)
		return
	}

	projectPath := p.Project.PathWithNamespace
	action := p.ObjectAttributes.Action
	state := p.ObjectAttributes.State
	workspaceID := uuidToString(conn.WorkspaceID)
	glIID := p.ObjectAttributes.IID
	labels := gitlabIssueLabels(p)

	syncLabel := defaultGitLabIssueSyncLabel
	if ws, err := h.Queries.GetWorkspace(ctx, conn.WorkspaceID); err == nil {
		syncLabel = workspaceGitLabIssueSyncLabel(ws.Settings)
	}
	hasSyncLabel := hasGitLabIssueSyncTrigger(labels, syncLabel)
	syncLabelRemoved := gitlabSyncLabelRemoved(p.Changes.Labels, syncLabel)
	closed := isGitLabIssueClosed(action, state, p.Changes.State)
	reopened := isGitLabIssueReopened(action, state, p.Changes.State)

	assigneeUsername := ""
	if len(p.Assignees) > 0 {
		assigneeUsername = p.Assignees[0].Username
	}

	row, rowErr := h.Queries.GetGitLabIssueByProjectAndIID(ctx, db.GetGitLabIssueByProjectAndIIDParams{
		WorkspaceID: conn.WorkspaceID,
		ProjectPath: projectPath,
		GlIssueIid:  glIID,
	})
	rowExists := rowErr == nil

	slog.Info("gitlab: issue hook",
		"workspace", workspaceID, "project", projectPath, "gl_iid", glIID,
		"action", action, "state", state, "sync_label", syncLabel,
		"has_sync_label", hasSyncLabel, "sync_label_removed", syncLabelRemoved,
		"closed", closed, "reopened", reopened, "already_linked", rowExists,
	)

	if hasSyncLabel {
		if !rowExists && (action == "open" || action == "update") {
			creatorID, ok := h.vcsConnectionCreatorID(ctx, conn)
			if !ok {
				slog.Error("gitlab: no creator available, skipping issue creation",
					"workspace", workspaceID, "project", projectPath, "gl_iid", glIID)
				return
			}

			projectID := h.resolveMulticaProjectForGitLabRepo(ctx, conn.WorkspaceID, conn.InstanceUrl, p.Project)

			createParams := service.IssueCreateParams{
				WorkspaceID:    conn.WorkspaceID,
				Title:          gitlabImportedIssueTitle(p.ObjectAttributes.Title, glIID),
				Description:    pgtype.Text{String: p.ObjectAttributes.Description, Valid: p.ObjectAttributes.Description != ""},
				Status:         "todo",
				Priority:       "none",
				CreatorType:    "member",
				CreatorID:      creatorID,
				ProjectID:      projectID,
				AllowDuplicate: true,
			}
			var assignedAgentID string
			if agents, err := h.Queries.ListAgents(ctx, conn.WorkspaceID); err != nil {
				slog.Warn("gitlab: list agents for label assign failed", "workspace", workspaceID, "err", err)
			} else if agent, ok := matchAgentByGitLabLabels(agents, labels, syncLabel); ok {
				createParams.AssigneeType = pgtype.Text{String: "agent", Valid: true}
				createParams.AssigneeID = agent.ID
				assignedAgentID = uuidToString(agent.ID)
			}

			res, err := h.IssueService.Create(ctx, createParams, service.IssueCreateOpts{
				AnalyticsAgentID: assignedAgentID,
			})
			if err != nil {
				slog.Error("gitlab: failed to create issue", "err", err, "workspace", workspaceID, "project", projectPath, "gl_iid", glIID)
				return
			}

			glRow, err := h.Queries.InsertGitLabIssue(ctx, db.InsertGitLabIssueParams{
				WorkspaceID:        conn.WorkspaceID,
				ConnectionID:       conn.ID,
				ProjectPath:        projectPath,
				GlIssueIid:         glIID,
				GlProjectID:        p.Project.ID,
				IssueID:            res.Issue.ID,
				GlAssigneeUsername: pgtype.Text{String: assigneeUsername, Valid: assigneeUsername != ""},
			})
			if err != nil {
				slog.Error("gitlab: failed to insert gitlab_issue row", "err", err, "workspace", workspaceID, "issue_id", uuidToString(res.Issue.ID))
				return
			}
			row = glRow
			rowExists = true
			h.syncGitLabLabelsToIssue(ctx, conn.WorkspaceID, res.Issue.ID, labels, syncLabel)
			slog.Info("gitlab: created multica issue from sync label",
				"workspace", workspaceID, "project", projectPath, "gl_iid", glIID,
				"issue_id", uuidToString(res.Issue.ID), "assignee_agent_id", assignedAgentID)

		} else if rowExists {
			if err := h.Queries.UpdateIssueDescription(ctx, db.UpdateIssueDescriptionParams{
				ID:          row.IssueID,
				Description: pgtype.Text{String: p.ObjectAttributes.Description, Valid: p.ObjectAttributes.Description != ""},
			}); err != nil {
				slog.Warn("gitlab: failed to sync description", "err", err, "issue_id", uuidToString(row.IssueID))
			}
			if err := h.Queries.UpdateGitLabIssueAssignee(ctx, db.UpdateGitLabIssueAssigneeParams{
				ID:                 row.ID,
				GlAssigneeUsername: pgtype.Text{String: assigneeUsername, Valid: assigneeUsername != ""},
			}); err != nil {
				slog.Warn("gitlab: failed to sync assignee", "err", err, "issue_id", uuidToString(row.IssueID))
			}
			h.syncGitLabLabelsToIssue(ctx, conn.WorkspaceID, row.IssueID, labels, syncLabel)
		}
	}

	if rowExists {
		issue, err := h.Queries.GetIssue(ctx, row.IssueID)
		if err != nil {
			slog.Warn("gitlab: issue not found for status transition", "issue_id", uuidToString(row.IssueID))
			return
		}
		switch {
		case closed:
			h.advanceIssueToDone(ctx, issue, workspaceID, "gitlab_issue_closed")
		case reopened:
			h.setGitLabLinkedIssueStatus(ctx, issue, workspaceID, "in_progress", "gitlab_issue_reopened")
		case syncLabelRemoved:
			if issue.Status != "done" && issue.Status != "cancelled" {
				h.setGitLabLinkedIssueStatus(ctx, issue, workspaceID, "cancelled", "gitlab_sync_label_removed")
			}
		case hasSyncLabel && issue.Status == "cancelled":
			h.setGitLabLinkedIssueStatus(ctx, issue, workspaceID, "todo", "gitlab_sync_label_restored")
		}
	}
}

// ── Agent → GitLab comment relay (dual-write) ────────────────────────────────
//
// Multica never imports GitLab notes anymore (that was comment-import,
// removed with the OAuth-era gitlab.go). This is the other direction: an
// agent working a GitLab-synced issue posts a comment in Multica, and that
// comment is best-effort relayed to the linked GitLab issue as a note so the
// human who filed it sees the agent's progress without leaving GitLab. Wired
// as service.TaskService.PostCommentToGitLab in handler.go.

// gitlabNoteRelaySentinel marks a GitLab note as Multica-originated so an
// autopilot webhook watching Note Hook events doesn't re-trigger on its own
// relayed output (see isGitLabRelayNote in autopilot_webhook.go).
const gitlabNoteRelaySentinel = "<!-- multica:gitlab-relay -->"

// AppendGitLabNoteRelaySentinel appends the echo-prevention sentinel to a
// GitLab note body posted by Multica-controlled code.
func AppendGitLabNoteRelaySentinel(body string) string {
	body = strings.TrimRight(body, "\n")
	if strings.Contains(body, gitlabNoteRelaySentinel) {
		return body
	}
	return body + "\n\n" + gitlabNoteRelaySentinel
}

// postCommentToGitLab relays a newly-created Multica comment to the linked
// GitLab issue as a note, using the workspace's gitlab vcs_connection.
// Best-effort: errors are logged but not surfaced.
func (h *Handler) postCommentToGitLab(ctx context.Context, comment db.Comment, issue db.Issue) {
	glIssue, err := h.Queries.GetGitLabIssueByIssueID(ctx, issue.ID)
	if err != nil {
		return // issue not linked to GitLab — nothing to relay
	}
	conn, err := h.Queries.GetVCSConnectionByID(ctx, glIssue.ConnectionID)
	if err != nil {
		slog.Warn("gitlab relay: connection not found", "connection_id", uuidToString(glIssue.ConnectionID), "error", err)
		return
	}
	token, err := h.gitlabAccessTokenFromVCSConnection(conn)
	if err != nil {
		slog.Warn("gitlab relay: token decrypt failed", "error", err)
		return
	}

	body := AppendGitLabNoteRelaySentinel(comment.Content)
	payload, _ := json.Marshal(map[string]string{"body": body})
	apiURL := strings.TrimRight(conn.InstanceUrl, "/") + fmt.Sprintf("/api/v4/projects/%d/issues/%d/notes", glIssue.GlProjectID, glIssue.GlIssueIid)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(string(payload)))
	if err != nil {
		slog.Warn("gitlab relay: build request failed", "error", err)
		return
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Warn("gitlab relay: post note failed", "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Warn("gitlab relay: post note returned error", "status", resp.StatusCode)
		return
	}
	var note struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&note); err != nil || note.ID == 0 {
		return
	}
	_ = h.Queries.SetCommentGitLabNoteID(ctx, db.SetCommentGitLabNoteIDParams{
		ID:           comment.ID,
		GitlabNoteID: pgtype.Int8{Int64: note.ID, Valid: true},
	})
}

// ── HTTP handlers (issue-link badge in the issue sidebar) ───────────────────

// LinkGitLabIssueForIssue (PUT /api/issues/{id}/gitlab-issue) manually links a
// Multica issue to a GitLab issue using the workspace's gitlab vcs_connection.
func (h *Handler) LinkGitLabIssueForIssue(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	var body struct {
		ProjectPath string `json:"project_path"`
		GlIssueIID  int32  `json:"gl_issue_iid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProjectPath == "" || body.GlIssueIID == 0 {
		writeError(w, http.StatusBadRequest, "project_path and gl_issue_iid are required")
		return
	}

	conn, err := h.firstGitLabVCSConnection(r.Context(), issue.WorkspaceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "no gitlab connection for workspace")
		return
	}
	token, err := h.gitlabAccessTokenFromVCSConnection(conn)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to decrypt token")
		return
	}

	encodedPath := url.PathEscape(body.ProjectPath)
	apiURL := strings.TrimRight(conn.InstanceUrl, "/") + fmt.Sprintf("/api/v4/projects/%s/issues/%d", encodedPath, body.GlIssueIID)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, apiURL, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build gitlab request")
		return
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "gitlab api request failed")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		writeError(w, http.StatusNotFound, "gitlab issue not found")
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeError(w, http.StatusBadGateway, "gitlab api returned error")
		return
	}
	var glIssue struct {
		ProjectID int64 `json:"project_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&glIssue); err != nil || glIssue.ProjectID == 0 {
		writeError(w, http.StatusBadGateway, "failed to parse gitlab issue")
		return
	}

	row, err := h.Queries.InsertGitLabIssue(r.Context(), db.InsertGitLabIssueParams{
		WorkspaceID:  issue.WorkspaceID,
		ConnectionID: conn.ID,
		ProjectPath:  body.ProjectPath,
		GlIssueIid:   body.GlIssueIID,
		GlProjectID:  glIssue.ProjectID,
		IssueID:      issue.ID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to link gitlab issue")
		return
	}

	issueURL := strings.TrimRight(conn.InstanceUrl, "/") + "/" + row.ProjectPath + "/-/issues/" + strconv.Itoa(int(row.GlIssueIid))
	writeJSON(w, http.StatusOK, GitLabIssueResponse{
		GlIssueIID:  row.GlIssueIid,
		ProjectPath: row.ProjectPath,
		URL:         issueURL,
	})
}

// UnlinkGitLabIssueForIssue (DELETE /api/issues/{id}/gitlab-issue) removes the
// manual or auto-created link between a Multica issue and a GitLab issue.
func (h *Handler) UnlinkGitLabIssueForIssue(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	if err := h.Queries.DeleteGitLabIssueByIssueID(r.Context(), db.DeleteGitLabIssueByIssueIDParams{
		IssueID:     issue.ID,
		WorkspaceID: issue.WorkspaceID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to unlink gitlab issue")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetGitLabIssueForIssue (GET /api/issues/{id}/gitlab-issue) returns the linked
// GitLab issue info for display in the sidebar, or 404 if none.
func (h *Handler) GetGitLabIssueForIssue(w http.ResponseWriter, r *http.Request) {
	issueID := chi.URLParam(r, "id")
	issueUUID, ok := parseUUIDOrBadRequest(w, issueID, "issue id")
	if !ok {
		return
	}

	glIssue, err := h.Queries.GetGitLabIssueByIssueID(r.Context(), issueUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "no gitlab issue linked")
		return
	}

	instanceURL := ""
	if conn, err := h.Queries.GetVCSConnectionByID(r.Context(), glIssue.ConnectionID); err == nil {
		instanceURL = conn.InstanceUrl
	}

	issueURL := strings.TrimRight(instanceURL, "/") + "/" + glIssue.ProjectPath + "/-/issues/" + strconv.Itoa(int(glIssue.GlIssueIid))
	writeJSON(w, http.StatusOK, GitLabIssueResponse{
		GlIssueIID:         glIssue.GlIssueIid,
		ProjectPath:        glIssue.ProjectPath,
		URL:                issueURL,
		GlAssigneeUsername: textToPtr(glIssue.GlAssigneeUsername),
	})
}
