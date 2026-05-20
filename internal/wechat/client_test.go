package wechat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	c := NewClient(DefaultBaseURL, "test-token", time.Now(), "")
	if c.Status() != StatusDisconnected {
		t.Error("expected disconnected status")
	}
}

func TestParseMessage(t *testing.T) {
	raw := map[string]interface{}{
		"from_user_id":  "user123@im.wechat",
		"to_user_id":    "bot456@im.bot",
		"message_type":  float64(1),
		"message_state": float64(2),
		"context_token": "tokentest123",
		"item_list": []interface{}{
			map[string]interface{}{
				"type": float64(1),
				"text_item": map[string]interface{}{
					"text": "Hello bot",
				},
			},
		},
	}
	msg := parseMessage(raw)
	if msg.FromUserID != "user123@im.wechat" {
		t.Errorf("unexpected from: %s", msg.FromUserID)
	}
	if msg.Text != "Hello bot" {
		t.Errorf("unexpected text: %s", msg.Text)
	}
	if msg.MessageType != 1 {
		t.Errorf("unexpected message type: %d", msg.MessageType)
	}
	if msg.ContextToken != "tokentest123" {
		t.Errorf("unexpected token: %s", msg.ContextToken)
	}
}

func TestClientHeaders(t *testing.T) {
	c := NewClient(DefaultBaseURL, "test-token", time.Now(), "")
	headers := c.makeHeaders()
	if headers["AuthorizationType"] != "ilink_bot_token" {
		t.Error("missing AuthorizationType header")
	}
	if headers["Content-Type"] != "application/json" {
		t.Error("missing Content-Type header")
	}
}

func TestSetToken(t *testing.T) {
	c := NewClient(DefaultBaseURL, "", time.Now(), "")
	c.SetToken("new-token", "https://custom.url")
	if c.Token() != "new-token" {
		t.Errorf("expected new-token, got %s", c.Token())
	}
}

func TestStatusTransitions(t *testing.T) {
	c := NewClient(DefaultBaseURL, "", time.Now(), "")
	if c.Status() != StatusDisconnected {
		t.Error("expected disconnected initially")
	}
	c.SetStatus(StatusConnected)
	if c.Status() != StatusConnected {
		t.Error("expected connected after SetStatus")
	}
}

func TestParseLoginTime(t *testing.T) {
	lt := ParseLoginTime("2026-05-10T12:00:00Z")
	if lt.Year() != 2026 {
		t.Errorf("expected 2026, got %d", lt.Year())
	}

	lt2 := ParseLoginTime("")
	if !lt2.IsZero() {
		t.Error("expected zero time for empty string")
	}
}

func TestPollLoopKeepsConnectedAfterTransientPollError(t *testing.T) {
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ilink/bot/getupdates" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`not-json`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"get_updates_buf": "",
			"msgs":            []interface{}{},
		})
	}))
	defer api.Close()

	c := NewClient(api.URL, "test-token", time.Now(), "")
	c.Start()
	defer c.Stop()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if calls.Load() >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if calls.Load() < 2 {
		t.Fatalf("expected poll loop to retry after a transient error, calls=%d status=%s", calls.Load(), c.Status())
	}
	if c.Status() != StatusConnected {
		t.Fatalf("expected client to remain connected after transient poll error, got %s", c.Status())
	}
}
