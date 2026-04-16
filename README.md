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
  <b>A reverse proxy built for your home lab that isn't afraid of the open internet.</b>
</p>

---

> **`v0.1.0` — early alpha.** Things will break. Things will change. But it works, and it's already running on a Pi exposed to the internet. If you find bugs, open an issue.

## Why Torii?

You're running services at home. Jellyfin, Immich, Home Assistant, whatever. You want to expose some of them to the internet, but you don't want to think about nginx configs, certbot cron jobs, or figuring out how to block an entire country from scanning your `.env` files at 3am.

Torii is a single binary that handles all of that. Point it at your backends, set up your routes in a YAML file or through the web UI, and it takes care of TLS certificates, rate limiting, bot detection, geo-blocking, and a bunch of other things that become important the moment you open a port to the world.

I wanted observability first. I didn't want to manage an entire stack just to see what was going on with my home setup. No compose file with three or four dependencies just to watch traffic. No Grafana (it does a great job, but you still need to route your traffic to it, set up time spans, and keep the whole thing running). No ELK with the entire Beats suite on top. I just wanted to see what was happening, and the more complex the stack, the harder that gets. So the dashboard was the first thing built, before the proxy even forwarded a single request. The metrics go deep because of that.

No Docker required. No dependencies. Just the binary and a config file.

## Screenshots

|                  Dashboard                   |                    Proxy Routes                    |
|:--------------------------------------------:|:--------------------------------------------------:|
| ![Dashboard](docs/screenshots/dashboard.png) | ![Proxy Routes](docs/screenshots/proxy-routes.png) |

|                    System Requests                    |                 System Health                 |
|:-----------------------------------------------------:|:---------------------------------------------:|
| ![Create Proxy](docs/screenshots/system-requests.png) | ![System](docs/screenshots/system-health.png) |

## What You Get

**Routing:** HTTP/HTTPS listeners with virtual-host routing (route by domain), path-based routing with wildcards, per-path backend overrides, and path stripping. The usual stuff, but configured in one place, either in the YAML file or through the web UI.

**Automatic TLS:** Let's Encrypt via DNS-01 challenges. No port 80 required, no certbot, no cron. Certificates renew automatically in the background. Per-domain SNI so each hostname gets its own cert.

**Hierarchical metrics:** every Metrics middleware emits data that bubbles up through a tree: global -> route -> path. Each node aggregates its own traffic plus everything beneath it. The dashboard lets you switch between them through a single dropdown, drilling from the full system down to a specific route or path, all in one place. Other proxies don't really do this without a separate stack. Request logs and error logs update live via SSE, served straight from the binary.

**A real web UI:** built with HTMX, fast and no build pipeline needed. Dashboard with live metrics (requests/sec, latency, error rates, response codes), a proxy route viewer, a server creation wizard, system health monitoring, ACME/TLS management, and API key management. Everything configurable here or in the YAML, whichever you prefer.

**Bot defense:** user-agent blocking with built-in pattern lists for scanners, scrapers, AI crawlers, and more. A honeypot system that traps bots hitting paths like `.git/config` or `wp-login.php` and blocks their IP. Optional trickster mode that wastes their time with tarpits, fake credential files, and infinite data streams. GeoIP blocking by country or continent.

**Rate limiting:** token-bucket rate limiter, global or per-client IP. Returns proper `429` with `Retry-After`.

**Header validation:** set, strip, or require specific headers. Need a shared secret between services? `cmp-headers-req` rejects anything that doesn't match. Values can be loaded from files or environment variables at startup.

**Circuit breaker:** if your backend starts returning 5xx errors, Torii stops sending it traffic until it recovers. Your users see a clean error instead of a cascade of failures.

