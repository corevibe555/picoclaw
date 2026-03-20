// Package websocket_audio provides a gateway WebSocket channel for voice/text interaction
// using pkg/asr and pkg/tts (issue #1648). Non-streaming only.
package websocket_audio

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/asr"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/tts"
)

const channelName = "websocket_audio"

// Channel serves JSON WebSocket messages with optional ASR input and TTS output.
type Channel struct {
	*channels.BaseChannel
	bus      *bus.MessageBus
	cfg      config.WebSocketAudioConfig
	asrSvc   *asr.Service
	ttsSvc   *tts.Service
	prefix   string
	waiters  sync.Map // chatID -> chan string
	upgrader websocket.Upgrader
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewChannel builds the channel from full config.
func NewChannel(cfg *config.Config, b *bus.MessageBus) (*Channel, error) {
	ws := cfg.Channels.WebSocketAudio
	if strings.TrimSpace(ws.Token) == "" {
		return nil, fmt.Errorf("websocket_audio: token is required")
	}
	prefix := normalizePathPrefix(ws.PathPrefix)
	base := channels.NewBaseChannel(channelName, ws, b, ws.AllowFrom,
		channels.WithReasoningChannelID(ws.ReasoningChannelID),
	)
	var asrSvc *asr.Service
	var ttsSvc *tts.Service
	if cfg.Audio.Enabled {
		var err error
		asrSvc, err = asr.NewService(cfg)
		if err != nil {
			logger.WarnCF("websocket_audio", "ASR service unavailable", map[string]any{"error": err.Error()})
		}
		ttsSvc, err = tts.NewService(cfg)
		if err != nil {
			logger.WarnCF("websocket_audio", "TTS service unavailable", map[string]any{"error": err.Error()})
		}
	}
	allowOrigins := ws.AllowOrigins
	ch := &Channel{
		BaseChannel: base,
		bus:         b,
		cfg:         ws,
		asrSvc:      asrSvc,
		ttsSvc:      ttsSvc,
		prefix:      prefix,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				if len(allowOrigins) == 0 {
					return true
				}
				o := r.Header.Get("Origin")
				for _, a := range allowOrigins {
					if a == "*" || a == o {
						return true
					}
				}
				return false
			},
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
		},
	}
	return ch, nil
}

func normalizePathPrefix(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/ws/audio"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimRight(p, "/")
}

// WebhookPath implements channels.WebhookHandler.
func (c *Channel) WebhookPath() string { return c.prefix + "/" }

// ServeHTTP upgrades to WebSocket on <path_prefix>/ws.
func (c *Channel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !c.IsRunning() {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	suffix := strings.TrimPrefix(r.URL.Path, c.prefix)
	suffix = strings.Trim(suffix, "/")
	if suffix != "ws" {
		http.NotFound(w, r)
		return
	}
	if !c.authenticate(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := c.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.ErrorCF("websocket_audio", "WebSocket upgrade failed", map[string]any{"error": err.Error()})
		return
	}
	go c.readLoop(conn)
}

func (c *Channel) authenticate(r *http.Request) bool {
	tok := strings.TrimSpace(c.cfg.Token)
	if tok == "" {
		return false
	}
	if r.URL.Query().Get("token") == tok {
		return true
	}
	auth := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(auth, "Bearer "); ok && strings.TrimSpace(after) == tok {
		return true
	}
	for _, proto := range websocket.Subprotocols(r) {
		if after, ok := strings.CutPrefix(proto, "token."); ok && after == tok {
			return true
		}
	}
	return false
}

func (c *Channel) readLoop(conn *websocket.Conn) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var msg wsClientMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			_ = c.writeError(conn, "invalid_json", err.Error())
			continue
		}
		c.handleClientMessage(context.Background(), conn, msg)
	}
}

type wsClientMsg struct {
	Type   string `json:"type"`
	Text   string `json:"text"`
	Format string `json:"format"`
	Data   string `json:"data"` // base64
	User   string `json:"user"`
}

