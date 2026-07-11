package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSendMessageWithMarkupIncludesCompleteButton(t *testing.T) {
	var gotBody SendMessageRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer srv.Close()

	c := New("tok").WithAPIBase(srv.URL)
	if err := c.SendMessageWithMarkup(context.Background(), "123", "hello", completeKeyboard("task-9", "Complete")); err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotBody.Text != "hello" || gotBody.ChatID != "123" {
		t.Fatalf("unexpected body: %+v", gotBody)
	}
	var markup InlineKeyboardMarkup
	if err := json.Unmarshal(gotBody.ReplyMarkup, &markup); err != nil {
		t.Fatalf("markup decode: %v", err)
	}
	if len(markup.InlineKeyboard) != 1 || len(markup.InlineKeyboard[0]) != 1 {
		t.Fatalf("expected single button, got %+v", markup.InlineKeyboard)
	}
	btn := markup.InlineKeyboard[0][0]
	if btn.Text != "Complete" || btn.CallbackData != "complete:task-9" {
		t.Fatalf("unexpected button: %+v", btn)
	}
}

func TestCallRetriesOn429(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":0}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer srv.Close()

	c := New("tok").WithAPIBase(srv.URL)
	if err := c.SendMessage(context.Background(), "1", "hi"); err != nil {
		t.Fatalf("expected success after retry, got %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 calls (1 rate-limited + 1 retry), got %d", got)
	}
}

func TestCallReturnsAPIErrorAfterMaxRetries(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":0}}`))
	}))
	defer srv.Close()

	c := New("tok").WithAPIBase(srv.URL)
	err := c.SendMessage(context.Background(), "1", "hi")
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 APIError, got %v", err)
	}
	if got := calls.Load(); got != maxRetries+1 {
		t.Fatalf("expected %d attempts, got %d", maxRetries+1, got)
	}
}
