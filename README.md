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

> **`v0.0.1` — early alpha.** Things will break. Things will change. But it works, and it's already running on a Pi exposed to the internet. If you find bugs, open an issue.

## Why Torii?

You're running services at home. Jellyfin, Immich, Home Assistant, whatever. You want to expose some of them to the internet, but you don't want to think about nginx configs, certbot cron jobs, or figuring out how to block the entire country of China from scanning your `.env` files at 3am.

Torii is a single binary that handles all of that. Point it at your backends, set up your routes in a YAML file (or through the web UI), and it takes care of TLS certificates, rate limiting, bot detection, geo-blocking, and a bunch of other things that become important the moment you open a port to the world.

No Docker. No dependencies. Just the binary and a config file.

## Screenshots

|                  Dashboard                   |                    Proxy Routes                    |
|:--------------------------------------------:|:--------------------------------------------------:|
| ![Dashboard](docs/screenshots/dashboard.png) | ![Proxy Routes](docs/screenshots/proxy-routes.png) |

|                    System Requests                    |                 System Health                 |
|:-----------------------------------------------------:|:---------------------------------------------:|
| ![Create Proxy](docs/screenshots/system-requests.png) | ![System](docs/screenshots/system-health.png) |

## What You Get

**Routing** — HTTP/HTTPS listeners with virtual-host routing (route by domain), path-based routing with wildcards, per-path backend overrides, and path stripping. The usual stuff, but configured in one place.

**Automatic TLS** — Let's Encrypt via DNS-01 challenges. No port 80 required, no certbot, no cron. Certificates renew automatically in the background. Per-domain SNI so each hostname gets its own cert.

**A real web UI** — Dashboard with live metrics (requests/sec, latency, error rates, response codes), a proxy route viewer, a server creation wizard, system health monitoring, ACME/TLS management, and API key management. Not an afterthought.

**Bot defense** — User-agent blocking with built-in pattern lists for scanners, scrapers, AI crawlers, and more. A honeypot system that traps bots hitting paths like `.git/config` or `wp-login.php` and blocks their IP. Optional trickster mode that wastes their time with tarpits, fake credential files, and infinite data streams. GeoIP blocking by country or continent.

**Rate limiting** — Token-bucket rate limiter, global or per-client IP. Returns proper `429` with `Retry-After`.

**Header validation** — Set, strip, or require specific headers. Need a shared secret between services? `cmp-headers-req` rejects anything that doesn't match. Values can be loaded from files or environment variables at startup.

**Circuit breaker** — If your backend starts returning 5xx errors, Torii stops sending it traffic until it recovers. Your users see a clean error instead of a cascade of failures.

**14 middlewares total** — All composable, all configurable per-route or per-path. See the [full list](docs/configuration.md#middleware-reference).

**SQLite storage** — Single-file database, WAL mode, embedded migrations. Stores users, sessions, ACME certs, and system config.

**Debug mode** — Pass `--debug` and Torii spins up stub servers for all your configured backends. Useful for testing your config without running the actual services.

## Quick Start

```bash
# Build
go build -o torii ./cmd/torii

# Run
./torii --config config.yaml
```

On first launch, open `http://127.0.0.1:27000/ui` to set your admin password.

### Minimal config

```yaml
log:
  logLevel: INFO

apiServer:
  host: 127.0.0.1
  port: 27000

netConfig:
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
netConfig:
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

## Documentation

- [Configuration & Middleware Reference](docs/configuration.md)
- [Management API](docs/api.md)

## TODO

Keeping track of what's done and what's next. This is a side project so no promises on timelines.

### Needs work
- [ ] IP block lists — middleware is registered, filtering logic is stubbed out
- [ ] Create HTTP Proxy UI — works, but the UX needs another pass
- [ ] ACME UI — need a delete button for individual certificates, the reset button placement is bad
- [ ] Proxy Routes UI — host names should be clickable links that open in a new tab
- [ ] Server timeouts — review the defaults for the Create HTTP Proxy wizard

### Up next
- [ ] TCP proxying — config schema is there, implementation is not
- [ ] Make `RequestId`, `RequestLog`, and `Metrics` default on all endpoints (too easy to forget and then the dashboard shows nothing)
- [ ] Blocked IP observability — surface blocked IPs (from honeypot, UA blocker, country block) in the UI with timestamps and metadata, probably as a rolling log
- [ ] GitHub Actions CI
- [ ] Docker image
- [ ] `.deb` package — want to run this on a Pi and let the bots come

### Maybe
- [ ] Proxy-level authentication — login pages so the proxy handles auth before forwarding to backends
- [ ] Dedicated tar pitting middleware (separate from the honeypot trickster mode)
- [ ] Login endpoint rate limiting

## Requirements

- Go 1.25+

## License

[Apache License 2.0](LICENSE)
