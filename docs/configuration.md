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
  - [IpFilter](#ipfilter)
  - [CountryBlock](#countryblock)
  - [UserAgentBlocker](#useragentblocker)
  - [HoneyPot](#honeypot)
  - [CircuitBreaker](#circuitbreaker)
  - [Redirect](#redirect)
  - [BodySizeLimit](#bodysizelimit)
  - [Timeout](#timeout)
  - [Compression](#compression)
  - [BasicAuth](#basicauth)
  - [CorazaWaf](#corazawaf)
  - [StaticResponse](#staticresponse)
  - [StaticFileServer](#staticfileserver)
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
| `api-server.access-log-path` | `string` | | Optional path to write API access logs. |
| `api-server.idle-timeout` | `int` | `60` | Idle connection timeout in seconds. |
| `api-server.read-timeout` | `int` | `60` | Read timeout in seconds. |
| `api-server.write-timeout` | `int` | `60` | Write timeout in seconds. |

## ACME (`acme`)

Seeds ACME (Let's Encrypt) configuration into the database on first startup. Once stored in the DB, this section is ignored — the DB becomes the source of truth (editable via the dashboard).

| Key | Type | Default | Description |
|---|---|---|---|
| `acme.email` | `string` | | Contact email for the ACME account. **Required.** |
| `acme.dns-provider` | `string` | | DNS provider for DNS-01 challenges. Currently supported: `cloudflare`. **Required.** |
| `acme.credentials` | `map[string]string` | | Provider-specific credential key-value pairs. Supports `$env:VAR` and `$file:/path` syntax. |
| `acme.ca-url` | `string` | *(Let's Encrypt production)* | Custom ACME CA directory URL (e.g. staging). |
| `acme.renewal-check-interval` | `duration` | `12h` | How often to check/renew certificates. Minimum `1h`. |
| `acme.enabled` | `bool` | `false` | Whether ACME is active. Set `false` to seed config without activating. |
| `acme.domains` | `[]string` | | Explicit list of domains to obtain certificates for. Supports wildcards (e.g. `*.example.com`). When set, only these domains get certs. When empty, a cert is requested for each route host individually. |

**Example — wildcard cert for all subdomains:**
```yaml
acme:
  email: admin@example.com
  dns-provider: cloudflare
  credentials:
    api_token: "$env:CF_API_TOKEN"
  renewal-check-interval: 12h
  enabled: true
  domains:
    - "*.example.com"       # one cert covers app.example.com, api.example.com, etc.
    - "example.com"         # bare domain is NOT covered by the wildcard
```

### Wildcard Certificates

To use a wildcard certificate, list it in `acme.domains`. A single `*.example.com` cert covers all single-level subdomains (`app.example.com`, `api.example.com`, etc.) — no matter how many routes you add. During TLS handshake, Torii automatically falls back to a matching wildcard cert when no exact cert exists for the requested server name.

> **Note:** The bare domain (`example.com`) is **not** covered by `*.example.com` and must be listed separately if needed.

### Auto-discovery (no `acme.domains`)

When `acme.domains` is omitted, Torii collects domains from the route hosts on ACME-enabled listeners and requests a separate cert for each one. This is simpler for small setups but means every new route triggers a new certificate request.

**Use `acme.domains` with a wildcard when** you have multiple subdomains under the same parent — one cert covers them all, and adding new routes doesn't require new certs.

## HTTP Listener (`net-config.http[]`)

| Key | Type | Description |
|---|---|---|
| `port` | `int` | Port to listen on. |
| `bind` | `int` | `1` = IPv4, `2` = IPv6, `3` = both. |
| `interface` | `string` | Network interface name to bind to (e.g. `en0`). Defaults to loopback. |
| `disable-http2` | `bool` | Set `true` to disable HTTP/2 (H2C for plain HTTP). |
| `read-timeout` | `duration` | Server read timeout. |
| `read-header-timeout` | `duration` | Server read-header timeout. |
| `write-timeout` | `duration` | Server write timeout. |
| `idle-timeout` | `duration` | Server idle timeout. |
| `tls.use-acme` | `bool` | Enable automatic TLS via ACME DNS-01. |
| `tls.cert` | `string` | Path to TLS certificate (manual TLS). |
| `tls.key` | `string` | Path to TLS private key (manual TLS). |
| `routes[].host` | `string` | Hostname to match (virtual hosting). Supports wildcards (e.g. `*.home.example.com`). |
| `routes[].target.backend` | `string` | Backend URL to proxy to. |
| `routes[].target.middlewares` | `list` | Middleware chain for this route. |
| `routes[].target.paths[]` | `list` | Path-specific rules with their own middleware chains. |
| `routes[].target.disable-default-middlewares` | `bool` | If `true`, `RequestId`/`RequestLog`/`Metrics` are not auto-prepended for this route. |
| `default.backend` | `string` | Catch-all backend when no host matches. |
| `default.middlewares` | `list` | Middleware chain for the default route. |
| `default.disable-default-middlewares` | `bool` | If `true`, `RequestId`/`RequestLog`/`Metrics` are not auto-prepended for the default route. |

The `backend` field can also be an object with additional options:

```yaml
backend:
  address: http://192.168.1.27:2283
  replace-host-header: true    # Replace the Host header with the backend's host
```

## Path Rules (`paths[]`)

| Key | Type | Description |
|---|---|---|
| `pattern` | `string` | URL pattern to match (supports `*` wildcards). |
| `backend` | `string` | Optional backend override for this path. |
| `strip-prefix` | `bool` | Strip the matched path prefix before proxying (e.g. `/jellyfin/foo` → `/foo`). |
| `drop-query` | `bool` | Strip query parameters before proxying. |
| `disable-default-middlewares` | `bool` | If `true`, `RequestId`/`RequestLog`/`Metrics` are not auto-prepended for this path. |
| `middlewares` | `list` | Middleware chain for this path. |

## Global Middlewares (`net-config.global`)

| Key | Type | Description |
|---|---|---|
| `disable-default-middlewares` | `bool` | If `true`, `RequestId`/`RequestLog`/`Metrics` are not auto-prepended globally. |
| `middlewares` | `list` | Middleware chain applied to all requests on all listeners. |

## Session (`session`)

| Key | Type | Default | Description |
|---|---|---|---|
| `lifetime` | `duration` | `16h` | Maximum session lifetime. |
| `idle-timeout` | `duration` | `60m` | Session idle timeout. |
| `cleanup-interval` | `duration` | `1h` | Expired session cleanup interval. |
| `cookie-domain` | `string` | | Cookie domain (empty = auto from request host). |
| `cookie-secure` | `bool` | `true` | Set the `Secure` flag on session cookies. |
| `cookie-http-only` | `bool` | `true` | Set the `HttpOnly` flag on session cookies. |
| `cookie-same-site` | `string` | `lax` | `SameSite` cookie attribute: `strict`, `lax`, or `none`. |

## CLI Flags

```
--config <path>       Path to the YAML configuration file.
--debug               Start stub servers for all configured backends.
--log-level <level>   Override the log level from config.
--read-only           Prevent UI mutations from modifying the config file or proxy state.
```

---

## Middleware Reference

Middlewares run in a fixed layer order:

```
Global middlewares → Route / Default middlewares → Path middlewares → Reverse Proxy
```

Within each layer, they run in the order you list them.

By default, `RequestId`, `RequestLog`, and `Metrics` are automatically prepended to every middleware chain (unless already present or disabled with `disable-default-middlewares`).

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

Structured request logging. Optionally writes to a file in addition to stdout.

```yaml
- type: "RequestLog"
  options:
    request-log-path: access.log  # optional file path for access logs
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
    allowed-origins:
      - "*"
    allowed-methods:
      - "GET"
      - "POST"
      - "PUT"
      - "DELETE"
      - "PATCH"
      - "OPTIONS"
    allowed-headers:
      - "Content-Type"
      - "Authorization"
    expose-headers:
      - "X-Request-Id"
    allow-credentials: false
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

### IpFilter

IP-based allow/block lists with CIDR support. Optionally fetches block lists from [AbuseIPDB](https://www.abuseipdb.com/).

The allow list always takes priority — IPs matching it are never blocked. When AbuseIPDB is configured, it replaces the static block list.

```yaml
- type: "IpFilter"
  options:
    allow:                               # always allowed (overrides block list)
      - 192.168.1.0/24
      - 10.0.0.1
      - 2001:db8::/32
    block:                               # denied (ignored when AbuseIPDB is configured)
      - 94.0.0.1/24
      - 182.78.86.62
    # --- AbuseIPDB (optional) ---
    # abuseipdb-api-key: "$env:ABUSEIPDB_API_KEY"     # supports $env:VAR, $file:/path
    # abuseipdb-confidence-minimum: 90                  # min abuse score (1-100)
    # abuseipdb-refresh-interval: "24h"                 # how often to re-fetch
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
    lan-allow-list:
      - 192.168.1.1/24
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

Redirects requests to a different target. Terminates the middleware chain — no backend is needed.

- **Internal**: proxies the request transparently (the client doesn't know).
- **External**: sends an HTTP 3xx redirect response to a fixed target.
- **External-scheme-only**: sends an HTTP 3xx redirect preserving the incoming host, only changing the scheme. Ideal for HTTP→HTTPS across multiple subdomains.

```yaml
- type: "Redirect"
  options:
    mode: "external"               # "internal", "external", or "external-scheme-only"
    status-code: 302               # external modes only (301, 302, 307, 308)
    target: "http://192.168.1.100:8080"  # URL for internal/external; "https" for external-scheme-only
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

### Compression

Compresses response bodies to reduce bandwidth usage.

```yaml
- type: "Compression"
  options:
    type: "gzip"                   # "gzip" or "zlib"
    level: 9                       # 1 (fastest) to 9 (best compression)
```

---

### BasicAuth

HTTP Basic Authentication with Argon2id password hashes. Generate hashes with the included `argon2` CLI tool (`go run ./cmd/argon2`).

```yaml
- type: "BasicAuth"
  options:
    realm: "Internal"
    credentials:
      admin: "$argon2id$v=19$m=65536,t=3,p=4$abc123$hashedpassword"
```

---

### CorazaWaf

Web Application Firewall powered by [Coraza](https://coraza.io/) and the OWASP Core Rule Set (CRS). Inspects requests for SQL injection, XSS, protocol violations, and more.

```yaml
- type: "CorazaWaf"
  options:
    paranoia-level: 1              # CRS paranoia level 1-4 (higher = stricter, more false positives)
    inbound-threshold: 5           # anomaly score to trigger block (lower = stricter)
    outbound-threshold: 4          # outbound anomaly score threshold
    mode: "detect"                 # "detect" (log only) or "block" (reject)
    inspect-request-body: false    # buffer and inspect request bodies (adds latency)
    inspect-response-body: false   # inspect response bodies (detect mode only)
    exclusions:                    # CRS rule IDs to suppress (for known false positives)
      - "920170"
      - "941100"
```

---

### StaticResponse

Returns a fixed HTTP response without forwarding to any backend. Useful for maintenance pages, health stubs, or canned API responses. Terminates the middleware chain — no backend is needed.

```yaml
- type: "StaticResponse"
  options:
    status-code: 503
    response-body: '{"status":"maintenance"}'
    content-type: "application/json"
    headers:
      Retry-After: "3600"
      Cache-Control: "no-store"
```

---

### StaticFileServer

Serves static files from a local directory. Supports SPA fallback, index files, and dotfile protection. Terminates the middleware chain — no backend is needed.

```yaml
- type: "StaticFileServer"
  options:
    root: "/var/www/html"          # path to the directory to serve
    index-file: "index.html"       # file to serve for directory requests
    spa: false                     # SPA mode: unmatched paths fall back to index
    allow-dot-files: false         # allow serving hidden files (.env, .git, etc.)
```

---

## Full Example

See [`config-example.yaml`](../config-example.yaml) for a complete configuration file with all middleware types.
