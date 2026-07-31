package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterPushToken_RejectsInvalidFormat(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"token": "not-a-valid-token", "platform": "expo"})
	req := httptest.NewRequest(http.MethodPost, "/api/push-tokens", bytes.NewReader(body))
	req.Header.Set("X-User-ID", testUserID)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testHandler.RegisterPushToken(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRegisterPushToken_AcceptsValidToken(t *testing.T) {
	body, _ := json.Marshal(map[string]string{
		"token":    "ExponentPushToken[xxxxxxxxxxxxxxxxxxxxxx]",
		"platform": "expo",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/push-tokens", bytes.NewReader(body))
	req.Header.Set("X-User-ID", testUserID)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testHandler.RegisterPushToken(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}
