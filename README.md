# Ding

A fast network scanner that answers: who is on your network, what are they, and what ports are open.

**Architecture:** Rust handles low-level scanning (ARP, ICMP, TCP, mDNS). Go handles orchestration, enrichment, change detection, alerting, storage, and serves the web UI. The two communicate via JSON over stdout — no FFI, no shared memory.

---

## Install

### Docker (recommended — Linux host)

Pre-built images are published to **[hamed0406/ding](https://hub.docker.com/r/hamed0406/ding)** and **[ghcr.io/hamed0406/ding](https://github.com/hamed0406/ding/pkgs/container/ding)** for both `linux/amd64` and `linux/arm64` (Raspberry Pi-friendly). No source checkout needed.

**One-liner:**

```bash
docker run -d \
  --name ding \
  --network host \
  --cap-add NET_RAW --cap-add NET_ADMIN \
  -v ding-data:/data \
  hamed0406/ding:latest
```

**docker-compose (recommended):**

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
      DING_PORTS: "22,80,443,554,8000,8080,8443"
      DING_HTTP_ADDR: ":8081"
      DING_SCAN_INTERVAL: "60s"
    restart: unless-stopped
```

```bash
docker compose up -d                            # start
docker compose logs -f                          # tail logs
docker compose pull && docker compose up -d     # upgrade to newest :latest
docker compose down                             # stop
```

**Podman:**

```bash
# Option A — podman-compose (same workflow as Docker)
pip install podman-compose
sudo podman-compose up --build

# Option B — Quadlet (systemd service)
sudo podman build -t ding .
# Place ding.container in /etc/containers/systemd/ — see docs/quadlet below
sudo systemctl daemon-reload && sudo systemctl enable --now ding
```

> **Note:** Docker/Podman on Windows runs inside a Linux VM. `network_mode: host` gives the VM's virtual NIC, not your real LAN — ARP scanning won't find your devices. Use native binaries on Windows instead.

---

### Native binaries (Linux, macOS, Windows)

Download the latest release from the [GitHub Releases](../../releases/latest) page. Each archive contains two binaries: `scanner` (Rust) and `ding` (Go controller with UI embedded).

| Platform | File | Notes |
|---|---|---|
| Linux x86\_64 | `ding-linux-amd64.tar.gz` | |
| Linux ARM64 | `ding-linux-arm64.tar.gz` | Raspberry Pi, NAS |
| macOS Intel | `ding-macos-amd64.tar.gz` | Needs `sudo` |
| macOS Apple Silicon | `ding-macos-arm64.tar.gz` | Needs `sudo` |
| Windows x86\_64 | `ding-windows-amd64.zip` | Requires [Npcap](https://npcap.com) + run as Administrator |

**Linux / macOS:**

```bash
tar xzf ding-linux-amd64.tar.gz
sudo ./ding
# UI at http://localhost:8081
```

**Windows:**

1. Install [Npcap](https://npcap.com) (free)
2. Unzip `ding-windows-amd64.zip`
3. Run `ding.exe` as Administrator

**Verify download integrity:**

```bash
sha256sum -c sha256sums.txt
```

---

### Picking an image tag

| Tag | When to use |
|---|---|
| `1.2.3` | **Production.** Pinned, immutable, no surprise upgrades. |
| `1.2` | Latest patch of `1.2.x` — auto-upgrades on bug fixes |
| `1` | Latest `1.x.x` release |
| `latest` | Demos. Moves under you — not for production. |
| `main-<sha>` | Bleeding-edge build from `main`. Unstable. |

A single Docker tag is multi-arch — Docker picks `amd64` or `arm64` automatically.

---

## Web UI

The UI is a React PWA bundled into the Go binary. It works in any browser and is installable on Android from Chrome ("Add to Home Screen").

| Feature | Description |
|---|---|
| **Scan now** | Trigger an on-demand scan; results appear in real time via SSE |
| **Device grid** | All discovered devices — IP, MAC, hostname, vendor, device type, OS, open ports, alive status |
| **Search & filter** | Filter by name, IP, vendor, OS, or type; filter by online/offline status |
| **Device history** | Click any device card to open a full-page history view — dot timeline, uptime %, scan log with port-change markers |
| **Device labelling** | Assign a custom name ("Living Room TV") that persists across scans |
| **Per-device port scan** | Scan one device's ports on demand — no full network scan needed |
| **Wake-on-LAN** | Send a magic packet to wake an offline device (requires known MAC) |
| **Notification opt-in** | Bell icon on each card — off by default; click to enable alerts per device |
| **Topology map** | Interactive SVG star-topology map; switch between Grid and Topology views |
| **Changes feed** | NEW / GONE / BACK / PORTS changes with colour coding |
| **Passive detection** | Devices that send ARP traffic appear instantly without waiting for a scan |
| **Authentication** | Email/password + Google and GitHub OAuth; session persists via HttpOnly cookie and bearer token |
| **Telegram alerts** | Each user stores their own Telegram bot token and chat ID; alerts fire per-device when the bell is enabled |
| **Webhook alerts** | POST a JSON payload to any URL on change events — works natively with Slack, Discord, ntfy.sh, Home Assistant |
| **First / last seen** | Every device card shows when it was first discovered and when it last responded |
| **Speed test** | On-demand internet speed test (ping, download, upload) via Cloudflare; results saved and shown in history |
| **PWA** | Installable on Android; service worker disabled to avoid intercepting OAuth redirects |

---

## Configuration

All options are set via environment variables (or `.env` file when using the provided `docker-compose.yml`).

| Variable | Default | Description |
|---|---|---|
| `DING_INTERFACE` | _(auto)_ | Network interface to scan on |
| `DING_SUBNET` | _(auto)_ | Target subnet in CIDR notation |
| `DING_PORTS` | `22,80,443,554,8000,8080,8443` | TCP ports to probe on each host |
| `DING_TIMEOUT_MS` | `500` | Per-host timeout in milliseconds |
| `DING_DATA_PATH` | `/data/ding.db` | SQLite database path |
| `DING_HTTP_ADDR` | `:8081` | Address the web server listens on |
| `DING_SCAN_INTERVAL` | `60s` | Auto-scan interval (`""` = on-demand only) |
| `DING_SCANNER_BIN` | `/usr/local/bin/scanner` | Path to the Rust scanner binary |
| `DING_TELEGRAM_TOKEN` | _(empty)_ | Global fallback Telegram bot token |
| `DING_TELEGRAM_CHAT_ID` | _(empty)_ | Global fallback Telegram chat ID |
| `DING_BASE_URL` | _(empty)_ | Public URL — required behind a reverse proxy for OAuth |
| `DING_GOOGLE_CLIENT_ID` | _(empty)_ | Google OAuth client ID |
| `DING_GOOGLE_CLIENT_SECRET` | _(empty)_ | Google OAuth client secret |
| `DING_GITHUB_CLIENT_ID` | _(empty)_ | GitHub OAuth client ID |
| `DING_GITHUB_CLIENT_SECRET` | _(empty)_ | GitHub OAuth client secret |

---

## REST API

All endpoints except the auth ones require a valid session (`Authorization: Bearer <token>` header or `ding_session` cookie).

```bash
TOKEN="your-session-token"

# Status
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/status

# Device list
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/devices

# Per-device scan history
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/devices/192.168.1.42/history

# Trigger full scan
curl -X POST -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/scan

# Scan one device's ports
curl -X POST -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/devices/192.168.1.42/scan

# Wake-on-LAN
curl -X POST -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/devices/192.168.1.42/wake

# Set custom name
curl -X PUT -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/devices/192.168.1.42/label \
     -H 'Content-Type: application/json' -d '{"name":"Living Room TV"}'

# Remove custom name
curl -X DELETE -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/devices/192.168.1.42/label

# Enable notifications for a device
curl -X PUT -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/devices/192.168.1.42/notify \
     -H 'Content-Type: application/json' -d '{"enabled":true}'

# SSE stream
curl -N -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/events

# Get / save webhook URL
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/settings/webhook
curl -X PUT -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/settings/webhook \
     -H 'Content-Type: application/json' -d '{"url":"https://hooks.slack.com/services/…"}'

# Send a test webhook payload
curl -X POST -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/settings/webhook/test

# Run a speed test (takes 10–30 s)
curl -X POST -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/speedtest

# Speed test history (last 20 results)
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/speedtest/history
```

SSE event types: `connected`, `scan_start`, `scan_result`, `scan_error`, `device_seen`.

---

## Alerts

Ding sends alerts when a tracked device joins, leaves, or changes ports. Notifications are **opt-in per device** — disabled by default. Enable them by clicking the bell icon on a device card (turns cyan when active).

### Telegram

**Two ways to configure:**

1. **Per-user (recommended):** Log in → gear icon → Settings → Notifications. Each user's token and chat ID are stored in their account in the database.
2. **Global fallback:** Set `DING_TELEGRAM_TOKEN` and `DING_TELEGRAM_CHAT_ID` environment variables. Used only when no per-user config exists.

**Setup:**

1. Create a bot via [@BotFather](https://t.me/BotFather) on Telegram and copy the token.
2. Get your chat ID from [@userinfobot](https://t.me/userinfobot).
3. Enter them in Settings → Notifications and click **Send test message** to confirm.
4. Enable the bell icon on each device you want to track.

### Webhooks

Settings → Webhooks → paste any HTTPS URL. Ding will POST JSON on every change event:

```json
{
  "event": "network_change",
  "text": "Ding network changes:\n• NEW 192.168.1.42 …",
  "content": "…",
  "message": "…",
  "changes": [{ "kind": "NEW", "ip": "192.168.1.42", "desc": "…" }],
  "timestamp": "2026-04-30T12:00:00Z"
}
```

- **Slack** — paste your Incoming Webhook URL directly; the `text` field is picked up automatically.
- **Discord** — append `/slack` to your Discord webhook URL for Slack-compatible mode.
- **ntfy.sh** — use `https://ntfy.sh/your-topic`; the `message` field is used.
- **Home Assistant / n8n / Make** — any URL; parse the full JSON.

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
  ├── NetBIOS lookup        → hostnames for Windows / NAS / printers DNS misses (UDP 137, 16 workers)
  ├── MAC vendor lookup     → IEEE OUI database embedded in binary
  ├── device classify       → 50+ vendor + port rules → device category
  ├── HTTP banner probe     → Server header + <title> on port 80/8000/8080
  ├── RTSP probe            → OPTIONS handshake on port 554
  ├── mDNS overlay          → authoritative service-type categories
  ├── OS fingerprinting     → TTL + hostname patterns + device type → OS family
  ├── diffs results         → NEW / BACK / GONE / PORTS changes
  ├── saves to /data/ding.db (SQLite) + updates first/last-seen timestamps
  ├── pushes scan events to all SSE clients
  └── sends Telegram + webhook alerts (per-user config; bell-enabled devices only)

Passive ARP listener (always running)
  └── watches ARP traffic → device_seen SSE events without waiting for scan
```

---

## Building from source

### Docker (recommended)

The multi-stage Dockerfile builds everything inside containers — no host toolchains needed.

```bash
docker compose build
docker compose up -d
docker compose up --build   # rebuild + start in one shot
docker compose logs -f
```

### Local development

**Prerequisites**

| Toolchain | Version | Used for |
|---|---|---|
| Rust | stable | `scanner/` |
| Go | 1.25+ | `controller/` |
| Node | 22+ | `ui/` |
| Npcap SDK | any | Windows only — needed to `cargo build` on Windows |

Raw sockets require elevated privileges: `sudo` on Linux/macOS, Administrator on Windows.

**1. Build the Rust scanner**

```bash
cd scanner
cargo build --release
```

Smoke test (replace interface and subnet with yours):
```bash
# Linux / macOS
sudo ./target/release/scanner --interface eth0 --subnet 192.168.1.0/24
# Windows (as Administrator, with Npcap installed)
.\target\release\scanner.exe --interface "Ethernet" --subnet 192.168.1.0/24
```

**2. Build the React UI**

```bash
cd ui && npm install

npm run dev    # hot-reload on :5173, proxies /api → :8081
npm run build  # production build → dist/ (needed for the Go binary)
```

**3. Run the Go controller**

```bash
cd controller
cp -r ../ui/dist/* ./internal/api/static/

sudo DING_SCANNER_BIN=../scanner/target/release/scanner \
     DING_HTTP_ADDR=:8081 \
     go run ./cmd/ding
```

Open <http://localhost:8081> (or <http://localhost:5173> for the Vite dev server).

**Tests**

```bash
cd controller && go test ./...
cd scanner    && cargo test
```

**Common pitfalls**

- *`contains no embeddable files`* — `controller/internal/api/static/` is empty. Run `npm run build` and copy `dist/*` in.
- *`interface not found`* — list interfaces with `ip -4 addr show` (Linux) or `ipconfig` (Windows) and set `DING_INTERFACE`.
- *Empty scan results* — ran without `sudo` / Administrator. ARP/ICMP need elevated privileges.
- *Windows build error about wpcap* — set `LIB=<npcap-sdk>\Lib\x64` before `cargo build`.

---

## Releasing

Push a semver tag to trigger the release pipeline:

```bash
git tag v1.2.3
git push origin v1.2.3
```

GitHub Actions will:
1. Build Docker images (`linux/amd64` + `linux/arm64`) → Docker Hub + GHCR
2. Build native binaries for Linux, macOS, and Windows (5 targets)
3. Package archives and attach them to a GitHub Release with auto-generated changelog

Tags containing `-` (e.g. `v1.2.3-rc1`) are automatically marked as pre-releases.

---

## Scan data

Results are stored in `./data/ding.db` (SQLite). Key tables:

| Table | Contents |
|---|---|
| `scans` | One row per scan run (timestamp) |
| `devices` | One row per device per scan — IP, MAC, hostname, ports, alive, OS, etc. |
| `device_seen_at` | First and last seen timestamps per IP (updated on every scan) |
| `device_labels` | User-assigned custom names — survive scan cycles |
| `device_notify` | Per-device alert opt-in flags |
| `users` | Accounts — email, bcrypt hash, Telegram config, webhook URL |
| `speedtest_results` | Historical internet speed test results |

Schema migrations run automatically on startup — no manual steps needed when upgrading. No external database service is required; the SQLite engine is compiled into the binary.
