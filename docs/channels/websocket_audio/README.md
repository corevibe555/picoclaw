# WebSocket audio channel (`websocket_audio`)

Gateway-integrated **non-streaming** voice/text channel for [issue #1648](https://github.com/sipeed/picoclaw/issues/1648). It uses the shared `audio` ASR/TTS configuration and exposes a WebSocket at:

`ws(s)://<gateway_host>:<gateway_port><path_prefix>/ws`

Default `path_prefix` is `/ws/audio` → `ws://127.0.0.1:<port>/ws/audio/ws`.

## Prerequisites

1. Set `audio.enabled` and configure `audio.asr` / `audio.tts` providers (see below).
2. Enable this channel and set `channels.websocket_audio.token` (required).

## Authentication

- Query: `?token=<token>`
- Header: `Authorization: Bearer <token>`
- WebSocket subprotocol: `token.<token>`

## Client messages (JSON text frames)

| `type` | Fields | Mode |
|--------|--------|------|
| `user_message` | `text`, optional `user` | Text in → agent → text/audio out |
| `audio_input` | `format` (hint), `data` (base64), optional `user` | Voice in (requires `channels.websocket_audio.audio.enable_input` and working ASR) |

## Server messages

- `user_text` — transcript after `audio_input` (before assistant reply).
- `assistant_text` — final reply as text (ASR-only or when TTS fails/disabled).
- `assistant_audio` — `content_type`, `format`, `data` (base64) when TTS succeeds and `enable_output` is true.
- `error` — `code`, `message`.

## Modes (`channels.websocket_audio.audio`)

- **Text only**: `enable_input: false`, `enable_output: false` — use `user_message` only.
- **ASR only**: `enable_input: true`, `enable_output: false` — voice in, text out.
- **TTS only**: `enable_input: false`, `enable_output: true` — `user_message`, audio reply.
- **Full voice**: both `true` — `audio_input`, audio reply.

Streaming ASR/LLM/TTS pipelines are **not** implemented in this channel yet.

## Example `config.json` snippets

```json
"audio": {
  "enabled": true,
  "asr": {
    "provider": "openai",
    "openai": {
      "api_key": "sk-...",
      "base_url": "https://api.openai.com/v1",
      "model": "whisper-1"
    }
  },
  "tts": {
    "provider": "openai",
    "openai": {
      "api_key": "sk-...",
      "model": "tts-1",
      "voice": "alloy",
      "response_format": "mp3"
    }
  }
},
"channels": {
  "websocket_audio": {
    "enabled": true,
    "token": "your-ws-secret",
    "path_prefix": "/ws/audio",
    "audio": {
      "enable_input": true,
      "enable_output": true,
      "input_format": "wav",
      "output_format": "mp3"
    }
  }
}
```

ASR provider `groq` reuses the same Groq API key discovery as `voice` (see `providers.groq` or a `groq/` model in `model_list`).
