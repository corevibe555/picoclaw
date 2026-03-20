package websocket_audio

import "testing"

func TestNormalizePathPrefix(t *testing.T) {
	if got := normalizePathPrefix(""); got != "/ws/audio" {
		t.Fatalf("got %q", got)
	}
	if got := normalizePathPrefix("/custom/"); got != "/custom" {
		t.Fatalf("got %q", got)
	}
}
