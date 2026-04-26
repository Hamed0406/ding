# Ding

A fast network scanner that answers: who is on your network, what are they, and what ports are open.

**Architecture:** Rust handles low-level scanning (ARP, ICMP, TCP). Go handles orchestration, change detection, alerting, storage, and serves the web UI. Docker ships everything — no host dependencies required.

---

## Requirements

- Docker + Docker Compose (Linux host only — ARP scanning requires `AF_PACKET` raw sockets)
- `NET_RAW` / `NET_ADMIN` capabilities (granted automatically via compose)

---

## Install from Docker Hub

Pre-built images are published to **[hamed0406/ding](https://hub.docker.com/r/hamed0406/ding)** for both `linux/amd64` and `linux/arm64` (Raspberry Pi-friendly). No source checkout needed.

### One-liner

```bash
docker run -d \
  --name ding \
  --network host \
  --cap-add NET_RAW --cap-add NET_ADMIN \
  -v ding-data:/data \
  hamed0406/ding:latest
```

Then open **http://\<host-ip\>:8081**.

### docker-compose (recommended)

Save this as `docker-compose.yml`:

```yaml
services:
  ding:
    image: hamed0406/ding:latest
    network_mode: host          # ARP needs to see the LAN
    cap_add:
      - NET_RAW
      - NET_ADMIN
    volumes:
      - ./data:/data
    environment:
      # Leave DING_INTERFACE / DING_SUBNET unset to auto-detect
      DING_PORTS: "22,80,443,8080,8443"
      DING_HTTP_ADDR: ":8081"
      DING_SCAN_INTERVAL: "60s"
      # Optional Telegram alerts:
      # DING_TELEGRAM_TOKEN: "..."
      # DING_TELEGRAM_CHAT_ID: "..."
    restart: unless-stopped
```

Then:

```bash
docker compose up -d                            # start
docker compose logs -f                          # tail logs
docker compose pull && docker compose up -d     # upgrade to newest :latest
docker compose down                             # stop
```

### Picking a tag

`hamed0406/ding` is one repository — the tags are just labels pointing at builds.

| Tag | When to use it |
|---|---|
| `1.2.3` | **Production.** Pinned, immutable, no surprise upgrades. |
| `1.2` | Latest patch of `1.2.x` — auto-upgrades on bug fixes |
| `1`   | Latest `1.x.x` release |
| `latest` | Demos. Moves under you — not for production. |
| `main-<sha>` | Bleeding-edge build from `main`. Unstable. |

A single tag is multi-arch — Docker picks `amd64` or `arm64` for you automatically.

### Find your interface and subnet (only if auto-detect picks the wrong one)

```bash
ip -4 addr show
```

Then set `DING_INTERFACE` and `DING_SUBNET` in the compose file.

---

## Web UI

The UI is a React PWA bundled into the Go binary. It works in any browser and is installable on Android from Chrome ("Add to Home Screen").

| Feature | Description |
|---|---|
| **Scan now** | Trigger an on-demand scan; results appear in real time via SSE |
| **Device grid** | All discovered devices — IP, MAC, hostname, open ports, alive status |
| **Hostnames** | Reverse-DNS lookup runs in parallel after every scan (best-effort, 300ms per host) |
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
  ├── reverse-DNS lookup   → fills in hostnames (parallel, best-effort)
  ├── diffs results against last scan → NEW / GONE / PORTS
  ├── saves results to /data/ding.json
  ├── pushes scan events to all SSE clients
  └── sends Telegram alert (if configured)
```

---

## Building from source

### Build with Docker (recommended)

The multi-stage Dockerfile builds everything inside containers — no host toolchains needed.

```bash
docker compose build           # builds UI, Rust scanner, Go controller
docker compose up              # start (foreground)
docker compose up -d           # start (detached)
docker compose up --build      # rebuild + start in one shot
docker compose down            # stop and remove the container
docker compose logs -f         # tail logs
```

The build is layer-cached: changing only Go code reuses the UI and Rust stages, and vice versa.

### Run locally for development

Useful when iterating on a single component (UI hot-reload, Rust logging, Go debugging).

**Prerequisites**

| Toolchain | Version | Used for |
|---|---|---|
| Rust | 1.70+ (stable) | `scanner/` |
| Go | 1.22+ | `controller/` |
| Node | 22+ (with npm) | `ui/` |

You also need `sudo` (or `CAP_NET_RAW` + `CAP_NET_ADMIN`) to run the scanner — ARP and ICMP need raw sockets.

**1. Build the Rust scanner**

```bash
cd scanner
cargo build --release          # output: target/release/scanner
cargo test                     # optional
```

Quick standalone smoke test (replace `eth0` and the subnet with your own):

```bash
sudo ./target/release/scanner --interface eth0 --subnet 192.168.1.0/24
```

**2. Build the React UI**

Two modes — pick one:

```bash
cd ui
npm install

# Mode A — hot-reload dev server on :5173, proxies /api to :8081
npm run dev

# Mode B — production build (output: dist/), needed before running the Go binary standalone
npm run build
```

**3. Run the Go controller**

```bash
cd controller

# go:embed needs static/ to be non-empty at compile time.
# Copy the UI build output in (only needed if you used Mode B above).
cp -r ../ui/dist/* ./internal/api/static/

# Run the controller. Sudo is required because the spawned scanner needs raw sockets.
sudo DING_SCANNER_BIN=../scanner/target/release/scanner \
     DING_HTTP_ADDR=:8081 \
     go run ./cmd/ding
```

Open <http://localhost:8081>. If you're using UI Mode A (Vite), open <http://localhost:5173> instead — it proxies API calls to the Go server.

**Run the tests**

```bash
cd controller && go test ./...
cd scanner    && cargo test
cd ui         && npm run build      # type-checks via tsc
```

**Common pitfalls**

- *`pattern static: cannot embed directory static: contains no embeddable files`* — `controller/internal/api/static/` is empty. Run `npm run build` in `ui/` and copy `dist/*` in (see step 3).
- *`interface "..." not found or has no IPv4 address`* — list interfaces with `ip -4 addr show` and pass a real one via `DING_INTERFACE`.
- *Empty scan results* — you probably ran without `sudo`. ARP/ICMP need `CAP_NET_RAW`.
