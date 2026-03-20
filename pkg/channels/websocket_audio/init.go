package websocket_audio

import (
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
)

func init() {
	channels.RegisterFactory("websocket_audio", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewChannel(cfg, b)
	})
}
