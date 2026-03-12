<p align="center">
  <h1 align="center">VyanaWatch</h1>
  <p align="center">Lightweight, self-hosted uptime monitoring tool built in Go</p>
  <p align="center">
    <img src="https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white" alt="Go">
    <img src="https://img.shields.io/badge/License-MIT-green" alt="License">
    <img src="https://img.shields.io/badge/Docker-Ready-2496ED?logo=docker&logoColor=white" alt="Docker">
  </p>
</p>

---

VyanaWatch is a **lightweight, self-hosted uptime monitoring tool** — an alternative to Uptime Kuma and Gatus. It ships as a single binary with an embedded web dashboard, requires zero external dependencies, and supports SQLite or PostgreSQL for storage.

## Features

- **5 Monitor Types** — HTTP(s), ICMP Ping, TCP Port, DNS, Push/Heartbeat
- **Real-time Dashboard** — Dark-themed SPA with live SSE updates, response-time charts, incident history
- **Public Status Pages** — Shareable, customizable status pages with auto-refresh
- **4 Alert Channels** — Email (SMTP with HTML templates), Telegram, Discord, Custom Webhooks
- **REST API** — Full CRUD, history, summary, pause/resume, SSE events
- **Single Binary** — Pure Go, no CGO, no external dependencies
- **Docker Ready** — Multi-stage Dockerfile, docker-compose with optional PostgreSQL
- **Configurable** — YAML config + environment variable overrides + hot-reload

## Quick Start

### Binary

```bash
# Clone and build
git clone https://github.com/vyanawatch/vyanawatch.git
cd vyanawatch
go build -o vyanawatch .

# Copy and edit config
cp config.yaml.example config.yaml

# Run
./vyanawatch
```

Open **http://localhost:8080** in your browser.

### Docker

```bash
# Copy config
cp config.yaml.example config.yaml

# Start with SQLite (default)
docker compose up -d

# Or start with PostgreSQL
docker compose --profile postgres up -d
```

### Docker (pre-built)

```bash
docker run -d \
  --name vyanawatch \
  --cap-add NET_RAW \
  -p 8080:8080 \
  -v vyanawatch-data:/app/data \
  -v ./config.yaml:/app/config.yaml:ro \
  vyanawatch:latest
```

## Configuration

Copy `config.yaml.example` to `config.yaml`. All values can be overridden with environment variables prefixed with `VYANAWATCH_`.

```yaml
server:
  port: 8080
  host: "0.0.0.0"
  log_level: "info"    # debug, info, warn, error

database:
  driver: "sqlite"     # "sqlite" or "postgres"
  dsn: "./data/vyanawatch.db"

alerting:
  email:
    enabled: false
    host: "smtp.gmail.com"
    port: 587
    username: ""
    password: ""       # or VYANAWATCH_SMTP_PASSWORD
    from: "VyanaWatch <noreply@example.com>"
    to: "admin@example.com"

  telegram:
    enabled: false
    token: ""          # or VYANAWATCH_TELEGRAM_TOKEN
    chat_id: ""

  discord:
    enabled: false
    webhook_url: ""    # or VYANAWATCH_DISCORD_WEBHOOK_URL

  webhook:
    enabled: false
    url: ""
    method: "POST"
    headers: '{"Authorization": "Bearer token"}'

auth:
  enabled: false
  username: "admin"
  password: ""         # or VYANAWATCH_AUTH_PASSWORD
```

### Environment Variables

| Variable | Description |
|---|---|
| `VYANAWATCH_SERVER_PORT` | HTTP server port |
| `VYANAWATCH_DB_DRIVER` | `sqlite` or `postgres` |
| `VYANAWATCH_DB_DSN` | Database connection string |
| `VYANAWATCH_SMTP_PASSWORD` | SMTP password |
| `VYANAWATCH_TELEGRAM_TOKEN` | Telegram bot token |
| `VYANAWATCH_DISCORD_WEBHOOK_URL` | Discord webhook URL |
| `VYANAWATCH_AUTH_PASSWORD` | Admin password |

## Monitor Types

| Type | Description | Key Fields |
|---|---|---|
| **HTTP(s)** | Checks URL status code, keywords, SSL expiry | `url`, `method`, `expected_status_code`, `keyword_check`, `ssl_check` |
| **Ping** | ICMP ping with packet loss detection | `hostname` |
| **TCP** | TCP port connectivity | `hostname`, `port` |
| **DNS** | DNS record resolution (A, AAAA, CNAME, MX, TXT, NS) | `hostname`, `dns_type` |
| **Push** | Passive heartbeat — your service pings VyanaWatch | Auto-generated `push_token` |

## API Reference

Base URL: `http://localhost:8080/api/v1`

