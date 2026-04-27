# Ding

A fast network scanner that answers: who is on your network, what are they, and what ports are open.

**Architecture:** Rust handles low-level scanning (ARP, ICMP, TCP, mDNS). Go handles orchestration, enrichment, change detection, alerting, storage, and serves the web UI. Docker ships everything — no host dependencies required.

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
      DING_PORTS: "22,80,443,554,8000,8080,8443"
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
| **Device grid** | All discovered devices — IP, MAC, hostname, vendor, device type, OS, open ports, alive status |
| **Device history** | Click any device card to open a full-page history view — dot timeline, uptime %, scan log with port-change markers |
| **Device labelling** | Assign a custom name to any device ("Living Room TV") that persists across scans |
| **Hostnames** | Reverse-DNS lookup runs in parallel after every scan (best-effort, 300 ms per host) |
| **MAC vendor** | Manufacturer name looked up from the embedded IEEE OUI database (no account needed) |
| **Device type** | Device category guessed from vendor + ports + HTTP banner + RTSP + mDNS (50+ rules) |
| **OS detection** | OS family inferred from TTL, hostname patterns, and device type (Linux, Windows, macOS, iOS, Android) |
| **Port names** | Port numbers shown as service names — `SSH/22`, `HTTPS/443`, `RTSP/554`, etc. |
| **Topology map** | Interactive SVG star-topology map; switch between Grid and Topology views |
| **Changes feed** | NEW / GONE / BACK / PORTS changes highlighted with colour coding |
| **Passive detection** | Devices that send ARP traffic appear in the UI instantly without waiting for a scheduled scan |
| **Auto-detect** | Interface and subnet shown in the header |
| **PWA** | Installable on Android home screen, works offline (cached shell) |

---

## Configuration

All options are set via environment variables.

| Variable | Default | Description |
|---|---|---|
| `DING_INTERFACE` | _(auto)_ | Network interface to scan on |
| `DING_SUBNET` | _(auto)_ | Target subnet in CIDR notation |
| `DING_PORTS` | `22,80,443,554,8000,8080,8443` | TCP ports to probe on each host |
| `DING_TIMEOUT_MS` | `500` | Per-host timeout in milliseconds |
| `DING_DATA_PATH` | `/data/ding.db` | Where scan history is stored (SQLite database) |
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

# Latest device list (with hostnames, vendor, device type, OS, ports)
curl http://localhost:8081/api/devices

# Per-device scan history (last 100 scans, oldest first)
curl http://localhost:8081/api/devices/192.168.1.42/history

# Scan history (last 20 full scan records)
curl http://localhost:8081/api/history

# Network topology graph (nodes + edges)
curl http://localhost:8081/api/topology

# Trigger a scan (returns 202; results arrive via SSE)
curl -X POST http://localhost:8081/api/scan

# Set a custom name for a device
curl -X PUT http://localhost:8081/api/devices/192.168.1.42/label \
     -H 'Content-Type: application/json' \
     -d '{"name":"Living Room TV"}'

# Remove a custom name
curl -X DELETE http://localhost:8081/api/devices/192.168.1.42/label

# SSE stream (real-time events)
curl -N http://localhost:8081/api/events
```

SSE event types: `connected`, `scan_start`, `scan_result`, `scan_error`, `device_seen`.

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

Results are stored in `./data/ding.db` (SQLite, mounted into the container). The database uses a normalized schema — one row per device per scan — which enables per-device history queries. No external database service is needed; the SQLite engine is compiled into the binary.

If you are upgrading from an older version that used `ding.json`, update `DING_DATA_PATH` in your compose file and the old JSON file can be left in place or deleted — it will not be read.

---

## How it works

```
Browser / Android PWA
  └── GET /            ← React SPA (embedded in Go binary via go:embed)
  └── GET /api/events  ← SSE stream (real-time scan events)
  └── POST /api/scan   ← trigger scan

Go controller (ding)
  ├── serves HTTP on :8081
  ├── spawns Rust scanner binary as subprocess ──────────────┐
  │     ├── ARP broadcast  → discovers IPs + MACs           │ parallel
  │     ├── ICMP echo      → confirms liveness + reads TTL  │ with
  │     ├── TCP connect    → finds open ports               │ mDNS
  │     └── prints JSON array to stdout                     │
  ├── spawns Rust scanner in --mode mdns ────────────────────┘
  │     └── PTR queries → device service types (3 s window)
  ├── reverse-DNS lookup    → hostnames (16 workers, 300 ms per host)
  ├── MAC vendor lookup     → IEEE OUI database embedded in binary
  ├── device classify       → 50+ vendor + port rules → device category
  ├── HTTP banner probe     → Server header + <title> on port 80/8000/8080
  ├── RTSP probe            → OPTIONS handshake on port 554
  ├── mDNS overlay          → authoritative service-type categories
  ├── OS fingerprinting     → TTL + hostname patterns + device type → OS family
  ├── diffs results         → NEW / BACK / GONE / PORTS changes
  ├── saves to /data/ding.db (SQLite)
  ├── pushes scan events to all SSE clients
  └── sends Telegram alert (if configured)

Passive ARP listener (always running)
  └── watches ARP traffic → device_seen SSE events without waiting for scan
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
| Go | 1.25+ | `controller/` |
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
