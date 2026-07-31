package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// RegisterPushToken upserts a device push token for the authenticated user.
// POST /api/push-tokens
// Body: { "token": "ExponentPushToken[...]", "platform": "expo" }
func (h *Handler) RegisterPushToken(w http.ResponseWriter, r *http.Request) {
	userID := requestUserID(r)

	var req struct {
		Token    string `json:"token"`
		Platform string `json:"platform"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}
	// Expo push tokens always start with "ExponentPushToken[".
	if !strings.HasPrefix(req.Token, "ExponentPushToken[") {
		writeError(w, http.StatusBadRequest, "invalid expo push token format")
		return
	}
	platform := req.Platform
	if platform == "" {
		platform = "expo"
	}

	err := h.Queries.UpsertPushToken(r.Context(), db.UpsertPushTokenParams{
		UserID:   parseUUID(userID),
		Token:    req.Token,
		Platform: platform,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to register push token")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
