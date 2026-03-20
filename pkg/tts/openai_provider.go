package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

type openAIProvider struct {
	cfg    config.OpenAITTSConfig
	client *http.Client
}

func newOpenAIProvider(cfg config.OpenAITTSConfig) *openAIProvider {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "tts-1"
	}
	voice := strings.TrimSpace(cfg.Voice)
	if voice == "" {
		voice = "alloy"
	}
	format := strings.TrimSpace(cfg.ResponseFormat)
	if format == "" {
		format = "mp3"
	}
	return &openAIProvider{
		cfg: config.OpenAITTSConfig{
			APIKey:         cfg.APIKey,
			BaseURL:        base,
			Model:          model,
			Voice:          voice,
			Speed:          cfg.Speed,
			ResponseFormat: format,
		},
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *openAIProvider) Name() string { return "openai" }

func (p *openAIProvider) Synthesize(ctx context.Context, text string) (*Result, error) {
	logger.DebugCF("tts.openai", "Synthesize", map[string]any{"chars": len(text)})

	body := map[string]any{
		"model":           p.cfg.Model,
		"input":           text,
		"voice":           p.cfg.Voice,
		"response_format": p.cfg.ResponseFormat,
	}
	if p.cfg.Speed > 0 {
		body["speed"] = p.cfg.Speed
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	url := p.cfg.BaseURL + "/audio/speech"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tts openai: status %d: %s", resp.StatusCode, string(data))
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		switch p.cfg.ResponseFormat {
		case "opus":
			ct = "audio/opus"
		case "wav":
			ct = "audio/wav"
		case "aac":
			ct = "audio/aac"
		default:
			ct = "audio/mpeg"
		}
	}
	return &Result{Data: data, ContentType: ct}, nil
}
