<p align="center">
<pre>
  _              _ _
 | |_ ___  _ __ (_|_)
 | __/ _ \| '_ \| | |
 | || (_) | |   | | |
  \__\___/|_|   |_|_|
</pre>
</p>

<p align="center">
  <b>A reverse proxy that gets out of your way.</b>
</p>

---

> **Early development.** Torii is being actively built. Expect rough edges, breaking changes, and new features showing up regularly.

## What is Torii?

Torii (鳥居) is the gate that stands at the entrance of a shrine — a threshold between the outside world and what's inside. This project is the same idea applied to your services: a single gateway that handles routing, TLS, security, and observability so your backends don't have to.

It started as a small home-lab proxy, and it still works great for that. But it's grown into something more capable — a lightweight, self-contained reverse proxy with a real management layer, designed around simplicity without sacrificing the features you actually need.

## Features

### Reverse Proxy
- **HTTP and HTTPS** listeners with independent configuration per port.
- **Virtual-host routing** — route traffic by `Host` header to different backends.
- **Path-based routing** — apply different middleware chains to specific URL patterns within a route, with wildcard support (`/api/*`, `/users/*/jobs`).
- **Default fallback** — catch-all backend when no route matches.
- **Dual-stack** — bind to IPv4, IPv6, or both per listener.

### TLS & ACME
- **Automatic TLS** via Let's Encrypt with DNS-01 challenges (no port 80 required).
- **Bring your own certificates** — point to a cert and key file.
- **Automatic renewal** — background loop checks and renews certificates before expiry.
- **Per-domain SNI** — each hostname gets its own certificate, resolved at TLS handshake time.
- **Pluggable DNS providers** — Cloudflare supported out of the box, with an extensible provider registry.

### Middleware
Middlewares are configured per-route, per-path, or globally. They run in the order you define them.

| Middleware | What it does |
|---|---|
| **`Metrics`** | Tracks request count, latency percentiles (P50/P95/P99), status code distribution, bytes in/out, upstream timeouts, and active connections. |
| **`RequestId`** | Generates a unique ID per request and injects it into the context. Supports custom prefixes. |
| **`RequestLog`** | Structured request logging with method, URL, host, remote address, user agent, and request ID via zap. |
| **`Headers`** | Set, strip, or compare headers on requests and responses. Supports `$env:VAR` and `$file:/path` value resolution. |
| **`RateLimiter`** | Token-bucket rate limiting — global or per-client IP. Configurable rate, burst, cache TTL, and cleanup. Returns `429` with `Retry-After`. |
| **`CountryBlock`** | GeoIP-based allow/block lists using MaxMind databases. Supports remote DB download with auto-refresh and in-memory caching. |
| **`IpBlock`** | IP-based allow/block lists with CIDR support. *(in progress)* |

### Management API
A REST API (default `127.0.0.1:27000`) for managing the proxy at runtime:

- **First-time setup** — set admin credentials on first launch.
- **Authentication** — session-based login/logout with secure cookies.
- **Proxy control** — list configured proxy servers, start/stop individual listeners.
- **Metrics** — global and per-server metrics, plus a real-time SSE stream.
- **Observability** — recent request log, recent error log, system health (memory, goroutines, GC, uptime).
- **ACME management** — view/update ACME configuration and list managed certificates.
- **Password management** — change the admin password.

### Web UI
A built-in admin interface served at `/ui` with:
- **Setup wizard** — first-time setup flow for setting the admin password.
- **Login page** — session-based authentication.
- **Dashboard** — live view of proxy state and metrics.

### Storage
- **SQLite** with WAL mode — single-file database, no external dependencies.
- **Embedded migrations** — schema versioning runs automatically on startup.
- Stores users, roles, sessions, system configuration, ACME accounts, certificates, and proxy metrics.

### Debug Mode
Pass `--debug` to automatically start stub HTTP and TCP servers for every configured backend. Useful for development and testing without real upstream services.

## Quick Start

### Build

```bash
go build -o torii ./cmd/torii
```

### Create a config file

```yaml
log:
  logLevel: INFO

apiServer:
  host: 127.0.0.1
  port: 27000

netConfig:
  http:
    - port: 80
      bind: 3                          # 1 = IPv4, 2 = IPv6, 3 = both
      default:
        backend: http://192.168.1.50:8080
        middlewares:
          - type: "RequestId"
          - type: "RequestLog"
          - type: "Metrics"

    - port: 443
      bind: 3
      tls:
        use-acme: true
      routes:
        - host: app.home.local
          target:
            backend: http://192.168.1.50:8080
            middlewares:
              - type: "RequestId"
              - type: "RequestLog"
              - type: "Metrics"
              - type: "RateLimiter"
                options:
                  mode: "per-client"
                  cache-ttl: 24h
                  cleanup-interval: 1h
                  max-cache-size: 10000
                  limiter-req:
                    rate-per-second: 10
                    burst: 20
      default:
        backend: http://192.168.1.50:8080
```

