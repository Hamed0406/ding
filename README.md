# Ding

A fast network scanner that answers: who is on your network, what are they, and what ports are open. Inspired by [Fing](https://www.fing.com/).

**Architecture:** Rust handles low-level scanning (ARP, ICMP, TCP). Go handles orchestration, change detection, alerting, storage, and serves the web UI. Docker ships everything — no host dependencies required.

---

## Requirements

- Docker + Docker Compose (Linux host only — ARP scanning requires `AF_PACKET` raw sockets)
- `NET_RAW` / `NET_ADMIN` capabilities (granted automatically via compose)

---

## Quick start

```bash
git clone <repo>
cd ding
docker compose build
docker compose up
```

Then open **http://\<host-ip\>:8081** in a browser or on your Android device.

The interface and subnet are auto-detected. To override, set `DING_INTERFACE` and `DING_SUBNET` in `docker-compose.yml`.

Find your LAN interface and subnet:
```bash
ip -4 addr show
```

---

## Web UI

The UI is a React PWA bundled into the Go binary. It works in any browser and is installable on Android from Chrome ("Add to Home Screen").

| Feature | Description |
|---|---|
| **Scan now** | Trigger an on-demand scan; results appear in real time via SSE |
| **Device grid** | All discovered devices — IP, MAC, open ports, alive status |
| **Changes feed** | NEW / GONE / PORTS changes highlighted with colour coding |
| **Auto-detect** | Interface and subnet shown in the header |
| **PWA** | Installable on Android home screen, works offline (cached shell) |

---

## Configuration

All options are set via environment variables.

| Variable | Default | Description |
|---|---|---|
| `DING_INTERFACE` | _(auto)_ | Network interface to scan on |
| `DING_SUBNET` | _(auto)_ | Target subnet in CIDR notation |
| `DING_PORTS` | `22,80,443,8080,8443` | TCP ports to probe on each host |
| `DING_TIMEOUT_MS` | `500` | Per-host timeout in milliseconds |
| `DING_DATA_PATH` | `/data/ding.json` | Where scan history is stored |
| `DING_HTTP_ADDR` | `:8081` | Address the web server listens on |
| `DING_SCAN_INTERVAL` | `60s` | Auto-scan interval (`""` = on-demand only) |
| `DING_SCANNER_BIN` | `/usr/local/bin/scanner` | Path to Rust scanner binary |
| `DING_TELEGRAM_TOKEN` | _(empty)_ | Telegram bot token for alerts |
| `DING_TELEGRAM_CHAT_ID` | _(empty)_ | Telegram chat or channel ID |

---

## REST API

The Go server exposes a small API used by the UI. You can also call it directly.

```bash
# Current status (interface, subnet, last scan time)
curl http://localhost:8081/api/status

# Latest device list
curl http://localhost:8081/api/devices

# Scan history (last 20 runs)
curl http://localhost:8081/api/history

# Trigger a scan (returns 202; results arrive via SSE)
curl -X POST http://localhost:8081/api/scan

# SSE stream (real-time events)
curl -N http://localhost:8081/api/events
```

SSE event types: `connected`, `scan_start`, `scan_result`, `scan_error`.

---

## Alerts

To receive a Telegram message whenever a device joins, leaves, or changes ports:

1. Create a bot via [@BotFather](https://t.me/BotFather) and copy the token.
2. Get your chat ID (send a message to your bot, then visit `https://api.telegram.org/bot<TOKEN>/getUpdates`).
3. Set the env vars in `docker-compose.yml`:

```yaml
DING_TELEGRAM_TOKEN: "123456:ABC-your-token"
DING_TELEGRAM_CHAT_ID: "987654321"
```

---

## Scan data

Results are stored in `./data/ding.json` (mounted into the container). The file holds the last 100 scans in JSON format.

---

## How it works

```
Browser / Android PWA
  └── GET /            ← React SPA (embedded in Go binary via go:embed)
  └── GET /api/events  ← SSE stream (real-time scan events)
  └── POST /api/scan   ← trigger scan

Go controller (ding)
  ├── serves HTTP on :8081
  ├── spawns Rust scanner binary as subprocess
  │     ├── ARP broadcast  → discovers IPs + MACs
  │     ├── ICMP echo      → confirms liveness
  │     └── TCP connect    → finds open ports
  │     └── prints JSON to stdout
  ├── diffs results against last scan → NEW / GONE / PORTS
  ├── saves results to /data/ding.json
  ├── pushes scan events to all SSE clients
  └── sends Telegram alert (if configured)
```

---

## Building from source

Rust, Go, and Node toolchains are not required on the host — the multi-stage Dockerfile handles everything.

```bash
docker compose build
```

To build locally for development:

```bash
# 1. Build and watch the React UI (Vite dev server on :5173, proxies /api to :8081)
cd ui && npm install && npm run dev

# 2. Build the Rust scanner
cd scanner && cargo build --release

# 3. Run the Go controller (serving API on :8081)
cd controller
cp -r ../ui/dist ./internal/api/static/
DING_SCANNER_BIN=../scanner/target/release/scanner go run ./cmd/ding
```