package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// ── Pure-function tests ──────────────────────────────────────────────────────

func TestWorkspaceGitLabIssueSyncLabel(t *testing.T) {
	if got := workspaceGitLabIssueSyncLabel(nil); got != "agent" {
		t.Fatalf("nil settings: got %q", got)
	}
	if got := workspaceGitLabIssueSyncLabel([]byte(`{}`)); got != "agent" {
		t.Fatalf("empty object: got %q", got)
	}
	if got := workspaceGitLabIssueSyncLabel([]byte(`{"gitlab_issue_sync_label":""}`)); got != "agent" {
		t.Fatalf("blank label: got %q", got)
	}
	if got := workspaceGitLabIssueSyncLabel([]byte(`{"gitlab_issue_sync_label":"  multica  "}`)); got != "multica" {
		t.Fatalf("custom label: got %q", got)
	}
}

func TestGitLabImportedIssueTitle(t *testing.T) {
	if got := gitlabImportedIssueTitle("Fix login", 42); got != "Fix login #42" {
		t.Fatalf("got %q, want %q", got, "Fix login #42")
	}
	if got := gitlabImportedIssueTitle("  spaced  ", 7); got != "spaced #7" {
		t.Fatalf("trim: got %q, want %q", got, "spaced #7")
	}
	if got := gitlabImportedIssueTitle("", 99); got != "#99" {
		t.Fatalf("empty title: got %q, want %q", got, "#99")
	}
	if got := gitlabImportedIssueTitle("   ", 3); got != "#3" {
		t.Fatalf("blank title: got %q, want %q", got, "#3")
	}
}

