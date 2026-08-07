package main

import (
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

	sendPushDirect(nil, "Test title", "my-workspace", "issue-id-123")
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

	sendPushDirect([]string{"ExponentPushToken[abc123]"}, "New comment", "my-workspace", "issue-id-456")

	select {
	case <-done:
		// request was received — success
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for push HTTP request")
	}

	mu.Lock()
	body := string(received)
	mu.Unlock()

	if body == "" {
		t.Fatal("expected non-empty request body")
	}
	if len(body) == 0 {
		t.Fatal("expected request body to contain push message")
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
