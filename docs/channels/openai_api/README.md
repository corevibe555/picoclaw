# OpenAI API channel (`openai_api`)

Exposes an [OpenAI-compatible](https://platform.openai.com/docs/api-reference/chat/create) HTTP surface on the **gateway** process (same host and port as `gateway.host` / `gateway.port`), so existing clients can point their base URL at Picoclaw without protocol changes.

## Configuration

In `config.json` under `channels.openai_api`:

| Field | Description |
|-------|-------------|
| `enabled` | Set `true` to enable the channel. |
| `api_key` | If non-empty, clients must send `Authorization: Bearer <api_key>`. If empty, **no auth** (suitable only for trusted local use). |
| `path_prefix` | HTTP prefix for routes (default `"/openai"`). Endpoints are `<prefix>/v1/chat/completions` and `<prefix>/v1/models`. |
| `allow_from` | Optional allow-list (same semantics as other channels). Use entries like `openai_api:<user>` matching the OpenAI `user` field, or leave empty to allow all. |
| `request_timeout_sec` | Max time to wait for the agent reply (default `300`). |

**Port:** Use the global gateway port (`gateway.port`, env `PICOCLAW_GATEWAY_PORT`), not a separate listener.

## Example

With default path prefix and gateway on `127.0.0.1:18789`:

```bash
curl http://127.0.0.1:18789/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_KEY" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello!"}],
    "user": "my-app-user-id"
  }'
```

Point OpenAI SDKs at base URL `http://127.0.0.1:18789/openai` (no `/v1` suffix; the client adds `/v1/...`).

## Limitations

- `stream: true` is not supported yet (returns HTTP 501).
- Token `usage` in responses is not populated (zeros).
