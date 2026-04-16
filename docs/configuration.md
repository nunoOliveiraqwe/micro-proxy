# Configuration Reference

## Table of Contents

- [Top-level](#top-level)
- [HTTP Listener](#http-listener)
- [Path Rules](#path-rules)
- [Global Middlewares](#global-middlewares)
- [Session](#session)
- [CLI Flags](#cli-flags)
- [Middleware Reference](#middleware-reference)
  - [RequestId](#requestid)
  - [RequestLog](#requestlog)
  - [Metrics](#metrics)
  - [Cors](#cors)
  - [Headers](#headers)
  - [RateLimiter](#ratelimiter)
  - [IpBlock](#ipblock)
  - [CountryBlock](#countryblock)
  - [UserAgentBlocker](#useragentblocker)
  - [HoneyPot](#honeypot)
  - [CircuitBreaker](#circuitbreaker)
  - [Redirect](#redirect)
  - [BodySizeLimit](#bodysizelimit)
  - [Timeout](#timeout)
- [Full Example](#full-example)

---

## Top-level

| Key | Type | Default | Description |
|---|---|---|---|
| `log.log-level` | `string` | `INFO` | Log level: `DEBUG`, `INFO`, `WARN`, `ERROR`. |
| `log.log-path` | `string` | | File path to write logs to (in addition to stdout). |
| `log.log-debug` | `bool` | `false` | Enable development-mode logging. |
| `log.encoding` | `string` | `console` | Log format: `console` (human-readable) or `json` (structured). |
| `log.color` | `bool` | `true` | Colored log levels (only applies to `console` encoding). |
| `api-server.host` | `string` | `127.0.0.1` | Management API bind address. |
| `api-server.port` | `int` | `27000` | Management API port. |

## HTTP Listener (`net-config.http[]`)

| Key | Type | Description |
|---|---|---|
| `port` | `int` | Port to listen on. |
| `bind` | `int` | `1` = IPv4, `2` = IPv6, `3` = both. |
| `interface` | `string` | Network interface name to bind to (e.g. `en0`). Defaults to loopback. |
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

## Path Rules (`paths[]`)

| Key | Type | Description |
|---|---|---|
| `pattern` | `string` | URL pattern to match (supports `*` wildcards). |
| `backend` | `string` | Optional backend override for this path. |
| `drop-query` | `bool` | Strip query parameters before proxying. |
| `middlewares` | `list` | Middleware chain for this path. |

## Global Middlewares (`net-config.global`)

| Key | Type | Description |
|---|---|---|
| `middlewares` | `list` | Middleware chain applied to all requests on all listeners. |

## Session (`session`)

| Key | Type | Default | Description |
|---|---|---|---|
| `lifetime` | `duration` | `16h` | Maximum session lifetime. |
| `idle-timeout` | `duration` | `60m` | Session idle timeout. |
| `cleanup-interval` | `duration` | `1h` | Expired session cleanup interval. |
| `cookie-secure` | `bool` | `true` | Set the `Secure` flag on session cookies. |
| `cookie-http-only` | `bool` | `true` | Set the `HttpOnly` flag on session cookies. |
| `cookie-same-site` | `string` | `lax` | `SameSite` cookie attribute: `strict`, `lax`, or `none`. |

## CLI Flags

```
--config <path>       Path to the YAML configuration file.
--debug               Start stub servers for all configured backends.
--log-level <level>   Override the log level from config.
```

---

## Middleware Reference

Middlewares run in a fixed layer order:

```
Global middlewares → Route / Default middlewares → Path middlewares → Reverse Proxy
```

Within each layer, they run in the order you list them.

---

### RequestId

Generates a unique ID per request. If applied at multiple layers, deduplicates automatically.

```yaml
- type: "RequestId"
  options:
    prefix: "my-proxy"  # optional, auto-generated from hostname if omitted
```

---

### RequestLog

Structured request logging. No options.

```yaml
- type: "RequestLog"
```

---

### Metrics

Collects request/response metrics that power the dashboard. No options.

```yaml
- type: "Metrics"
```

---

### Cors

Handles CORS preflight and response headers.

```yaml
- type: "Cors"
  options:
    allow-origin: "*"
    allow-methods: "GET, POST, PUT, DELETE"
    allow-headers: "Content-Type, Authorization"
    max-age: 3600
```

---

### Headers

Manipulate request and response headers. Supports dynamic value resolution with `$file:/path/to/secret` and `$env:VAR`.

Use `cmp-headers-req` to require specific header values. Requests that don't match get a `401`. This is useful for shared-secret validation between services.

```yaml
- type: "Headers"
  options:
    set-headers-req:
      X-Forwarded-Proto: "https"
    set-headers-res:
      X-Api-Version: "1.0"
    strip-headers-req:
      - "X-Debug"
    strip-headers-res:
      - "Server"
    cmp-headers-req:
      X-Proxy-Id: "Torii"         # rejects with 401 if header doesn't match
```

---

### RateLimiter

Token-bucket rate limiting. Returns `429 Too Many Requests` with a `Retry-After` header.

```yaml
- type: "RateLimiter"
  options:
    mode: "per-client"             # "global" or "per-client"
    cache-ttl: 24h                 # per-client only
    cleanup-interval: 1h           # per-client only
    max-cache-size: 10000          # per-client only
    limiter-req:
      rate-per-second: 5
      burst: 10
```

---

### IpBlock

IP-based allow/block lists with CIDR support.

> **Note:** The middleware is registered but the filtering logic is not yet fully implemented.

```yaml
- type: "IpBlock"
  options:
    list-mode: allow               # "allow" or "block"
    list:
      - 192.168.1.0/24
      - 10.0.0.1
      - 2001:db8::/32
```

---

### CountryBlock

GeoIP-based blocking using MaxMind MMDB databases.

When both country and continent lists are defined, the country list acts as an exception override to the continent policy. For example: block all of EU, but allow PT and CA.

```yaml
- type: "CountryBlock"
  options:
    source:
      mode: "remote"               # "remote" or "local"
      path: "https://example.com/geo.mmdb"
      country-field: "country_code"
      continent-field: "continent_code"  # optional
      max-size: 300m               # remote only
      refresh-interval: 24h        # remote only
    cache-ttl: 24h
    cleanup-interval: 1h
    max-cache-size: 10000
    on-unknown: block              # "allow" or "block"
    country-list-mode: allow
    country-list: [PT, CA]
    continent-list-mode: block
    continent-list: [EU]
```

---

### UserAgentBlocker

Blocks requests from known bots, scanners, and unwanted user agents. Once a UA is blocked, the client IP is cached so subsequent requests are blocked immediately regardless of UA.

Built-in categories: `scanners`, `recon`, `scrapers`, `headless`, `malicious`, `http-clients`, `cli-tools`, `ai-crawlers`, `seo-bots`, `generic`, `social`, `search-engines`.

```yaml
- type: "UserAgentBlocker"
  options:
    cache-ttl: 24h
    cleanup-interval: 1h
    max-cache-size: 100000
    block-empty-ua: true
    block-defaults:
      - "scanners"
      - "recon"
      - "scrapers"
      - "ai-crawlers"
    block:
      - "custom-bad-bot"
    allow-defaults:
      - "search-engines"
    allow:
      - "my-good-bot"
```

---

### HoneyPot

Monitors access to common attack paths. Once an IP hits a honeypot path, all subsequent requests from that IP are blocked and cached.

Default path groups:
- `php`: `.env`, `wp-login.php`, `wp-admin`, `phpmyadmin`, etc.
- `git`: `.git/config`, `.svn/entries`
- `infra`: `/actuator`, `/.aws/credentials`
- `backups`: `backup.zip`, `dump.sql`, etc.
- `cgi`: `/cgi-bin/`

**Trickster mode** replies with deceptive responses: tarpits that drip-feed bytes to tie up bot connections, fake `.env` files with realistic-looking credentials, fake directory listings, and infinite data streams.

```yaml
- type: "HoneyPot"
  options:
    cache-ttl: 24h
    cleanup-interval: 1h
    max-cache-size: 100000
    defaults:
      - "git"
      - "infra"
      - "backups"
      - "cgi"
    paths:
      - "/my-custom-trap"
    response:
      trickster-mode: true
      max-slow-tricks: 10          # max concurrent tarpit connections
      # If trickster-mode is false:
      # status-code: 403
      # body: "Forbidden"
```

---

### CircuitBreaker

Stops sending requests to a failing backend after consecutive 5xx or timeout failures. Enters half-open state after the recovery time, where it probes with a few requests before fully closing.

```yaml
- type: "CircuitBreaker"
  options:
    failure-threshold: 5
    recovery-time: 30s
    half-open-success-threshold: 3
```

---

### Redirect

Redirects requests to a different target.

- **Internal**: proxies the request transparently (the client doesn't know).
- **External**: sends an HTTP 3xx redirect response.

```yaml
- type: "Redirect"
  options:
    mode: "external"               # "internal" or "external"
    status-code: 302               # external mode only (301, 302, 307, 308)
    target: "http://192.168.1.100:8080"
    drop-path: true
    drop-query: true
```

---

### BodySizeLimit

Limits request body size to protect against oversized uploads.

```yaml
- type: "BodySizeLimit"
  options:
    max-size: 100m                 # supports k, m, g
```

---

### Timeout

Sets the maximum time allowed for the entire request to complete.

```yaml
- type: "Timeout"
  options:
    request-timeout: 30s
```

---

## Full Example

See [`config-example.config`](../config-example.config) for a complete configuration file with all middleware types.

