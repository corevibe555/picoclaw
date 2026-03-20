package asr

import (
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/voice"
)

// SelectTranscriber returns a transcriber when audio.asr is enabled and configured.
// Returns nil if audio ASR is not active or misconfigured (not an error).
func SelectTranscriber(cfg *config.Config) voice.Transcriber {
	if cfg == nil || !cfg.Audio.Enabled || cfg.Audio.ASR.Provider == "" {
		return nil
	}
	s, err := NewService(cfg)
	if err != nil || s == nil {
		return nil
	}
	return s.Transcriber()
}
