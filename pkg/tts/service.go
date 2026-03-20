package tts

import (
	"context"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
)

// Service routes TTS to the configured provider.
type Service struct {
	provider Provider
}

// NewService returns a TTS service when audio is enabled and a provider is configured.
func NewService(cfg *config.Config) (*Service, error) {
	if cfg == nil || !cfg.Audio.Enabled {
		return nil, nil
	}
	p := strings.ToLower(strings.TrimSpace(cfg.Audio.TTS.Provider))
	switch p {
	case "openai":
		if strings.TrimSpace(cfg.Audio.TTS.OpenAI.APIKey) == "" {
			return nil, fmt.Errorf("audio.tts.provider is openai but api_key is empty")
		}
		return &Service{provider: newOpenAIProvider(cfg.Audio.TTS.OpenAI)}, nil
	default:
		return nil, nil
	}
}

// Synthesize delegates to the underlying provider.
func (s *Service) Synthesize(ctx context.Context, text string) (*Result, error) {
	if s == nil || s.provider == nil {
		return nil, fmt.Errorf("tts: no provider configured")
	}
	return s.provider.Synthesize(ctx, text)
}
