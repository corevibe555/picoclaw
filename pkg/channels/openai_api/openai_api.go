// Package openai_api implements an OpenAI-compatible HTTP channel (Chat Completions API)
// mounted on the gateway HTTP server. Embed Picoclaw behind existing OpenAI clients by
// setting base URL to http://<gateway_host>:<gateway_port><path_prefix>.
package openai_api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

const channelName = "openai_api"

// OpenAIAPIChannel serves /v1/chat/completions (and /v1/models) under a configurable path prefix.
type OpenAIAPIChannel struct {
	*channels.BaseChannel
	bus           *bus.MessageBus
	cfg           config.OpenAIAPIConfig
	pathPrefix    string // normalized, no trailing slash, e.g. "/openai"
	waiters       sync.Map // chatID -> chan string (single assistant reply)
	responseModel string   // advertised model id for JSON responses
}

// NewOpenAIAPIChannel constructs the channel.
func NewOpenAIAPIChannel(cfg config.OpenAIAPIConfig, messageBus *bus.MessageBus) (*OpenAIAPIChannel, error) {
	prefix := normalizePathPrefix(cfg.PathPrefix)
	base := channels.NewBaseChannel(channelName, cfg, messageBus, cfg.AllowFrom,
		channels.WithReasoningChannelID(cfg.ReasoningChannelID),
	)
	return &OpenAIAPIChannel{
		BaseChannel:   base,
		bus:           messageBus,
		cfg:           cfg,
		pathPrefix:    prefix,
		responseModel: "picoclaw",
	}, nil
}

func normalizePathPrefix(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/openai"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimRight(p, "/")
}

// Start implements channels.Channel.
func (c *OpenAIAPIChannel) Start(ctx context.Context) error {
	logger.InfoC("openai_api", "OpenAI API channel started")
	c.SetRunning(true)
	return nil
}

// Stop implements channels.Channel.
func (c *OpenAIAPIChannel) Stop(ctx context.Context) error {
	c.SetRunning(false)
	logger.InfoC("openai_api", "OpenAI API channel stopped")
	return nil
}

// WebhookPath implements channels.WebhookHandler — mount point on the shared gateway HTTP server.
func (c *OpenAIAPIChannel) WebhookPath() string {
	return c.pathPrefix + "/"
}

// ServeHTTP handles OpenAI-compatible routes under pathPrefix.
func (c *OpenAIAPIChannel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !c.IsRunning() {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}

	rel := strings.TrimPrefix(r.URL.Path, c.pathPrefix)
	rel = strings.TrimPrefix(rel, "/")

	switch {
	case r.Method == http.MethodPost && rel == "v1/chat/completions":
		c.handleChatCompletions(w, r)
	case r.Method == http.MethodGet && rel == "v1/models":
		c.handleModels(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (c *OpenAIAPIChannel) handleModels(w http.ResponseWriter, r *http.Request) {
	if !c.authenticate(w, r) {
		return
	}
	now := time.Now().Unix()
	resp := map[string]any{
		"object": "list",
		"data": []map[string]any{
			{
				"id":       c.responseModel,
				"object":   "model",
				"created":  now,
				"owned_by": "picoclaw",
			},
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (c *OpenAIAPIChannel) authenticate(w http.ResponseWriter, r *http.Request) bool {
	want := strings.TrimSpace(c.cfg.APIKey)
	if want == "" {
		return true
	}
	auth := r.Header.Get("Authorization")
	token, ok := strings.CutPrefix(auth, "Bearer ")
	token = strings.TrimSpace(token)
	if !ok || token != want {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid_api_key", "Incorrect API key provided.")
		return false
	}
	return true
}

func (c *OpenAIAPIChannel) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if !c.authenticate(w, r) {
		return
	}

	var req chatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "Invalid JSON body")
		return
	}
	if req.Stream {
		writeOpenAIError(w, http.StatusNotImplemented, "not_implemented", "Streaming (stream: true) is not supported yet")
		return
	}
	if len(req.Messages) == 0 {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "messages is required")
		return
	}

	userID := strings.TrimSpace(req.User)
	if userID == "" {
		userID = "anonymous"
	}

	sender := bus.SenderInfo{
		Platform:    "openai_api",
		PlatformID:  userID,
		CanonicalID: "openai_api:" + userID,
		Username:    userID,
	}
	if !c.IsAllowedSender(sender) {
		writeOpenAIError(w, http.StatusForbidden, "permission_denied", "Sender is not allowed")
		return
	}

	content, err := flattenChatMessages(req.Messages)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	chatID := "openai_api:" + uuid.New().String()
	replyCh := make(chan string, 1)
	c.waiters.Store(chatID, replyCh)
	defer c.waiters.Delete(chatID)

	timeout := c.cfg.RequestTimeoutSec
	if timeout <= 0 {
		timeout = 300
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeout)*time.Second)
	defer cancel()

	in := bus.InboundMessage{
		Channel:  channelName,
		SenderID: userID,
		Sender:   sender,
		ChatID:   chatID,
		Content:  content,
		Peer: bus.Peer{
			Kind: "direct",
			ID:   userID,
		},
	}

	if err := c.bus.PublishInbound(ctx, in); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}

	select {
	case text := <-replyCh:
		if text == "" {
			text = " "
		}
		c.writeChatCompletionResponse(w, req.Model, text)
	case <-ctx.Done():
		writeOpenAIError(w, http.StatusGatewayTimeout, "timeout", "Request timed out waiting for agent response")
	}
}

// Send delivers the agent reply to the waiting HTTP request.
func (c *OpenAIAPIChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
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

func (c *OpenAIAPIChannel) writeChatCompletionResponse(w http.ResponseWriter, model string, content string) {
	if model == "" {
		model = c.responseModel
	}
	id := "chatcmpl-" + uuid.New().String()
	now := time.Now().Unix()
	resp := map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": now,
		"model":   model,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeOpenAIError(w http.ResponseWriter, status int, typ, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"message": msg,
			"type":    typ,
		},
	})
}

// --- Request/response types (minimal OpenAI compatibility)

type chatCompletionRequest struct {
	Model    string           `json:"model"`
	Messages []map[string]any `json:"messages"`
	Stream   bool             `json:"stream"`
	User     string           `json:"user"`
}

func flattenChatMessages(messages []map[string]any) (string, error) {
	var b strings.Builder
	for i, m := range messages {
		role, _ := m["role"].(string)
		if role == "" {
			return "", fmt.Errorf("messages[%d].role is required", i)
		}
		content := m["content"]
		text, err := contentToString(content)
		if err != nil {
			return "", fmt.Errorf("messages[%d].content: %w", i, err)
		}
		b.WriteString(strings.ToUpper(role))
		b.WriteString(": ")
		b.WriteString(text)
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String()), nil
}

func contentToString(content any) (string, error) {
	if content == nil {
		return "", errors.New("content is required")
	}
	switch v := content.(type) {
	case string:
		return v, nil
	case []any:
		var parts []string
		for _, item := range v {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := obj["type"].(string); t == "text" {
				if txt, ok := obj["text"].(string); ok {
					parts = append(parts, txt)
				}
			}
		}
		if len(parts) == 0 {
			raw, err := json.Marshal(v)
			if err != nil {
				return "", err
			}
			return string(raw), nil
		}
		return strings.Join(parts, "\n"), nil
	default:
		raw, err := json.Marshal(content)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	}
}
