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
  <b>See everything hitting your server. Block what shouldn't be there.</b>
</p>

---

<p align="center">
  <img src="docs/screenshots/dashboard.png" alt="Torii Dashboard" width="800" />
</p>

<p align="center">
  <i>Live dashboard on a Raspberry Pi exposed to the internet. Every request, every blocked IP, every bot caught — in real time.</i>
</p>

---

## What is this?

A reverse proxy with a built-in security dashboard. One binary, one config file, no dependencies.

You point it at your services, expose it to the internet, and watch what happens. Torii handles routing, TLS certificates, rate limiting, and bot defense — and shows you all of it live.

## 30-second start

```bash
go build -o torii ./cmd/torii
./torii -config config.yaml --debug
```

Open `http://127.0.0.1:27000/ui`, set your password, and you're looking at live traffic. `--debug` starts stub backends so you can explore without configuring real services.

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
      default:
        backend: http://192.168.1.50:8080
```

That's it. Torii adds request logging, metrics, and request IDs automatically.

Full configuration reference → [docs/configuration.md](docs/configuration.md)

---

## What it catches

Torii is running on a Raspberry Pi 3 with ports open to the internet. Here's what a typical day looks like:

- **Scanners probing for vulnerabilities** — `.env`, `wp-login.php`, `.git/config` — caught by honeypot traps and auto-blocked
- **Known scanner user agents** — Nuclei, zgrab, scrapers, AI crawlers — matched and blocked on sight
- **IPs with known abuse history** — checked against AbuseIPDB in real time
- **Brute-force attempts** — rate limited with proper `429` + `Retry-After`, repeat offenders blocked
- **Traffic from unexpected countries** — geo-blocked before it reaches your services

Everything that gets blocked shows up in the dashboard with timestamps, IPs, and the reason it was caught.

|                    Activity Log                    |                    Under Load                    |
|:--------------------------------------------------:|:------------------------------------------------:|
| ![Activity](docs/screenshots/activity.png)         | ![Load Test](docs/screenshots/load-test.png)     |

---

## What it does

**Routing** — HTTP/HTTPS, virtual-host routing, path-based routing with wildcards, per-path backend overrides.

**Automatic TLS** — Let's Encrypt via DNS-01. No port 80 required. Per-domain SNI. Background renewal.

**Bot defense** — UA blocking (scanners, scrapers, AI crawlers), honeypot traps with optional trickster mode (tarpits, fake credentials, infinite streams), GeoIP blocking.

**Rate limiting** — Token-bucket, global or per-client IP.

**Circuit breaker** — Stops forwarding to unhealthy backends until they recover.

**AbuseIPDB integration** — Check client IPs against community abuse reports. Optionally report blocked IPs back.

**Live dashboard** — Hierarchical metrics (global → route → path), real-time request and error logs via SSE, blocked IP log.

**19 composable middlewares** — all per-route or per-path. [Full list →](docs/configuration.md#middleware-reference)

---

## Homepage integration

Torii has scoped API keys for read-only stats access. Add this to your [Homepage](https://gethomepage.dev) config:

```yaml
- Torii:
    icon: shield
    href: http://127.0.0.1:27000/ui
    widget:
      type: customapi
      url: http://127.0.0.1:27000/api/v1/proxy/metrics
      headers:
        Authorization: Bearer <your-api-key>
      mappings:
        - field: total_requests
          label: Requests
        - field: blocked_ips
          label: Blocked IPs
```

<p align="center">
  <img src="docs/screenshots/homepage.png" alt="Homepage Integration"/>
</p>

---

## Screenshots

|                  Dashboard                   |                    Proxy Routes                    |
|:--------------------------------------------:|:--------------------------------------------------:|
| ![Dashboard](docs/screenshots/dashboard.png) | ![Proxy Routes](docs/screenshots/proxy-routes.png) |

---

## Installation

### Binary

```bash
go build -o torii ./cmd/torii
./torii -config config.yaml
```

### Docker

```bash
docker run -d \
  --network host \
  -v torii-data:/data \
  -v /path/to/config.yaml:/etc/torii/config.yaml \
  ghcr.io/nunoOliveiraqwe/torii:latest
