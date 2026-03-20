package asr

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestNewService_Disabled(t *testing.T) {
	cfg := config.DefaultConfig()
	s, err := NewService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if s != nil {
		t.Fatal("expected nil service when audio disabled")
	}
}

func TestNewService_OpenAIRequiresKey(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Audio.Enabled = true
	cfg.Audio.ASR.Provider = "openai"
	cfg.Audio.ASR.OpenAI.APIKey = ""
	_, err := NewService(cfg)
	if err == nil {
		t.Fatal("expected error without API key")
	}
}