### Monitors

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/monitors` | List all monitors with uptime stats |
| `POST` | `/monitors` | Create a monitor |
| `GET` | `/monitors/{id}` | Get monitor details |
| `PUT` | `/monitors/{id}` | Update a monitor |
| `DELETE` | `/monitors/{id}` | Delete a monitor |
| `GET` | `/monitors/{id}/history` | Response time history (`?hours=24&limit=500`) |
| `POST` | `/monitors/{id}/pause` | Pause monitoring |
| `POST` | `/monitors/{id}/resume` | Resume monitoring |
| `GET` | `/monitors/summary` | Aggregate status counts |

### Push / Heartbeat

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/push/{token}` | Send heartbeat for a push monitor |

### Status Pages

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/status-pages` | List all status pages |
| `POST` | `/status-pages` | Create a status page |
| `GET` | `/status-pages/{id}` | Get status page |
| `PUT` | `/status-pages/{id}` | Update status page |
| `DELETE` | `/status-pages/{id}` | Delete status page |
| `POST` | `/status-pages/{id}/monitors/{monitorId}` | Add monitor to page |
| `DELETE` | `/status-pages/{id}/monitors/{monitorId}` | Remove monitor from page |

### Real-time Events

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/events` | SSE stream (events: `down`, `recovery`, `monitor_created`, `monitor_updated`, `monitor_deleted`) |

### Other

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/health` | Health check |

### Example: Create HTTP Monitor

```bash
curl -X POST http://localhost:8080/api/v1/monitors \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Google",
    "type": "http",
    "url": "https://www.google.com",
    "interval": 60,
    "timeout": 10
  }'
```

### Example: Create Push Monitor

```bash
# Create
curl -X POST http://localhost:8080/api/v1/monitors \
  -H "Content-Type: application/json" \
  -d '{"name": "Cron Job", "type": "push", "interval": 300}'

# Send heartbeat (use the push_token from response)
curl -X POST http://localhost:8080/api/v1/push/TOKEN_HERE
```

## Project Structure

```
vyanawatch/
├── main.go                # Entry point, wiring, graceful shutdown
├── config/
│   └── config.go          # Viper-based config with hot-reload
├── db/
│   ├── database.go        # GORM init, migrations (SQLite/Postgres)
│   ├── models.go          # Monitor, Heartbeat, Incident, StatusPage
│   ├── repos.go           # Repository aggregator
│   ├── monitor_repo.go    # Monitor CRUD + stats queries
│   ├── heartbeat_repo.go  # Heartbeat storage + uptime calculations
│   ├── incident_repo.go   # Incident open/resolve tracking
│   └── statuspage_repo.go # Status page management
├── monitor/
│   ├── monitor.go         # Engine: goroutine-per-monitor scheduler
│   ├── http.go            # HTTP checker (status, keywords, SSL)
│   ├── ping.go            # ICMP ping checker
│   ├── tcp.go             # TCP port checker
│   └── dns.go             # DNS resolver checker
├── alert/
│   ├── alert.go           # Dispatcher + Notifier interface
│   ├── email.go           # SMTP with HTML email template
│   ├── telegram.go        # Telegram Bot API
│   ├── discord.go         # Discord webhook with embeds
│   └── webhook.go         # Custom webhook (JSON payload)
├── api/
│   ├── routes.go          # Chi router, middleware, route definitions
│   ├── handlers.go        # Monitor CRUD handlers
│   ├── statuspage_handlers.go  # Status page handlers
│   └── sse.go             # Server-Sent Events broker
├── ui/
│   ├── embed.go           # Go embed directives
│   └── templates/
│       ├── dashboard.html # Main dashboard SPA
│       └── status.html    # Public status page
├── config.yaml.example    # Config reference
├── Dockerfile             # Multi-stage, CGO_ENABLED=0
├── docker-compose.yml     # SQLite default + Postgres profile
├── .dockerignore
└── .gitignore
```

## Tech Stack

| Component | Technology |
|---|---|
| Language | Go 1.22+ |
| Router | [Chi v5](https://github.com/go-chi/chi) |
| ORM | [GORM](https://gorm.io) |
| SQLite | [glebarez/sqlite](https://github.com/glebarez/sqlite) (pure Go, no CGO) |
| PostgreSQL | [gorm.io/driver/postgres](https://github.com/go-gorm/postgres) |
| Config | [Viper](https://github.com/spf13/viper) + fsnotify hot-reload |
| Logging | [zerolog](https://github.com/rs/zerolog) |
| ICMP Ping | [pro-bing](https://github.com/prometheus-community/pro-bing) |
| Frontend | Vanilla HTML/CSS/JS (embedded, no build step) |
| Real-time | Server-Sent Events (SSE) |

## Notes

- **ICMP Ping** requires `NET_RAW` capability (Docker: `cap_add: NET_RAW`, Linux: `sudo setcap cap_net_raw+ep ./vyanawatch`)
- **SQLite** is the default and recommended for single-node deployments
- **PostgreSQL** is supported for high-availability setups
- The dashboard is a **single-page application** embedded in the binary — no Node.js or build tools needed
- Config file changes are **hot-reloaded** automatically (no restart needed)

## License

MIT