func (c *Channel) handleClientMessage(ctx context.Context, conn *websocket.Conn, msg wsClientMsg) {
	modes := c.cfg.Audio
	userID := strings.TrimSpace(msg.User)
	if userID == "" {
		userID = "anonymous"
	}
	sender := bus.SenderInfo{
		Platform:    "websocket_audio",
		PlatformID:  userID,
		CanonicalID: "websocket_audio:" + userID,
		Username:    userID,
	}
	if !c.IsAllowedSender(sender) {
		_ = c.writeError(conn, "forbidden", "not allowed")
		return
	}

	var userText string
	switch msg.Type {
	case "user_message":
		userText = strings.TrimSpace(msg.Text)
		if userText == "" {
			_ = c.writeError(conn, "invalid_request", "text is required")
			return
		}
		if modes.EnableInput {
			// text path still allowed alongside ASR
		}
	case "audio_input":
		if !modes.EnableInput {
			_ = c.writeError(conn, "invalid_request", "audio input disabled")
			return
		}
		if c.asrSvc == nil || c.asrSvc.Transcriber() == nil {
			_ = c.writeError(conn, "asr_unavailable", "configure audio.enabled and audio.asr with a valid provider")
			return
		}
		raw, err := base64.StdEncoding.DecodeString(msg.Data)
		if err != nil || len(raw) == 0 {
			_ = c.writeError(conn, "invalid_audio", "expected base64 data")
			return
		}
		tmp, err := os.CreateTemp("", "ws-audio-*")
		if err != nil {
			_ = c.writeError(conn, "internal", err.Error())
			return
		}
		path := tmp.Name()
		_, _ = tmp.Write(raw)
		_ = tmp.Close()
		defer os.Remove(path)

		tr := c.asrSvc.Transcriber()
		res, err := tr.Transcribe(ctx, path)
		if err != nil {
			_ = c.writeError(conn, "asr_failed", err.Error())
			return
		}
		if res == nil || strings.TrimSpace(res.Text) == "" {
			_ = c.writeError(conn, "asr_empty", "no speech recognized")
			return
		}
		userText = res.Text
	default:
		_ = c.writeError(conn, "unknown_type", msg.Type)
		return
	}

	timeout := c.cfg.RequestTimeoutSec
	if timeout <= 0 {
		timeout = 300
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	chatID := channelName + ":" + uuid.New().String()
	replyCh := make(chan string, 1)
	c.waiters.Store(chatID, replyCh)
	defer c.waiters.Delete(chatID)

	in := bus.InboundMessage{
		Channel:  channelName,
		SenderID: userID,
		Sender:   sender,
		ChatID:   chatID,
		Content:  userText,
		Peer: bus.Peer{
			Kind: "direct",
			ID:   userID,
		},
	}
	if err := c.bus.PublishInbound(reqCtx, in); err != nil {
		_ = c.writeError(conn, "bus", err.Error())
		return
	}

	var reply string
	select {
	case reply = <-replyCh:
	case <-reqCtx.Done():
		_ = c.writeError(conn, "timeout", "agent did not respond in time")
		return
	}

	// Optional: echo user transcript for ASR path
	if msg.Type == "audio_input" {
		_ = c.writeJSON(conn, map[string]any{
			"type":    "user_text",
			"content": userText,
		})
	}

	if modes.EnableOutput && c.ttsSvc != nil {
		ttsCtx, ttsCancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer ttsCancel()
		out, err := c.ttsSvc.Synthesize(ttsCtx, reply)
		if err != nil {
			logger.WarnCF("websocket_audio", "TTS failed, falling back to text", map[string]any{"error": err.Error()})
			_ = c.writeJSON(conn, map[string]any{"type": "assistant_text", "content": reply})
			return
		}
		b64 := base64.StdEncoding.EncodeToString(out.Data)
		_ = c.writeJSON(conn, map[string]any{
			"type":         "assistant_audio",
			"content_type": out.ContentType,
			"format":       strings.TrimPrefix(c.cfg.Audio.OutputFormat, "."),
			"data":         b64,
		})
		return
	}

	_ = c.writeJSON(conn, map[string]any{"type": "assistant_text", "content": reply})
}

func (c *Channel) writeJSON(conn *websocket.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, b)
}

func (c *Channel) writeError(conn *websocket.Conn, code, message string) error {
	return c.writeJSON(conn, map[string]any{"type": "error", "code": code, "message": message})
}

// Start implements channels.Channel.
func (c *Channel) Start(ctx context.Context) error {
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.SetRunning(true)
	logger.InfoC("websocket_audio", "WebSocket audio channel started")
	return nil
}

// Stop implements channels.Channel.
func (c *Channel) Stop(ctx context.Context) error {
	c.SetRunning(false)
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

// Send implements channels.Channel — deliver agent text to waiting WebSocket turn.
func (c *Channel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	if !c.IsRunning() {
		return channels.ErrNotRunning
	}
	if v, ok := c.waiters.Load(msg.ChatID); ok {
		ch := v.(chan string)
		select {
		case ch <- msg.Content:
		default:
		}
	}
	return nil
}
