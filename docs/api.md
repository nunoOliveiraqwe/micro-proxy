# Management API

The API server runs on `127.0.0.1:27000` by default. All endpoints are prefixed with `/api/v1`.

## Authentication

Most endpoints require a valid session. Log in via `POST /api/v1/auth/login` to get a session cookie. Some endpoints also accept API key authentication via the `X-API-Key` header.

## Endpoints

| Endpoint | Method | Auth | Description |
|---|---|---|---|
| `/healthcheck` | `GET` | No | Health check |
| `/fts` | `GET` | No | First-time setup status |
| `/fts` | `POST` | No | Complete first-time setup (set admin password) |
| `/auth/login` | `POST` | No | Login (returns session cookie) |
| `/auth/logout` | `POST` | Yes | Logout (destroys session) |
| `/auth/user` | `GET` | Yes | Get current user identity |
| `/auth/user` | `POST` | Yes | Change password |
| `/system/health` | `GET` | Yes | System health (memory, goroutines, GC, uptime) |
| `/system/interfaces` | `GET` | Yes | Available network interfaces |
| `/proxy/routes` | `GET` | Yes | List all configured proxy servers |
| `/proxy/routes/http` | `POST` | Yes | Create a new HTTP proxy server |
| `/proxy/routes/{serverId}/start` | `POST` | Yes | Start a stopped proxy |
| `/proxy/routes/{serverId}/stop` | `POST` | Yes | Stop a running proxy |
| `/proxy/routes/{serverId}` | `DELETE` | Yes | Delete a proxy |
| `/proxy/metrics` | `GET` | Yes | Global aggregated metrics |
| `/proxy/metrics/{serverId}` | `GET` | Yes | Per-server metrics |
| `/proxy/metrics/stream` | `GET` | Yes | Real-time metrics (SSE) |
| `/proxy/middlewares/schema` | `GET` | Yes | Middleware field schemas |
| `/proxy/requests` | `GET` | Yes | Recent proxied request log |
| `/proxy/errors` | `GET` | Yes | Recent 5xx error log |
| `/acme/config` | `GET` | Yes | Get ACME configuration |
| `/acme/config` | `POST` | Yes | Save ACME configuration |
| `/acme/config` | `PATCH` | Yes | Toggle ACME enabled/disabled |
| `/acme/certificates` | `GET` | Yes | List managed TLS certificates |
| `/acme/providers` | `GET` | Yes | Supported DNS providers |
| `/acme` | `DELETE` | Yes | Reset all ACME data |
| `/apiKeys` | `GET` | Yes | List API keys |
| `/apiKeys` | `POST` | Yes | Create an API key |
| `/apiKeys/{id}` | `DELETE` | Yes | Delete an API key |

## API Keys

API keys support scoped access for external integrations:

| Scope | Description |
|---|---|
| `read_stats` | Read metrics and request/error logs |