**API keys:** scoped API keys for external access. Currently supports the `read_stats` scope, which is useful for surfacing Torii metrics in tools like [Homepage](https://gethomepage.dev) without putting passwords in your config file.

**14 middlewares total:** all composable, all configurable per-route or per-path. See the [full list](docs/configuration.md#middleware-reference).

**SQLite storage:** single-file database, WAL mode, embedded migrations. Stores users, sessions, ACME certs, and system config.

**Debug mode:** pass `--debug` and Torii spins up stub servers for all your configured backends. Useful for testing your config without running the actual services.

## Quick Start

```bash
# Build
go build -o torii ./cmd/torii

# Run
./torii -config config.yaml
```

On first launch, open `http://127.0.0.1:27000/ui` to set your admin password.

### Minimal config

```yaml
log:
  log-level: INFO

api-server:
  host: 127.0.0.1
  port: 27000

net-config:
  http:
    - port: 80
      bind: 3
      default:
        backend: http://192.168.1.50:8080
        middlewares:
          - type: "RequestId"
          - type: "RequestLog"
          - type: "Metrics"
```

### With TLS and route-level security

```yaml
net-config:
  http:
    - port: 443
      bind: 3
      tls:
        use-acme: true
      routes:
        - host: jellyfin.home.example.com
          target:
            backend: http://192.168.1.100:8096
            middlewares:
              - type: "RequestId"
              - type: "RequestLog"
              - type: "Metrics"
              - type: "UserAgentBlocker"
                options:
                  block-empty-ua: true
                  block-defaults: ["scanners", "recon", "ai-crawlers"]
                  allow-defaults: ["search-engines"]
              - type: "HoneyPot"
                options:
                  defaults: ["git", "infra", "backups", "cgi"]
                  response:
                    trickster-mode: true
                    max-slow-tricks: 10
              - type: "RateLimiter"
                options:
                  mode: "per-client"
                  limiter-req:
                    rate-per-second: 10
                    burst: 20
      default:
        backend: http://192.168.1.50:8080
```

Full configuration reference: [docs/configuration.md](docs/configuration.md)

## Architecture

```
                    ┌─────────────────────────────────────────┐
                    │              torii                       │
  Incoming          │                                         │
  Request ─────────►│  Global Middlewares                      │
                    │    ▼                                     │
                    │  Host Dispatcher ──► Route Middlewares    │
                    │    ▼                                     │
                    │  Path Dispatcher ──► Path Middlewares     │
                    │    ▼                                     │
                    │  Reverse Proxy ──────────────────► Backend│
                    └─────────────────────────────────────────┘

  Management        ┌─────────────────────────────────────────┐
  (localhost)       │  API Server (:27000)                     │
                    │    ├── /ui          Web dashboard        │
                    │    ├── /api/v1      REST API             │
                    │    └── SSE stream   Real-time metrics    │
                    │                                         │
                    │  SQLite (torii.db)                       │
                    └─────────────────────────────────────────┘
```

## Installation

### Binary

```bash
go build -o torii ./cmd/torii
./torii -config config.yaml
```

### Docker

```bash
docker pull ghcr.io/nunoOliveiraqwe/torii:latest
```

Since Torii is a reverse proxy, `--network host` is recommended so it can bind directly to host interfaces and ports without Docker's network layer getting in the way.

```bash
docker run -d \
  --network host \
  -v torii-data:/data \
  -v /path/to/config.yaml:/etc/torii/config.yaml \
  ghcr.io/nunoOliveiraqwe/torii:latest
```

**Volumes:**
| Mount | Purpose |
|---|---|
| `torii-data:/data` | SQLite database (`torii.db` + WAL/SHM). Named volume so data persists across container restarts. |
| `/path/to/config.yaml:/etc/torii/config.yaml` | Your configuration file. Mounted read-write because the web UI can write proxy route changes back to the config. |

#### Building from source

```bash
# Multi-stage (builds inside Docker, outputs a static binary on scratch)
docker build -f docker/Multistage.Dockerfile -t torii .

# Single-stage (for CI — copy a pre-built binary into the image)
# Build the binary first with CGO and static linking:
#   CGO_ENABLED=1 go build -ldflags="-s -w -linkmode external -extldflags '-static'" -o torii ./cmd/torii
cp torii docker/
docker build -f docker/Dockerfile -t torii docker/
```

> **Note:** Torii uses `mattn/go-sqlite3` which requires CGO. The binary must be built with `CGO_ENABLED=1` and statically linked for the `scratch` base image to work.

### Debian / Ubuntu

Download the `.deb` from the [releases page](https://github.com/nunoOliveiraqwe/torii/releases) and install:

```bash
dpkg -i torii_<version>_amd64.deb
```

## Documentation

- [Configuration & Middleware Reference](docs/configuration.md)
- [Management API](docs/api.md)

## TODO

Keeping track of what's done and what's next. This is not a roadmap, just my personal scratchpad for the project.


### Needs work
- [x] IP block lists: middleware is registered, filtering logic is stubbed out
- [ ] Create HTTP Proxy UI: works, but the UX needs another pass
- [ ] ACME UI: need a delete button for individual certificates, the reset button placement is bad
- [ ] Proxy Routes UI: host names should be clickable links that open in a new tab
- [ ] Server timeouts: review the defaults for the Create HTTP Proxy wizard

### Up next
- [ ] Config persistence: proxy routes created or deleted via the UI are memory-only — they need to be written back to the config file so they survive restarts - not sure if I will. maybe with an overridable flag, because if I want to expose the proxy API to the internet for dynamic route management, I don't want those changes written to disk, and if I want to manage the config through the file, I don't want the UI overwriting it. Maybe a `persist-ui-changes` flag that defaults to `false` and can be set per-route or globally. Also maybe don't even allow proxy route changes to be created. If the session is hijacked, then not much can be done if no config can be done. Ask for password when creating a proxy!!!!!
- [ ] Wildcard host matching: `VirtualHostDispatcher` uses exact map lookup, no support for `*.home.example.com`. Replace the `map[string]http.Handler` in `VirtualHostDispatcher` with a reversed-label trie so wildcard routes match with DNS-style semantics (one label only, most-specific wins). Priority chain: exact match → longest wildcard match → `default` → 502. Normalize to lowercase, strip trailing dots. Watch out for ACME domain collection (preserve `*.example.com` for wildcard cert issuance) and SNI matching in TLS config.
- [ ] CPU usage in system activity: add CPU usage metrics to the system health/activity dashboard alongside the existing memory and other system stats.
- [ ] Terminating middleware support in UI: update the Create/Edit Proxy UI so that when a terminating middleware (e.g., Redirect) is present in the chain, the backend field becomes optional. Currently the UI always requires a backend.
- [ ] Route editing in UI: add edit support for existing proxy routes through the web UI (there is already a comment/placeholder for this in the codebase). Users should be able to modify host, backend, middlewares, and path rules for an existing route without deleting and recreating it.
- [ ] TCP proxying: config schema is there, implementation is not
- [x] Make `RequestId`, `RequestLog`, and `Metrics` default on all endpoints (too easy to forget and then the dashboard shows nothing)
- [x] Blocked IP observability: surface blocked IPs (from honeypot, UA blocker, country block) in the UI with timestamps and metadata, probably as a rolling log
- [ ] API keys: Homepage integration endpoint, `read_config` and `write_config` scopes (may be dropped depending on how useful they turn out to be)
- [ ] UA fingerprint rotation detection: bots that rotate user agents mid-scan are easy to spot — a real client doesn't switch from macOS to Linux to Windows between requests. Track UA consistency per IP and flag or block IPs whose OS/browser family changes unnaturally fast. I encountered this with a bot that rotated through 20+ UAs in a single scan, hitting 100+ endpoints in minutes. The honeypot caught it, but this would be another layer of defense against UA rotation.
- [ ] Coraza WAF integration: [Coraza](https://coraza.io/) is a full-featured open-source WAF (OWASP CRS compatible). Add it as a middleware so routes can opt into proper WAF rules alongside the existing bot defense. There's overlap with what the honeypot and UA blocker already do, but Coraza covers a much wider surface (SQLi, XSS, protocol violations, etc.).
- [x] AbuseIPDB middleware: check client IPs against [AbuseIPDB](https://www.abuseipdb.com/) and block or flag IPs with a high abuse confidence score. Optionally report blocked IPs back (honeypot hits, rate-limit violations, etc.) so the community benefits too.


### Known Bugs
- [x] **ACME port leak on startup:** if a route has `use-acme: true` but ACME is not configured, the server fails to start. Starting the proxy manually afterwards returns an ACME error, but the port is already bound from the first attempt. A second manual start then fails with a "bind: address already in use" error. The listener from the failed first start is never closed.
- [ ] **ACME config not loadable from config file:** there is no way to specify ACME configuration (provider, credentials, etc.) in the YAML config file. On first start with `use-acme: true`, it should be possible to seed the ACME configuration from the config file instead of requiring it to be set up through the UI first.

### Maybe
- [ ] Proxy-level authentication: login pages so the proxy handles auth before forwarding to backends
- [ ] Dedicated tar pitting middleware (separate from the honeypot trickster mode)
- [ ] Login endpoint rate limiting

## Requirements

- Go 1.25+

## License

[Apache License 2.0](LICENSE)