package tts

import "context"

// Result holds synthesized audio bytes and a MIME type hint.
type Result struct {
	Data        []byte
	ContentType string
}

// Provider performs text-to-speech.
type Provider interface {
	Name() string
	Synthesize(ctx context.Context, text string) (*Result, error)
}
