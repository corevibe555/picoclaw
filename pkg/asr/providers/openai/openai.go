// Package openai implements OpenAI Whisper-compatible HTTP ASR (audio/transcriptions).
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// Provider calls OpenAI-compatible /audio/transcriptions.
type Provider struct {
	cfg    config.OpenAIASRConfig
	client *http.Client
}

// New builds an ASR provider from config.
func New(cfg config.OpenAIASRConfig) *Provider {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "whisper-1"
	}
	return &Provider{
		cfg: config.OpenAIASRConfig{
			APIKey:   cfg.APIKey,
			BaseURL:  base,
			Model:    model,
			Language: cfg.Language,
		},
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

// Name implements asr.Provider.
func (p *Provider) Name() string { return "openai" }

// Transcribe implements asr.Provider.
func (p *Provider) Transcribe(ctx context.Context, audioPath string) (string, error) {
	logger.InfoCF("asr.openai", "Transcribing", map[string]any{"file": filepath.Base(audioPath)})

	f, err := os.Open(audioPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", filepath.Base(audioPath))
	if err != nil {
		return "", err
	}
	if _, err = io.Copy(part, f); err != nil {
		return "", err
	}
	_ = w.WriteField("model", p.cfg.Model)
	if lang := strings.TrimSpace(p.cfg.Language); lang != "" {
		_ = w.WriteField("language", lang)
	}
	_ = w.WriteField("response_format", "json")
	if err = w.Close(); err != nil {
		return "", err
	}

	url := p.cfg.BaseURL + "/audio/transcriptions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("asr openai: status %d: %s", resp.StatusCode, string(raw))
	}

	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("asr openai: decode: %w", err)
	}
	return out.Text, nil
}