### Run

```bash
./torii --config config.yaml
```

On first launch, open `http://127.0.0.1:27000/ui` to complete the setup wizard and set your admin password.

## Configuration Reference

### Top-level

| Key | Type | Default | Description |
|---|---|---|---|
| `log.logLevel` | `string` | `INFO` | Log level: `DEBUG`, `INFO`, `WARN`, `ERROR`. |
| `log.logPath` | `string` | | File path to write logs to. |
| `log.logDebug` | `bool` | `false` | Enable development-mode logging. |
| `apiServer.host` | `string` | `127.0.0.1` | Management API bind address. |
| `apiServer.port` | `int` | `27000` | Management API port. |

### HTTP Listener (`netConfig.http[]`)

| Key | Type | Description |
|---|---|---|
| `port` | `int` | Port to listen on. |
| `bind` | `int` | `1` = IPv4, `2` = IPv6, `3` = both. |
| `interface` | `string` | Network interface name to bind to. Defaults to loopback. |
| `read-timeout` | `duration` | Server read timeout. |
| `read-header-timeout` | `duration` | Server read-header timeout. |
| `write-timeout` | `duration` | Server write timeout. |
| `idle-timeout` | `duration` | Server idle timeout. |
| `tls.use-acme` | `bool` | Enable automatic TLS via ACME DNS-01. |
| `tls.cert` | `string` | Path to TLS certificate (manual TLS). |
| `tls.key` | `string` | Path to TLS private key (manual TLS). |
| `routes[].host` | `string` | Hostname to match (virtual hosting). |
| `routes[].target.backend` | `string` | Backend URL to proxy to. |
| `routes[].target.middlewares` | `list` | Middleware chain for this route. |
| `routes[].target.paths[]` | `list` | Path-specific rules with their own middleware chains. |
| `default.backend` | `string` | Catch-all backend when no host matches. |
| `default.middlewares` | `list` | Middleware chain for the default route. |

### Global Middlewares (`netConfig.global`)

| Key | Type | Description |
|---|---|---|
| `middlewares` | `list` | Middleware chain applied to all requests on all listeners. |

### Session (`session`)

| Key | Type | Default | Description |
|---|---|---|---|
| `lifetime` | `duration` | `16h` | Maximum session lifetime. |
| `idleTimeout` | `duration` | `60m` | Session idle timeout. |
| `cleanupInterval` | `duration` | `1h` | Expired session cleanup interval. |
| `cookieSecure` | `bool` | `true` | Set the `Secure` flag on session cookies. |
| `cookieHttpOnly` | `bool` | `true` | Set the `HttpOnly` flag on session cookies. |
| `cookieSameSite` | `string` | `lax` | `SameSite` cookie attribute: `strict`, `lax`, or `none`. |

## CLI Flags

```
--config <path>       Path to the YAML configuration file.
--debug               Start stub servers for all configured backends.
--log-level <level>   Override the log level from config.
```

## Architecture

```
                    ┌─────────────────────────────────────────┐
                    │              torii                       │
  Incoming          │                                         │
  Request ─────────►│  Global Middlewares                      │
                    │    │                                     │
                    │    ▼                                     │
                    │  Virtual Host Dispatcher (by Host)       │
                    │    │                                     │
                    │    ▼                                     │
                    │  Route Middlewares                       │
                    │    │                                     │
                    │    ▼                                     │
                    │  Path Dispatcher (by URL pattern)        │
                    │    │                                     │
                    │    ▼                                     │
                    │  Path Middlewares                        │
                    │    │                                     │
                    │    ▼                                     │
                    │  httputil.ReverseProxy ──────────► Backend│
                    └─────────────────────────────────────────┘

  Management        ┌─────────────────────────────────────────┐
  (localhost)       │  API Server (:27000)                     │
                    │    ├── /ui          Web dashboard        │
                    │    ├── /api/v1      REST API             │
                    │    └── SSE stream   Real-time metrics    │
                    │                                         │
                    │  SQLite (torii.db)                       │
                    │    ├── Users & sessions                  │
                    │    ├── ACME accounts & certificates      │
                    │    └── System configuration              │
                    └─────────────────────────────────────────┘
```

## Requirements

- **Go 1.25+**

## License

This project is licensed under the [Apache License 2.0](LICENSE).
