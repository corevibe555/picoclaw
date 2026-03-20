// Package groq implements ASR via Groq's OpenAI-compatible transcription API.
package groq

import (
	"context"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/voice"
)

// Provider wraps voice.GroqTranscriber for the asr.Service.
type Provider struct {
	inner *voice.GroqTranscriber
}

// NewFromConfig returns a Groq provider if API keys are available (same rules as voice.DetectTranscriber).
func NewFromConfig(cfg *config.Config) *Provider {
	if cfg == nil {
		return nil
	}
	if key := strings.TrimSpace(cfg.Providers.Groq.APIKey); key != "" {
		return &Provider{inner: voice.NewGroqTranscriber(key)}
	}
	for _, mc := range cfg.ModelList {
		if strings.HasPrefix(mc.Model, "groq/") && mc.APIKey != "" {
			return &Provider{inner: voice.NewGroqTranscriber(mc.APIKey)}
		}
	}
	return nil
}

// Name implements asr.Provider.
func (p *Provider) Name() string { return "groq" }

// Transcribe implements asr.Provider.
func (p *Provider) Transcribe(ctx context.Context, audioPath string) (string, error) {
	r, err := p.inner.Transcribe(ctx, audioPath)
	if err != nil {
		return "", err
	}
	if r == nil {
		return "", nil
	}
	return r.Text, nil
}
