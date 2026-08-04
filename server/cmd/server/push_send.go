package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// expoPushURL is a var (not const) so tests can override it with a local server.
var expoPushURL = "https://exp.host/api/v2/push/send"

type expoPushMessage struct {
	To    string         `json:"to"`
	Title string         `json:"title"`
	Sound string         `json:"sound"`
	Data  map[string]any `json:"data,omitempty"`
}

// sendPushDirect posts messages to the Expo Push API in a goroutine.
// Errors are logged and never returned. Separated from sendPushNotifications
// so tests can stub the HTTP layer.
func sendPushDirect(tokens []string, title, workspaceSlug, issueID string) {
	if len(tokens) == 0 {
		return
	}

	messages := make([]expoPushMessage, 0, len(tokens))
	for _, tok := range tokens {
		messages = append(messages, expoPushMessage{
			To:    tok,
			Title: title,
			Sound: "default",
			Data: map[string]any{
				"workspace_slug": workspaceSlug,
				"issue_id":       issueID,
			},
		})
	}

	go func() {
		body, err := json.Marshal(messages)
		if err != nil {
			slog.Error("push: marshal error", "error", err)
			return
		}
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Post(expoPushURL, "application/json", bytes.NewReader(body))
		if err != nil {
			slog.Error("push: expo request failed", "error", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			slog.Error("push: expo returned non-2xx", "status", resp.StatusCode)
			return
		}
		// Expo returns 200 with per-ticket errors (e.g. DeviceNotRegistered)
		// in the body; surface them so delivery failures aren't silent.
		var result struct {
			Data []struct {
				Status  string `json:"status"`
				Message string `json:"message"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return
		}
		for _, ticket := range result.Data {
			if ticket.Status != "ok" {
				slog.Error("push: expo ticket error", "status", ticket.Status, "message", ticket.Message)
			}
		}
	}()
}

// sendPushNotifications dispatches an Expo push to every registered device
// for userID. Fire-and-forget: errors are logged, never returned.
// workspaceSlug and issueID are included in the notification data for
// deep-linking on tap.
func sendPushNotifications(
	ctx context.Context,
	queries *db.Queries,
	userID string,
	title string,
	workspaceSlug string,
	issueID string,
) {
	tokens, err := queries.ListPushTokensByUser(ctx, parseUUID(userID))
	if err != nil || len(tokens) == 0 {
		return
	}

	var tokenStrings []string
	for _, t := range tokens {
		if t.Platform != "expo" {
			continue
		}
		tokenStrings = append(tokenStrings, t.Token)
	}

	sendPushDirect(tokenStrings, title, workspaceSlug, issueID)
}

// workspaceSlugByID returns the slug for a workspace ID, or "" on error.
func workspaceSlugByID(ctx context.Context, queries *db.Queries, workspaceID string) string {
	ws, err := queries.GetWorkspace(ctx, parseUUID(workspaceID))
	if err != nil {
		return ""
	}
	return ws.Slug
}