func TestGitLabLabelAgentNameCandidates(t *testing.T) {
	labels := []gitlabIssueLabel{
		{Title: "agent"},
		{Title: "agent::Coder"},
		{Title: "  Research  "},
	}
	got := gitlabLabelAgentNameCandidates(labels, "agent")
	want := []string{"agent", "agent::Coder", "Coder", "Research"}
	if len(got) != len(want) {
		t.Fatalf("candidates: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidates[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func testGitLabLabels(titles ...string) []gitlabIssueLabel {
	out := make([]gitlabIssueLabel, len(titles))
	for i, ti := range titles {
		out[i].Title = ti
	}
	return out
}

func TestHasGitLabIssueSyncTrigger(t *testing.T) {
	if !hasGitLabIssueSyncTrigger(testGitLabLabels("agent"), "agent") {
		t.Fatal("bare sync label should trigger")
	}
	if !hasGitLabIssueSyncTrigger(testGitLabLabels("agent::Implementer"), "agent") {
		t.Fatal("prefixed agent name alone should trigger import")
	}
	if !hasGitLabIssueSyncTrigger(testGitLabLabels("bug", "agent::Coder"), "agent") {
		t.Fatal("prefixed label among others should trigger")
	}
	if hasGitLabIssueSyncTrigger(testGitLabLabels("agent:"), "agent") {
		t.Fatal("single-colon form should not trigger")
	}
	if hasGitLabIssueSyncTrigger(testGitLabLabels("agent::"), "agent") {
		t.Fatal("empty name after prefix should not trigger")
	}
	if hasGitLabIssueSyncTrigger(testGitLabLabels("Implementer"), "agent") {
		t.Fatal("agent name alone without sync prefix should not trigger")
	}
	if hasGitLabIssueSyncTrigger(testGitLabLabels("agents"), "agent") {
		t.Fatal("unrelated label should not trigger")
	}
}

func TestGitLabSyncLabelRemoved(t *testing.T) {
	if gitlabSyncLabelRemoved(nil, "agent") {
		t.Fatal("nil changes must not cancel")
	}
	if !gitlabSyncLabelRemoved(&gitlabIssueLabelChange{
		Previous: testGitLabLabels("agent", "bug"),
		Current:  testGitLabLabels("bug"),
	}, "agent") {
		t.Fatal("removing bare sync label should count as removed")
	}
	if !gitlabSyncLabelRemoved(&gitlabIssueLabelChange{
		Previous: testGitLabLabels("agent::Coder"),
		Current:  nil,
	}, "agent") {
		t.Fatal("removing prefixed sync label should count as removed")
	}
	if gitlabSyncLabelRemoved(&gitlabIssueLabelChange{
		Previous: testGitLabLabels("bug"),
		Current:  testGitLabLabels("bug", "feature"),
	}, "agent") {
		t.Fatal("unrelated label changes must not count as sync remove")
	}
	if gitlabSyncLabelRemoved(&gitlabIssueLabelChange{
		Previous: testGitLabLabels("agent"),
		Current:  testGitLabLabels("agent", "bug"),
	}, "agent") {
		t.Fatal("still present after other labels change must not count as removed")
	}
}

func TestIsGitLabIssueReopened(t *testing.T) {
	if !isGitLabIssueReopened("reopen", "opened", nil) {
		t.Fatal("action=reopen should count")
	}
	if isGitLabIssueReopened("update", "opened", nil) {
		t.Fatal("update without state change should not count as reopen")
	}
	if !isGitLabIssueReopened("update", "opened", &gitlabIssueStateChange{Previous: "closed", Current: "opened"}) {
		t.Fatal("closed→opened state change should count as reopen")
	}
}

func TestMatchAgentByGitLabLabels(t *testing.T) {
	coderID := parseUUID("11111111-1111-1111-1111-111111111111")
	researchID := parseUUID("22222222-2222-2222-2222-222222222222")
	agents := []db.Agent{
		{ID: coderID, Name: "Coder", Kind: "user"},
		{ID: researchID, Name: "Research", Kind: "user"},
		{ID: parseUUID("33333333-3333-3333-3333-333333333333"), Name: "Persona", Kind: "system"},
	}
	if _, ok := matchAgentByGitLabLabels(agents, testGitLabLabels("agent"), "agent"); ok {
		t.Fatal("sync label alone should not assign when no agent is named agent")
	}
	got, ok := matchAgentByGitLabLabels(agents, testGitLabLabels("agent", "agent::Coder"), "agent")
	if !ok || uuidToString(got.ID) != uuidToString(coderID) {
		t.Fatalf("prefixed name: ok=%v agent=%q", ok, got.Name)
	}
	got, ok = matchAgentByGitLabLabels(agents, testGitLabLabels("agent", "Research"), "agent")
	if !ok || uuidToString(got.ID) != uuidToString(researchID) {
		t.Fatalf("exact name: ok=%v agent=%q", ok, got.Name)
	}
	if _, ok := matchAgentByGitLabLabels(agents, testGitLabLabels("agent", "Coder", "Research"), "agent"); ok {
		t.Fatal("two agent-name labels should be ambiguous")
	}
	if _, ok := matchAgentByGitLabLabels(agents, testGitLabLabels("agent", "Persona"), "agent"); ok {
		t.Fatal("system persona must not be assigned")
	}
	got, ok = matchAgentByGitLabLabels(agents, testGitLabLabels("agent", "coder"), "agent")
	if !ok || uuidToString(got.ID) != uuidToString(coderID) {
		t.Fatalf("case-insensitive: ok=%v agent=%q", ok, got.Name)
	}
}

func TestSplitGitRemote(t *testing.T) {
	tests := []struct {
		raw      string
		wantHost string
		wantPath string
	}{
		{"https://git.example.com/group/repo.git", "git.example.com", "group/repo"},
		{"https://git.example.com/group/repo", "git.example.com", "group/repo"},
		{"git@git.example.com:group/repo.git", "git.example.com", "group/repo"},
		{"ssh://git@git.example.com/group/sub/repo.git", "git.example.com", "group/sub/repo"},
		{"group/repo", "", "group/repo"},
		{"https://git.example.com:8443/Group/Repo.GIT", "git.example.com", "group/repo"},
		{"", "", ""},
	}
	for _, tc := range tests {
		host, path := splitGitRemote(tc.raw)
		if host != tc.wantHost || path != tc.wantPath {
			t.Errorf("splitGitRemote(%q) = (%q, %q), want (%q, %q)",
				tc.raw, host, path, tc.wantHost, tc.wantPath)
		}
	}
}

func TestGitRepoMatchesGitLabProject(t *testing.T) {
	pathKey := "paral/app"
	candidates := []string{
		"https://git.paral.no/paral/app.git",
		"git@git.paral.no:paral/app.git",
	}

	if !gitRepoMatchesGitLabProject("https://git.paral.no/paral/app", pathKey, candidates) {
		t.Error("expected https resource URL to match candidate")
	}
	if !gitRepoMatchesGitLabProject("git@git.paral.no:paral/app.git", pathKey, candidates) {
		t.Error("expected scp-style resource URL to match candidate")
	}
	if !gitRepoMatchesGitLabProject("https://git.paral.no/paral/app.git", pathKey, nil) {
		t.Error("expected path-only match against path_with_namespace")
	}
	if gitRepoMatchesGitLabProject("https://github.com/paral/app.git", pathKey, candidates) {
		t.Error("expected different host to not match when candidates carry host")
	}
	if gitRepoMatchesGitLabProject("https://git.paral.no/paral/other.git", pathKey, candidates) {
		t.Error("expected different path to not match")
	}
}

func TestAppendGitLabNoteRelaySentinel(t *testing.T) {
	got := AppendGitLabNoteRelaySentinel("hello")
	if !strings.Contains(got, "hello") || !strings.Contains(got, gitlabNoteRelaySentinel) {
		t.Fatalf("got %q, missing body or sentinel", got)
	}
	idempotent := AppendGitLabNoteRelaySentinel(got)
	if idempotent != got {
		t.Fatalf("appending twice should be idempotent: got %q, want %q", idempotent, got)
	}
}

func TestGitlabProjectCandidateURLs(t *testing.T) {
	p := gitlabWebhookProject{
		PathWithNamespace: "paral/app",
		WebURL:            "https://git.paral.no/paral/app",
		GitHTTPURL:        "https://git.paral.no/paral/app.git",
		GitSSHURL:         "git@git.paral.no:paral/app.git",
	}
	got := gitlabProjectCandidateURLs(p, "https://git.paral.no")
	wantAny := []string{
		"https://git.paral.no/paral/app.git",
		"git@git.paral.no:paral/app.git",
		"https://git.paral.no/paral/app",
	}
	for _, w := range wantAny {
		found := false
		for _, g := range got {
			if g == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("candidate URLs missing %q; got %v", w, got)
		}
	}
}

// ── Integration tests (via the shared VCS webhook + a gitlab vcs_connection) ─

// gitlabIssueHookReq builds a GitLab Issue Hook POST against the connection's
// generic VCS webhook endpoint, authenticated the same way merge-request and
// pipeline events are (X-Gitlab-Token compared verbatim to the connection's
// webhook secret).
func gitlabIssueHookReq(connID, payload string) *http.Request {
	return vcsWebhookReq(connID, map[string]string{
		"X-Gitlab-Token": vcsTestSecret,
		"X-Gitlab-Event": "Issue Hook",
	}, []byte(payload))
}

func TestHandleGitLabIssueSyncWebhook_LabelAdd(t *testing.T) {
	if testHandler == nil {
		t.Skip("no database available")
	}
	box := withVCSBox(t)
	ctx := context.Background()
	wsUUID := parseUUID(testWorkspaceID)
	connID := seedVCSConnection(t, ctx, box, "gitlab", "https://git.example.com")
	t.Cleanup(func() { cleanupVCS(ctx, "") })

	payload := `{
		"object_kind": "issue",
		"object_attributes": {"iid": 10, "title": "Sync me", "description": "desc", "action": "open"},
		"project": {"id": 99, "path_with_namespace": "testorg-issue-add/repo", "namespace": "testorg-issue-add"},
		"labels": [{"title": "agent"}],
		"assignees": []
	}`
	w := httptest.NewRecorder()
	testHandler.HandleVCSWebhook(w, gitlabIssueHookReq(connID, payload))
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	row, err := testHandler.Queries.GetGitLabIssueByProjectAndIID(ctx, db.GetGitLabIssueByProjectAndIIDParams{
		WorkspaceID: wsUUID,
		ProjectPath: "testorg-issue-add/repo",
		GlIssueIid:  10,
	})
	if err != nil {
		t.Fatalf("gitlab_issue not created: %v", err)
	}
	issue, err := testHandler.Queries.GetIssue(ctx, row.IssueID)
	if err != nil {
		t.Fatalf("multica issue not created: %v", err)
	}
	if issue.Title != "Sync me #10" {
		t.Errorf("title: got %q, want %q", issue.Title, "Sync me #10")
	}
	if issue.AssigneeType.Valid {
		t.Errorf("expected unassigned without agent-name label, got type=%q", issue.AssigneeType.String)
	}
}

func TestHandleGitLabIssueSyncWebhook_AssignsAgentByName(t *testing.T) {
	if testHandler == nil {
		t.Skip("no database available")
	}
	box := withVCSBox(t)
	ctx := context.Background()
	wsUUID := parseUUID(testWorkspaceID)
	connID := seedVCSConnection(t, ctx, box, "gitlab", "https://git.example.com")
	t.Cleanup(func() { cleanupVCS(ctx, "") })

	agents, err := testHandler.Queries.ListAgents(ctx, wsUUID)
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	var seeded db.Agent
	for _, a := range agents {
		if a.Name == "Handler Test Agent" && a.Kind == "user" {
			seeded = a
			break
		}
	}
	if !seeded.ID.Valid {
		t.Fatal("setup: fixture agent Handler Test Agent not found")
	}

	payload := `{
		"object_kind": "issue",
		"object_attributes": {"iid": 88, "title": "Assign me", "description": "", "action": "open"},
		"project": {"id": 888, "path_with_namespace": "testorg-agent-name/repo", "namespace": "testorg-agent-name"},
		"labels": [{"title": "agent::Handler Test Agent"}],
		"assignees": []
	}`
	w := httptest.NewRecorder()
	testHandler.HandleVCSWebhook(w, gitlabIssueHookReq(connID, payload))
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}

	row, err := testHandler.Queries.GetGitLabIssueByProjectAndIID(ctx, db.GetGitLabIssueByProjectAndIIDParams{
		WorkspaceID: wsUUID, ProjectPath: "testorg-agent-name/repo", GlIssueIid: 88,
	})
	if err != nil {
		t.Fatalf("gitlab_issue not created: %v", err)
	}
	issue, err := testHandler.Queries.GetIssue(ctx, row.IssueID)
	if err != nil {
		t.Fatalf("multica issue not created: %v", err)
	}
	if !issue.AssigneeType.Valid || issue.AssigneeType.String != "agent" {
		t.Fatalf("assignee_type: got %v, want agent", issue.AssigneeType)
	}
	if !issue.AssigneeID.Valid || uuidToString(issue.AssigneeID) != uuidToString(seeded.ID) {
		t.Fatalf("assignee_id: got %v, want %s", issue.AssigneeID, uuidToString(seeded.ID))
	}
}

func TestHandleGitLabIssueSyncWebhook_CustomSyncLabel(t *testing.T) {
	if testHandler == nil {
		t.Skip("no database available")
	}
	box := withVCSBox(t)
	ctx := context.Background()
	wsUUID := parseUUID(testWorkspaceID)
	connID := seedVCSConnection(t, ctx, box, "gitlab", "https://git.example.com")
	t.Cleanup(func() { cleanupVCS(ctx, "") })

	var previousSettings []byte
	if err := testPool.QueryRow(ctx, `SELECT settings FROM workspace WHERE id = $1`, testWorkspaceID).Scan(&previousSettings); err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if _, err := testPool.Exec(ctx, `UPDATE workspace SET settings = $1::jsonb WHERE id = $2`,
		`{"gitlab_issue_sync_label":"multica"}`, testWorkspaceID); err != nil {
		t.Fatalf("set settings: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `UPDATE workspace SET settings = $1::jsonb WHERE id = $2`, previousSettings, testWorkspaceID)
	})

	agentOnly := `{
		"object_kind": "issue",
		"object_attributes": {"iid": 50, "title": "Wrong label", "description": "", "action": "open"},
		"project": {"id": 500, "path_with_namespace": "testorg-custom-label/repo", "namespace": "testorg-custom-label"},
		"labels": [{"title": "agent"}],
		"assignees": []
	}`
	testHandler.HandleVCSWebhook(httptest.NewRecorder(), gitlabIssueHookReq(connID, agentOnly))
	if _, err := testHandler.Queries.GetGitLabIssueByProjectAndIID(ctx, db.GetGitLabIssueByProjectAndIIDParams{
		WorkspaceID: wsUUID, ProjectPath: "testorg-custom-label/repo", GlIssueIid: 50,
	}); err == nil {
		t.Fatal("expected no gitlab_issue for default agent label when custom label configured")
	}

	customPayload := `{
		"object_kind": "issue",
		"object_attributes": {"iid": 51, "title": "Custom label sync", "description": "hi", "action": "open"},
		"project": {"id": 500, "path_with_namespace": "testorg-custom-label/repo", "namespace": "testorg-custom-label"},
		"labels": [{"title": "multica"}],
		"assignees": []
	}`
	testHandler.HandleVCSWebhook(httptest.NewRecorder(), gitlabIssueHookReq(connID, customPayload))
	row, err := testHandler.Queries.GetGitLabIssueByProjectAndIID(ctx, db.GetGitLabIssueByProjectAndIIDParams{
		WorkspaceID: wsUUID, ProjectPath: "testorg-custom-label/repo", GlIssueIid: 51,
	})
	if err != nil {
		t.Fatalf("gitlab_issue not created for custom label: %v", err)
	}
	issue, err := testHandler.Queries.GetIssue(ctx, row.IssueID)
	if err != nil {
		t.Fatalf("multica issue not created: %v", err)
	}
	if issue.Title != "Custom label sync #51" {
		t.Errorf("title: got %q, want %q", issue.Title, "Custom label sync #51")
	}
}

func TestHandleGitLabIssueSyncWebhook_LabelRemoveCancelsIssue(t *testing.T) {
	if testHandler == nil {
		t.Skip("no database available")
	}
	box := withVCSBox(t)
	ctx := context.Background()
	wsUUID := parseUUID(testWorkspaceID)
	connID := seedVCSConnection(t, ctx, box, "gitlab", "https://git.example.com")
	t.Cleanup(func() { cleanupVCS(ctx, "") })

	addPayload := `{
		"object_kind": "issue",
		"object_attributes": {"iid": 11, "title": "Remove me", "description": "", "action": "open"},
		"project": {"id": 100, "path_with_namespace": "testorg-issue-remove/repo", "namespace": "testorg-issue-remove"},
		"labels": [{"title": "agent"}],
		"assignees": []
	}`
	testHandler.HandleVCSWebhook(httptest.NewRecorder(), gitlabIssueHookReq(connID, addPayload))

	row, err := testHandler.Queries.GetGitLabIssueByProjectAndIID(ctx, db.GetGitLabIssueByProjectAndIIDParams{
		WorkspaceID: wsUUID, ProjectPath: "testorg-issue-remove/repo", GlIssueIid: 11,
	})
	if err != nil {
		t.Fatalf("seed: gitlab_issue not found: %v", err)
	}
	issueID := row.IssueID

	removePayload := `{
		"object_kind": "issue",
		"object_attributes": {"iid": 11, "title": "Remove me", "description": "", "action": "update", "state": "opened"},
		"project": {"id": 100, "path_with_namespace": "testorg-issue-remove/repo", "namespace": "testorg-issue-remove"},
		"labels": [],
		"assignees": [],
		"changes": {"labels": {"previous": [{"title": "agent"}], "current": []}}
	}`
	testHandler.HandleVCSWebhook(httptest.NewRecorder(), gitlabIssueHookReq(connID, removePayload))

	issue, err := testHandler.Queries.GetIssue(ctx, issueID)
	if err != nil {
		t.Fatalf("multica issue should remain after agent label removal: %v", err)
	}
	if issue.Status != "cancelled" {
		t.Errorf("status: got %q, want cancelled", issue.Status)
	}
	if _, err := testHandler.Queries.GetGitLabIssueByProjectAndIID(ctx, db.GetGitLabIssueByProjectAndIIDParams{
		WorkspaceID: wsUUID, ProjectPath: "testorg-issue-remove/repo", GlIssueIid: 11,
	}); err != nil {
		t.Fatalf("gitlab_issue row should remain after agent label removal: %v", err)
	}
}

func TestHandleGitLabIssueSyncWebhook_LabelRestoreUncancelsToTodo(t *testing.T) {
	if testHandler == nil {
		t.Skip("no database available")
	}
	box := withVCSBox(t)
	ctx := context.Background()
	wsUUID := parseUUID(testWorkspaceID)
	connID := seedVCSConnection(t, ctx, box, "gitlab", "https://git.example.com")
	t.Cleanup(func() { cleanupVCS(ctx, "") })

	for _, p := range []string{
		`{"object_kind":"issue","object_attributes":{"iid":61,"title":"Restore me","description":"","action":"open"},"project":{"id":610,"path_with_namespace":"testorg-issue-restore/repo","namespace":"testorg-issue-restore"},"labels":[{"title":"agent"}],"assignees":[]}`,
		`{"object_kind":"issue","object_attributes":{"iid":61,"title":"Restore me","description":"","action":"update","state":"opened"},"project":{"id":610,"path_with_namespace":"testorg-issue-restore/repo","namespace":"testorg-issue-restore"},"labels":[],"assignees":[],"changes":{"labels":{"previous":[{"title":"agent"}],"current":[]}}}`,
	} {
		testHandler.HandleVCSWebhook(httptest.NewRecorder(), gitlabIssueHookReq(connID, p))
	}

	row, err := testHandler.Queries.GetGitLabIssueByProjectAndIID(ctx, db.GetGitLabIssueByProjectAndIIDParams{
		WorkspaceID: wsUUID, ProjectPath: "testorg-issue-restore/repo", GlIssueIid: 61,
	})
	if err != nil {
		t.Fatalf("gitlab_issue not found: %v", err)
	}
	before, err := testHandler.Queries.GetIssue(ctx, row.IssueID)
	if err != nil {
		t.Fatalf("get issue: %v", err)
	}
	if before.Status != "cancelled" {
		t.Fatalf("precondition: status = %q, want cancelled", before.Status)
	}

	restorePayload := `{
		"object_kind": "issue",
		"object_attributes": {"iid": 61, "title": "Restore me", "description": "", "action": "update"},
		"project": {"id": 610, "path_with_namespace": "testorg-issue-restore/repo", "namespace": "testorg-issue-restore"},
		"labels": [{"title": "agent::Implementer"}],
		"assignees": []
	}`
	testHandler.HandleVCSWebhook(httptest.NewRecorder(), gitlabIssueHookReq(connID, restorePayload))

	issue, err := testHandler.Queries.GetIssue(ctx, row.IssueID)
	if err != nil {
		t.Fatalf("get issue: %v", err)
	}
	if issue.Status != "todo" {
		t.Errorf("status: got %q, want todo", issue.Status)
	}
}

func TestHandleGitLabIssueSyncWebhook_Close(t *testing.T) {
	if testHandler == nil {
		t.Skip("no database available")
	}
	box := withVCSBox(t)
	ctx := context.Background()
	wsUUID := parseUUID(testWorkspaceID)
	connID := seedVCSConnection(t, ctx, box, "gitlab", "https://git.example.com")
	t.Cleanup(func() { cleanupVCS(ctx, "") })

	addPayload := `{
		"object_kind": "issue",
		"object_attributes": {"iid": 12, "title": "Close me", "description": "", "action": "open"},
		"project": {"id": 101, "path_with_namespace": "testorg-issue-close/repo", "namespace": "testorg-issue-close"},
		"labels": [{"title": "agent"}],
		"assignees": []
	}`
	testHandler.HandleVCSWebhook(httptest.NewRecorder(), gitlabIssueHookReq(connID, addPayload))

	row, _ := testHandler.Queries.GetGitLabIssueByProjectAndIID(ctx, db.GetGitLabIssueByProjectAndIIDParams{
		WorkspaceID: wsUUID, ProjectPath: "testorg-issue-close/repo", GlIssueIid: 12,
	})

	closePayload := `{
		"object_kind": "issue",
		"object_attributes": {"iid": 12, "title": "Close me", "description": "", "action": "close"},
		"project": {"id": 101, "path_with_namespace": "testorg-issue-close/repo", "namespace": "testorg-issue-close"},
		"labels": [{"title": "agent"}],
		"assignees": []
	}`
	testHandler.HandleVCSWebhook(httptest.NewRecorder(), gitlabIssueHookReq(connID, closePayload))

	issue, err := testHandler.Queries.GetIssue(ctx, row.IssueID)
	if err != nil {
		t.Fatalf("get issue: %v", err)
	}
	if issue.Status != "done" {
		t.Errorf("status: got %q, want %q", issue.Status, "done")
	}
}

func TestHandleGitLabIssueSyncWebhook_Reopen(t *testing.T) {
	if testHandler == nil {
		t.Skip("no database available")
	}
	box := withVCSBox(t)
	ctx := context.Background()
	wsUUID := parseUUID(testWorkspaceID)
	connID := seedVCSConnection(t, ctx, box, "gitlab", "https://git.example.com")
	t.Cleanup(func() { cleanupVCS(ctx, "") })

	for _, p := range []string{
		`{"object_kind":"issue","object_attributes":{"iid":13,"title":"Reopen me","description":"","action":"open","state":"opened"},"project":{"id":102,"path_with_namespace":"testorg-issue-reopen/repo","namespace":"testorg-issue-reopen"},"labels":[{"title":"agent"}],"assignees":[]}`,
		`{"object_kind":"issue","object_attributes":{"iid":13,"title":"Reopen me","description":"","action":"close","state":"closed"},"project":{"id":102,"path_with_namespace":"testorg-issue-reopen/repo","namespace":"testorg-issue-reopen"},"labels":[{"title":"agent"}],"assignees":[]}`,
	} {
		testHandler.HandleVCSWebhook(httptest.NewRecorder(), gitlabIssueHookReq(connID, p))
	}

	row, _ := testHandler.Queries.GetGitLabIssueByProjectAndIID(ctx, db.GetGitLabIssueByProjectAndIIDParams{
		WorkspaceID: wsUUID, ProjectPath: "testorg-issue-reopen/repo", GlIssueIid: 13,
	})

	reopenPayload := `{"object_kind":"issue","object_attributes":{"iid":13,"title":"Reopen me","description":"","action":"reopen","state":"opened"},"project":{"id":102,"path_with_namespace":"testorg-issue-reopen/repo","namespace":"testorg-issue-reopen"},"labels":[{"title":"agent"}],"assignees":[]}`
	testHandler.HandleVCSWebhook(httptest.NewRecorder(), gitlabIssueHookReq(connID, reopenPayload))

	issue, _ := testHandler.Queries.GetIssue(ctx, row.IssueID)
	if issue.Status != "in_progress" {
		t.Errorf("status: got %q, want %q", issue.Status, "in_progress")
	}
}

func TestGetGitLabIssueForIssue(t *testing.T) {
	if testHandler == nil {
		t.Skip("no database available")
	}
	box := withVCSBox(t)
	ctx := context.Background()
	connID := seedVCSConnection(t, ctx, box, "gitlab", "https://git.example.com")
	t.Cleanup(func() { cleanupVCS(ctx, "") })

	issuePayload := `{"object_kind":"issue","object_attributes":{"iid":40,"title":"Get me","description":"","action":"open"},"project":{"id":400,"path_with_namespace":"testorg-get-issue/repo","namespace":"testorg-get-issue"},"labels":[{"title":"agent"}],"assignees":[{"username":"getuser"}]}`
	testHandler.HandleVCSWebhook(httptest.NewRecorder(), gitlabIssueHookReq(connID, issuePayload))

	row, err := testHandler.Queries.GetGitLabIssueByProjectAndIID(ctx, db.GetGitLabIssueByProjectAndIIDParams{
		WorkspaceID: parseUUID(testWorkspaceID), ProjectPath: "testorg-get-issue/repo", GlIssueIid: 40,
	})
	if err != nil {
		t.Fatalf("seed issue not found: %v", err)
	}
	issueIDStr := uuidToString(row.IssueID)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", issueIDStr)
	req := httptest.NewRequest(http.MethodGet, "/api/issues/"+issueIDStr+"/gitlab-issue", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req.Header.Set("X-User-ID", testUserID)
	req.Header.Set("X-Workspace-ID", testWorkspaceID)
	w := httptest.NewRecorder()
	testHandler.GetGitLabIssueForIssue(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		GlIssueIID         int    `json:"gl_issue_iid"`
		ProjectPath        string `json:"project_path"`
		URL                string `json:"url"`
		GlAssigneeUsername string `json:"gl_assignee_username"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.GlIssueIID != 40 {
		t.Errorf("gl_issue_iid: got %d, want 40", resp.GlIssueIID)
	}
	if resp.ProjectPath != "testorg-get-issue/repo" {
		t.Errorf("project_path: got %q", resp.ProjectPath)
	}
	if resp.GlAssigneeUsername != "getuser" {
		t.Errorf("gl_assignee_username: got %q, want %q", resp.GlAssigneeUsername, "getuser")
	}
	if !strings.Contains(resp.URL, "testorg-get-issue/repo") {
		t.Errorf("url missing project path: %q", resp.URL)
	}
}

func TestGetGitLabIssueForIssue_NotFound(t *testing.T) {
	if testHandler == nil {
		t.Skip("no database available")
	}
	randomUUID := pgtype.UUID{}
	randomUUID.Bytes = [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	randomUUID.Valid = true

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuidToString(randomUUID))
	req := httptest.NewRequest(http.MethodGet, "/api/issues/"+uuidToString(randomUUID)+"/gitlab-issue", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req.Header.Set("X-User-ID", testUserID)
	req.Header.Set("X-Workspace-ID", testWorkspaceID)
	w := httptest.NewRecorder()
	testHandler.GetGitLabIssueForIssue(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
