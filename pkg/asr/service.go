package asr

import (
	"context"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/asr/providers/groq"
	"github.com/sipeed/picoclaw/pkg/asr/providers/openai"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/voice"
)

// Service routes ASR to the configured primary provider.
type Service struct {
	provider Provider
}

// NewService returns an ASR service when audio is enabled and a provider is configured.
// Returns (nil, nil) if ASR is not configured (not an error).
func NewService(cfg *config.Config) (*Service, error) {
	if cfg == nil || !cfg.Audio.Enabled {
		return nil, nil
	}
	p := strings.ToLower(strings.TrimSpace(cfg.Audio.ASR.Provider))
	switch p {
	case "openai":
		if strings.TrimSpace(cfg.Audio.ASR.OpenAI.APIKey) == "" {
			return nil, fmt.Errorf("audio.asr.provider is openai but api_key is empty")
		}
		return &Service{provider: openai.New(cfg.Audio.ASR.OpenAI)}, nil
	case "groq":
		if g := groq.NewFromConfig(cfg); g != nil {
			return &Service{provider: g}, nil
		}
		return nil, fmt.Errorf("audio.asr.provider is groq but no Groq API key is configured")
	default:
		return nil, nil
	}
}

// Transcriber adapts the ASR service to voice.Transcriber for the agent loop.
func (s *Service) Transcriber() voice.Transcriber {
	if s == nil || s.provider == nil {
		return nil
	}
	return &transcriberAdapter{p: s.provider}
}

type transcriberAdapter struct {
	p Provider
}

func (t *transcriberAdapter) Name() string { return t.p.Name() }

func (t *transcriberAdapter) Transcribe(ctx context.Context, audioFilePath string) (*voice.TranscriptionResponse, error) {
	text, err := t.p.Transcribe(ctx, audioFilePath)
	if err != nil {
		return nil, err
	}
	return &voice.TranscriptionResponse{Text: text}, nil
}
