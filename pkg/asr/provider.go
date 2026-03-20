package asr

import "context"

// Provider performs speech-to-text on a local audio file path.
type Provider interface {
	Name() string
	Transcribe(ctx context.Context, audioPath string) (text string, err error)
}
