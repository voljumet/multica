package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSkillConfigSourceURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "empty", raw: "", want: "", ok: false},
		{name: "no origin", raw: `{}`, want: "", ok: false},
		{name: "manual origin", raw: `{"origin":{"type":"manual"}}`, want: "", ok: false},
		{name: "blank url", raw: `{"origin":{"type":"github","source_url":"  "}}`, want: "", ok: false},
		{
			name: "github",
			raw:  `{"origin":{"type":"github","source_url":"https://github.com/acme/skills/tree/main/foo"}}`,
			want: "https://github.com/acme/skills/tree/main/foo",
			ok:   true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := skillConfigSourceURL([]byte(tc.raw))
			if ok != tc.ok || got != tc.want {
				t.Fatalf("skillConfigSourceURL(%q) = (%q, %v), want (%q, %v)", tc.raw, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestRefreshSkillFromURL_NoSourceURL(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("handler test DB not configured")
	}
	skillID := insertHandlerTestSkill(t, "refresh-no-url", "# Manual skill")

	w := httptest.NewRecorder()
	req := withURLParam(
		newRequestAsUser(testUserID, http.MethodPost, "/api/skills/"+skillID+"/refresh", nil),
		"id", skillID,
	)
	testHandler.RefreshSkillFromURL(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
}

func TestRefreshSkillFromURL_UpdatesContentAndOrigin(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("handler test DB not configured")
	}

	namePrefix := "refresh-from-url"
	skillName := namePrefix + "-" + t.Name()
	importURL := withMockClawHubImport(t, skillName)

	// Seed via import so config.origin.source_url is populated the same way
	// production writes it.
	wCreate := httptest.NewRecorder()
	reqCreate := newRequestAsUser(testUserID, http.MethodPost, "/api/skills/import", map[string]any{
		"url":         importURL,
		"on_conflict": "fail",
	})
	testHandler.ImportSkill(wCreate, reqCreate)
	if wCreate.Code != http.StatusCreated {
		t.Fatalf("import status = %d, want 201: %s", wCreate.Code, wCreate.Body.String())
	}
	var created SkillImportResult
	if err := json.Unmarshal(wCreate.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Skill == nil {
		t.Fatalf("create body missing skill: %#v", created)
	}
	skillID := created.Skill.ID
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM skill WHERE id = $1`, skillID)
	})

	beforeUpdatedAt := created.Skill.UpdatedAt
	// Ensure updated_at can move forward even on fast clocks.
	time.Sleep(5 * time.Millisecond)

	// Point the mock at a new body for the refresh fetch.
	// withMockClawHubImport already set clawHubAPIBase; re-mock with updated content.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/skills/review-helper":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"skill": map[string]any{
					"slug":        "review-helper",
					"displayName": skillName,
					"summary":     "Refreshed description",
					"tags":        map[string]string{"latest": "1.0.0"},
				},
			})
		case "/api/v1/skills/review-helper/versions/1.0.0":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"version": map[string]any{
					"version": "1.0.0",
					"files": []map[string]any{
						{"path": "SKILL.md", "size": 20},
						{"path": "notes.md", "size": 5},
					},
				},
			})
		case "/api/v1/skills/review-helper/file":
			path := r.URL.Query().Get("path")
			if path == "notes.md" {
				_, _ = w.Write([]byte("notes"))
				return
			}
			_, _ = w.Write([]byte("# Refreshed body\n"))
		default:
			t.Fatalf("unexpected ClawHub path: %s", r.URL.String())
		}
	}))
	prev := clawHubAPIBase
	clawHubAPIBase = srv.URL + "/api/v1"
	t.Cleanup(func() {
		clawHubAPIBase = prev
		srv.Close()
	})

	w := httptest.NewRecorder()
	req := withURLParam(
		newRequestAsUser(testUserID, http.MethodPost, "/api/skills/"+skillID+"/refresh", nil),
		"id", skillID,
	)
	testHandler.RefreshSkillFromURL(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var body SkillWithFilesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ID != skillID {
		t.Fatalf("id = %q, want %q (identity must be preserved)", body.ID, skillID)
	}
	if body.Name != skillName {
		t.Fatalf("name = %q, want %q (local name preserved)", body.Name, skillName)
	}
	if body.Content != "# Refreshed body\n" {
		t.Fatalf("content = %q, want refreshed body", body.Content)
	}
	if body.Description != "Refreshed description" {
		t.Fatalf("description = %q", body.Description)
	}
	if body.UpdatedAt == "" {
		t.Fatal("updated_at empty after refresh")
	}
	_ = beforeUpdatedAt
	foundNotes := false
	for _, f := range body.Files {
		if f.Path == "notes.md" && f.Content == "notes" {
			foundNotes = true
		}
	}
	if !foundNotes {
		t.Fatalf("expected notes.md supporting file, got %#v", body.Files)
	}
	rawCfg, _ := json.Marshal(body.Config)
	var cfg map[string]any
	if err := json.Unmarshal(rawCfg, &cfg); err != nil {
		t.Fatalf("config: %v", err)
	}
	origin, _ := cfg["origin"].(map[string]any)
	if origin == nil || origin["source_url"] == "" {
		t.Fatalf("expected origin.source_url after refresh, config=%#v", body.Config)
	}
}

func TestRefreshSkillFromURL_ForbiddenForNonCreatorMember(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("handler test DB not configured")
	}
	namePrefix := "refresh-forbidden"
	skillName := namePrefix + "-" + t.Name()
	importURL := withMockClawHubImport(t, skillName)

	wCreate := httptest.NewRecorder()
	reqCreate := newRequestAsUser(testUserID, http.MethodPost, "/api/skills/import", map[string]any{
		"url":         importURL,
		"on_conflict": "fail",
	})
	testHandler.ImportSkill(wCreate, reqCreate)
	if wCreate.Code != http.StatusCreated {
		t.Fatalf("import status = %d: %s", wCreate.Code, wCreate.Body.String())
	}
	var created SkillImportResult
	if err := json.Unmarshal(wCreate.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	skillID := created.Skill.ID
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM skill WHERE id = $1`, skillID)
	})

	otherUser := createRuntimeLocalSkillTestMember(t, "member")

	w := httptest.NewRecorder()
	req := withURLParam(
		newRequestAsUser(otherUser, http.MethodPost, "/api/skills/"+skillID+"/refresh", nil),
		"id", skillID,
	)
	testHandler.RefreshSkillFromURL(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", w.Code, w.Body.String())
	}
}
