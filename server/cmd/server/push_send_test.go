package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestSendPushDirect_SkipsOnNoTokens(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	origURL := expoPushURL
	expoPushURL = srv.URL
	defer func() { expoPushURL = origURL }()

	sendPushDirect(nil, "Test title", "my-workspace", "issue-id-123", "My Workspace")
	time.Sleep(100 * time.Millisecond)

	if called {
		t.Fatal("expected no HTTP call with empty token list")
	}
}

func TestSendPushDirect_FiresForExpoToken(t *testing.T) {
	var mu sync.Mutex
	var received []byte
	done := make(chan struct{})
	var once sync.Once

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received = make([]byte, r.ContentLength)
		r.Body.Read(received) //nolint:errcheck
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[]}`)) //nolint:errcheck

		once.Do(func() { close(done) })
	}))
	defer srv.Close()

	origURL := expoPushURL
	expoPushURL = srv.URL
	defer func() { expoPushURL = origURL }()

	sendPushDirect([]string{"ExponentPushToken[abc123]"}, "New comment", "my-workspace", "issue-id-456", "My Workspace")

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for push HTTP request")
	}

	mu.Lock()
	body := string(received)
	mu.Unlock()

	if body == "" {
		t.Fatal("expected non-empty request body")
	}

	var msgs []struct {
		Title string         `json:"title"`
		Body  string         `json:"body"`
		Data  map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &msgs); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("expected at least one push message")
	}
	msg := msgs[0]
	if msg.Title != "New comment" {
		t.Errorf("expected title %q, got %q", "New comment", msg.Title)
	}
	if msg.Body != "My Workspace" {
		t.Errorf("expected body %q, got %q", "My Workspace", msg.Body)
	}
	cat, _ := msg.Data["category"].(string)
	if cat != "issue_notification" {
		t.Errorf("expected data.category %q, got %q", "issue_notification", cat)
	}
	wsSlug, _ := msg.Data["workspace_slug"].(string)
	if wsSlug != "my-workspace" {
		t.Errorf("expected data.workspace_slug %q, got %q", "my-workspace", wsSlug)
	}
}

// The default Expo push endpoint must include the /--/ path prefix —
// https://exp.host/api/v2/push/send (without it) returns 404 and every
// push silently fails. Regression guard since other tests stub the URL.
func TestExpoPushURLDefault(t *testing.T) {
	if expoPushURL != "https://exp.host/--/api/v2/push/send" {
		t.Fatalf("unexpected default expoPushURL: %s", expoPushURL)
	}
}