```

`--network host` is recommended so Torii can bind directly to host interfaces.

| Mount | Purpose |
|---|---|
| `torii-data:/data` | SQLite database (persists across restarts) |
| `/path/to/config.yaml` | Config file |

<details>
<summary>Building from source (Docker)</summary>

```bash
docker build -f docker/Multistage.Dockerfile -t torii .
```

Torii uses `mattn/go-sqlite3` which requires CGO. The binary must be built with `CGO_ENABLED=1`.

</details>

### Debian / Ubuntu

```bash
dpkg -i torii_<version>_amd64.deb
```

---

## Performance

Full middleware chain over HTTPS on a **Raspberry Pi 3** (4-core ARM, 906 MB RAM):

| Concurrency | Req/s | p50 | p99 |
|:-----------:|------:|----:|----:|
| 10 | **663** | 12 ms | 98 ms |
| 100 | **656** | 146 ms | 359 ms |

**Desktop** (AMD 9800X3D, 32 GB RAM):

| Concurrency | Req/s | p50 | p99 |
|:-----------:|------:|----:|----:|
| 100 | **9,540** | 9 ms | 96 ms |
| 200 | **13,114** | 12 ms | 100 ms |

---

## Documentation

- [Configuration & Middleware Reference](docs/configuration.md)
- [Management API](docs/api.md)


## TODO

Keeping track of what's done and what's next. This is not a roadmap, just my personal scratchpad for the project.


### Needs work
- [ ] Create HTTP Proxy UI: works, but the UX needs another pass
- [ ] ACME UI: need a revoke button for individual certificates (altough this might cause some 'funny' side effects), the reset button placement is bad
- [ ] Proxy Routes UI: host names should be clickable links that open in a new tab

### Up next
- [ ] The rolling logs needs to be decoupled from metrics. Right now the metrics middleware is used to track them, but this feels wrong, it's just not elegant, and is leading to ~~issues~~ quirks when using global middlewares.
- [ ] Enabled API server midlewares to be fully configurable through the config file
- [ ] TCP proxying: config schema is there, implementation is not
- [ ] UA fingerprint rotation detection: bots that rotate user agents mid-scan are easy to spot — a real client doesn't switch from macOS to Linux to Windows between requests. Track UA consistency per IP and flag or block IPs whose OS/browser family changes unnaturally fast. I encountered this with a bot that rotated through 20+ UAs in a single scan, hitting 100+ endpoints in minutes. The honeypot caught it, but this would be another layer of defense against UA rotation.
- [ ] ForwardAuth middleware: delegate auth decisions to an external service (like Traefik's ForwardAuth / nginx auth_request)
- [ ] Setting headers based on IP or other request attributes (e.g. add `X-Client-IP` $remote_addr), need to buidl a list of useful attributes that can be templated
- [ ] Bait domains, e.g. `admin.yourdomain.com`, these would be domains that have no reason to receive any traffic, so any request to them would be highly suspicious. I can potentialy have a DNS provider integration that automatically creates and removes these bait domains, so they can be rotated periodically. This would be a great way to catch bots that are targeting specific subdomains (e.g. `admin`, `dev`, `staging`) without exposing any real services on those subdomains. Bonus points that this would serve as a legitimate IP trap, where any IP hitting this would be instantly blocked, and the funny thing is, they need to connect before they can verify it's legit or not, I only need for them to connect before banning them.
- [ ] CrowdSec integration: implement Torii as a CrowdSec bouncer using the streaming mode. Long-poll from the CrowdSec LAPI, so IP checks are pure in-memory lookups with no per-request latency. CrowdSec runs as a separate daemon, parses Torii's logs, and benefits from the community blocklist. Two directions: Torii as enforcer (reads decisions) and Torii as sensor (feeds logs to CrowdSec for scenario detection).
- [ ] Detect the slick ones. Most traffic is easy to catch with the right rules, but there are always going to be some bots that fly under the radar. Implement a "suspicious activity" score based on request patterns (e.g. high request rate, odd paths, inconsistent UAs) and flag IPs that exceed a certain threshold for manual review. This would be a way to catch the bots that are just good enough to avoid the traps, but still exhibit behavior that's not typical of real users.
- [ ] Active backend health checks, periodically probe backends in the background so they're marked down before real traffic is affected, rather than relying solely on the circuit breaker's passive failure detection. Config per-backend: health check path, interval, timeout, and consecutive failure/success thresholds to transition between healthy/unhealthy. Unhealthy backends should be shown in the dashboard. Health check state and circuit breaker state should share the same backend status so they don't contradict each other.
- [ ] Internal event bus, a lightweight pub/sub backbone so middlewares and subsystems can emit and react to events (request blocked, honeypot triggered, backend went down, suspicious IP flagged, etc.) without direct coupling. This seems unavoidable as I keep seeing the need for it emerging everywhere. is the prerequisite that makes CrowdSec sensor mode, suspicious activity scoring, bait domain blocking, where the IP is shared across middleware's, UA rotation detection, and alerting/webhooks all clean to implement. Without it, each new feature needs to reach into other components directly.


### Known Bugs

### Maybe
- [ ] Proxy-level authentication: login pages so the proxy handles auth before forwarding to backends
- [ ] Dedicated tar pitting middleware (separate from the honeypot trickster mode)
- [ ] Anubis integration (I really want this, but need to figure out how to best integrate it, given anubis has a pretty large config surface)
- 

## Requirements

- Go 1.26+

## License

[GNU Affero General Public License v3.0](LICENSE)
