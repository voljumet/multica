package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type gitLabUserLinkResponse struct {
	GitlabUsername string `json:"gitlab_username"`
}

// GetGitLabUserLink returns the current member's linked GitLab username, or 404.
func (h *Handler) GetGitLabUserLink(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	member, ok := ctxMember(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, ctxWorkspaceID(ctx), "workspace id")
	if !ok {
		return
	}
	link, err := h.Queries.GetGitLabUserLinkByMember(ctx, db.GetGitLabUserLinkByMemberParams{
		WorkspaceID: wsUUID,
		MemberID:    member.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusOK, map[string]any{"gitlab_username": nil})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get link")
		return
	}
	writeJSON(w, http.StatusOK, gitLabUserLinkResponse{GitlabUsername: link.GitlabUsername})
}

// LinkGitLabUser stores or replaces the current member's GitLab username.
func (h *Handler) LinkGitLabUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	member, ok := ctxMember(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, ctxWorkspaceID(ctx), "workspace id")
	if !ok {
		return
	}

	var req struct {
		GitlabUsername string `json:"gitlab_username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	username := strings.TrimSpace(req.GitlabUsername)
	if username == "" {
		writeError(w, http.StatusBadRequest, "gitlab_username is required")
		return
	}

	link, err := h.Queries.UpsertGitLabUserLink(ctx, db.UpsertGitLabUserLinkParams{
		WorkspaceID:    wsUUID,
		MemberID:       member.ID,
		GitlabUsername: username,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save link")
		return
	}
	writeJSON(w, http.StatusOK, gitLabUserLinkResponse{GitlabUsername: link.GitlabUsername})
}

// UnlinkGitLabUser removes the current member's GitLab username link.
func (h *Handler) UnlinkGitLabUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	member, ok := ctxMember(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, ctxWorkspaceID(ctx), "workspace id")
	if !ok {
		return
	}
	if err := h.Queries.DeleteGitLabUserLink(ctx, db.DeleteGitLabUserLinkParams{
		WorkspaceID: wsUUID,
		MemberID:    member.ID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to unlink")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
