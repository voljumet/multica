package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// RefreshSkillFromURL re-fetches a skill from the source URL stored in
// skill.config.origin.source_url (set on every hosted URL import) and
// overwrites description, content, supporting files, and origin provenance.
// Skill identity (id, name, created_by, created_at) and agent bindings are
// preserved. updated_at is bumped by UpdateSkill.
//
// Permission matches canManageSkill: skill creator or workspace owner/admin.
// Manual / runtime-local skills without a source_url cannot be refreshed.
func (h *Handler) RefreshSkillFromURL(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	skill, ok := h.loadSkillForUser(w, r, id)
	if !ok {
		return
	}
	if !h.canManageSkill(w, r, skill) {
		return
	}

	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	sourceURL, ok := skillConfigSourceURL(skill.Config)
	if !ok {
		writeError(w, http.StatusBadRequest, "this skill has no source URL to refresh from; only skills imported from GitHub, ClawHub, or Skills.sh can be updated from URL")
		return
	}

	source, normalized, err := detectImportSource(sourceURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, "saved source URL is not a supported import source: "+err.Error())
		return
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	var imported *importedSkill
	switch source {
	case sourceClawHub:
		imported, err = fetchFromClawHub(httpClient, normalized)
	case sourceSkillsSh:
		imported, err = fetchFromSkillsSh(httpClient, normalized)
	case sourceGitHub:
		imported, err = fetchFromGitHub(httpClient, normalized)
	default:
		writeError(w, http.StatusBadRequest, "saved source URL is not a supported import source")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	files := make([]CreateSkillFileRequest, 0, len(imported.files))
	for _, f := range imported.files {
		if !validateFilePath(f.path) {
			continue
		}
		files = append(files, CreateSkillFileRequest{
			Path:    f.path,
			Content: f.content,
		})
	}

	config := map[string]any{}
	if imported.origin != nil {
		config["origin"] = imported.origin
	}

	workspaceID := uuidToString(skill.WorkspaceID)
	resp, err := h.overwriteSkillWithFiles(r.Context(), skillOverwriteInput{
		WorkspaceID:   skill.WorkspaceID,
		TargetSkillID: skill.ID,
		UserID:        userID,
		Description:   imported.description,
		Content:       imported.content,
		Config:        config,
		Files:         files,
		AllowManager:  true,
		// Do not enforce ExpectedName: the workspace may keep a local name while
		// the upstream frontmatter name drifts.
	})
	if err != nil {
		status, reason := skillImportOverwriteFailure(err)
		writeError(w, status, reason)
		return
	}

	actorType, actorID := h.resolveActor(r, userID, workspaceID)
	h.publish(protocol.EventSkillUpdated, workspaceID, actorType, actorID, map[string]any{"skill": resp})
	writeJSON(w, http.StatusOK, resp)
}

// skillConfigSourceURL reads config.origin.source_url from a skill's JSONB config.
func skillConfigSourceURL(raw []byte) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		return "", false
	}
	origin, _ := config["origin"].(map[string]any)
	if origin == nil {
		return "", false
	}
	url, _ := origin["source_url"].(string)
	url = strings.TrimSpace(url)
	if url == "" {
		return "", false
	}
	return url, true
}
