package openai_api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestNormalizePathPrefix(t *testing.T) {
	if got := normalizePathPrefix(""); got != "/openai" {
		t.Fatalf("empty: got %q", got)
	}
	if got := normalizePathPrefix("custom"); got != "/custom" {
		t.Fatalf("custom: got %q", got)
	}
	if got := normalizePathPrefix("/v1/"); got != "/v1" {
		t.Fatalf("trim: got %q", got)
	}
}

func TestFlattenChatMessages(t *testing.T) {
	msgs := []map[string]any{
		{"role": "system", "content": "You are helpful."},
		{"role": "user", "content": "Hi"},
	}
	s, err := flattenChatMessages(msgs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "SYSTEM:") || !strings.Contains(s, "USER: Hi") {
		t.Fatalf("unexpected flatten: %q", s)
	}
}

func TestContentToString_Multimodal(t *testing.T) {
	raw := []any{
		map[string]any{"type": "text", "text": "hello"},
		map[string]any{"type": "text", "text": "world"},
	}
	s, err := contentToString(raw)
	if err != nil {
		t.Fatal(err)
	}
	if s != "hello\nworld" {
		t.Fatalf("got %q", s)
	}
}

func TestOpenAIAPIChannel_Auth(t *testing.T) {
	ch := &OpenAIAPIChannel{
		cfg: config.OpenAIAPIConfig{
			APIKey: "secret",
		},
		pathPrefix: "/openai",
	}

	req := httptest.NewRequest(http.MethodGet, "/openai/v1/models", nil)
	rec := httptest.NewRecorder()
	ch.authenticate(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without bearer, got %d", rec.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/openai/v1/models", nil)
	req2.Header.Set("Authorization", "Bearer secret")
	rec2 := httptest.NewRecorder()
	if !ch.authenticate(rec2, req2) {
		t.Fatal("expected auth success")
	}
}
