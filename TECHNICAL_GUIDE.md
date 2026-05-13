# Ding — Technical Guide

A fast network scanner that answers: who is on your network, what are they, and what ports are open.

**Architecture:** Rust handles low-level scanning (ARP, ICMP, TCP, mDNS). Go handles orchestration, enrichment, change detection, alerting, storage, and serves the web UI. The two communicate via JSON over stdout — no FFI, no shared memory.

---

## Table of Contents

1. [Installation](#installation)
2. [Web UI](#web-ui)
3. [Configuration](#configuration)
4. [Logging](#logging)
5. [REST API](#rest-api)
6. [Alerts](#alerts)
7. [How It Works](#how-it-works)
8. [Building from Source](#building-from-source)
9. [Releasing](#releasing)
10. [Scan Data](#scan-data)
11. [Feature Status & Roadmap](#feature-status--roadmap)
12. [Developer Guide](#developer-guide)

---

## Installation

### One-line install

**Linux** — installs via Docker or as a native systemd service (you choose):

```bash
curl -fsSL https://raw.githubusercontent.com/hamed0406/ding/main/scripts/install.sh | sudo bash
```

**Windows** (PowerShell as Administrator) — installs via Docker Desktop or as a native Windows Service (you choose):

```powershell
irm https://raw.githubusercontent.com/hamed0406/ding/main/scripts/install.ps1 | iex
```

Or download and inspect first (recommended):

```bash
# Linux
curl -fsSL https://raw.githubusercontent.com/hamed0406/ding/main/scripts/install.sh -o install.sh
cat install.sh          # review before running
sudo bash install.sh
```

```powershell
# Windows (PowerShell as Administrator)
Invoke-WebRequest https://raw.githubusercontent.com/hamed0406/ding/main/scripts/install.ps1 -OutFile install.ps1
Get-Content install.ps1 | more   # review before running
.\install.ps1
```

Both scripts:
- Ask whether to use Docker or a native binary (Docker is the default)
- Check all prerequisites before proceeding and offer to install missing ones
- Create the install directory (`/opt/ding` on Linux, `C:\Program Files\Ding` on Windows)
- Generate a random `DING_SECRET_KEY` automatically and save it to `.env`
- Optionally set up Telegram alerts interactively
- Start Ding and print the URL + your secret key (back it up!)

> **Linux native binary:** the script installs a `systemd` service and sets `CAP_NET_RAW`/`CAP_NET_ADMIN` on the scanner binary so it does not need to run as root.
>
> **Windows native binary:** requires [Npcap](https://npcap.com) for ARP/ICMP — the script offers to download and install it for you.
>
> **Docker on Windows won't scan your LAN:** Docker Desktop runs inside a Linux VM; `network_mode: host` attaches the VM's virtual NIC, not your real adapter. Use the native binary option on Windows for full LAN scanning.

For other platforms see below.

---

### Linux — Docker / Docker Compose (recommended)

Pre-built images for `linux/amd64` and `linux/arm64` are published on **[Docker Hub](https://hub.docker.com/r/hamed0406/ding)** and **[GHCR](https://github.com/hamed0406/ding/pkgs/container/ding)**. No source checkout needed.

**1. Create a working directory and config file**

```bash
mkdir ding && cd ding
```

Save the following as `docker-compose.yml`:

```yaml
services:
  ding:
    image: hamed0406/ding:latest
    network_mode: host        # required — ARP must reach the physical LAN
    cap_add:
      - NET_RAW               # raw sockets for ARP / ICMP
      - NET_ADMIN             # interface access
    volumes:
      - ./data:/data          # scan history, users, speed-test results
    environment:
      DING_SCAN_INTERVAL: "60s"
      # DING_INTERFACE: eth0  # uncomment if auto-detect picks the wrong NIC
      # DING_PORTS: "22,80,443,554,8000,8080,8443"
    restart: unless-stopped
```

**2. Start**

```bash
docker compose up -d
```

Open **http://localhost:8081** — you'll be prompted to create an account on first visit.

**3. Day-to-day commands**

```bash
docker compose logs -f                           # live logs
docker compose pull && docker compose up -d      # upgrade to latest
docker compose down                              # stop
docker compose down -v                           # stop + wipe data
```

**4. One-liner (no Compose)**

```bash
docker run -d \
  --name ding \
  --network host \
  --cap-add NET_RAW --cap-add NET_ADMIN \
  -v ding-data:/data \
  -e DING_SCAN_INTERVAL=60s \
  hamed0406/ding:latest
```

---

### Linux — native binary

Use this if you don't want Docker, or want to run Ding as a `systemd` service.

**1. Download and extract**

```bash
curl -LO https://github.com/hamed0406/ding/releases/latest/download/ding-linux-amd64.tar.gz
sha256sum -c sha256sums.txt          # verify integrity
tar xzf ding-linux-amd64.tar.gz
sudo mv ding scanner /usr/local/bin/
```

> ARM64 (Raspberry Pi, NAS): use `ding-linux-arm64.tar.gz` instead.

**2. Run manually**

```bash
sudo ding
# UI at http://localhost:8081
```

Configuration is done via environment variables:

```bash
sudo DING_SCAN_INTERVAL=60s DING_HTTP_ADDR=:8081 ding
```

**3. Run as a systemd service (auto-start on boot)**

```bash
sudo useradd -r -s /bin/false ding    # dedicated service user
sudo mkdir -p /var/lib/ding
```

Save as `/etc/systemd/system/ding.service`:

```ini
[Unit]
Description=Ding network scanner
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/ding
Environment=DING_DATA_PATH=/var/lib/ding/ding.db
Environment=DING_HTTP_ADDR=:8081
Environment=DING_SCAN_INTERVAL=60s
Restart=on-failure
RestartSec=5s
# Raw socket access — required for ARP/ICMP
AmbientCapabilities=CAP_NET_RAW CAP_NET_ADMIN
CapabilityBoundingSet=CAP_NET_RAW CAP_NET_ADMIN
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now ding
sudo systemctl status ding
journalctl -u ding -f           # live logs
```

**4. Upgrade**

```bash
curl -LO https://github.com/hamed0406/ding/releases/latest/download/ding-linux-amd64.tar.gz
tar xzf ding-linux-amd64.tar.gz
sudo systemctl stop ding
sudo mv ding scanner /usr/local/bin/
sudo systemctl start ding
```

---

### Linux — Podman

**Option A — podman-compose (same workflow as Docker Compose)**

```bash
pip install podman-compose
mkdir ding && cd ding
# create the same docker-compose.yml shown in the Docker section above
sudo podman-compose up -d
```

**Option B — Podman Quadlet (native systemd integration, no Docker Compose needed)**

Create `/etc/containers/systemd/ding.container`:

```ini
[Unit]
Description=Ding network scanner
After=network-online.target

[Container]
Image=docker.io/hamed0406/ding:latest
Network=host
AddCapability=CAP_NET_RAW CAP_NET_ADMIN
Volume=/var/lib/ding:/data
Environment=DING_SCAN_INTERVAL=60s
Environment=DING_HTTP_ADDR=:8081

[Service]
Restart=on-failure

[Install]
WantedBy=multi-user.target default.target
```

```bash
sudo mkdir -p /var/lib/ding
sudo systemctl daemon-reload
sudo systemctl enable --now ding
sudo systemctl status ding
```

Podman automatically pulls the image on first start and handles updates with `podman auto-update`.

---

### Raspberry Pi

Ding ships a native `linux/arm64` binary — tested on Raspberry Pi 3, 4, and 5 running Raspberry Pi OS (64-bit) or Ubuntu.

**Docker Compose (easiest)**

```bash
# Install Docker if not present
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER   # log out and back in after this

mkdir ding && cd ding
# create docker-compose.yml as shown above (same file, works on arm64 too)
docker compose up -d
```

Open **http://\<pi-ip\>:8081** from any device on your network.

**Native binary**

```bash
curl -LO https://github.com/hamed0406/ding/releases/latest/download/ding-linux-arm64.tar.gz
tar xzf ding-linux-arm64.tar.gz
sudo mv ding scanner /usr/local/bin/
sudo ding
```

Then follow the systemd service steps in the [Linux — native binary](#linux--native-binary) section.

**Tips for Raspberry Pi**

- Set a static IP on your Pi so the UI URL never changes.
- The default 512 MB swap on older Pi models is enough for normal use.
- If you have multiple NICs (e.g. both `eth0` and `wlan0`), set `DING_INTERFACE` explicitly.

---

### macOS

**Prerequisite:** raw socket access requires `sudo`. No other special drivers needed — macOS has BPF support built in.

**1. Download and extract**

```bash
# Apple Silicon (M1/M2/M3/M4)
curl -LO https://github.com/hamed0406/ding/releases/latest/download/ding-macos-arm64.tar.gz
tar xzf ding-macos-arm64.tar.gz

# Intel Mac
curl -LO https://github.com/hamed0406/ding/releases/latest/download/ding-macos-amd64.tar.gz
tar xzf ding-macos-amd64.tar.gz
```

**2. Remove quarantine and run**

macOS Gatekeeper will block unsigned binaries downloaded from the internet. Remove the quarantine attribute before running:

```bash
xattr -d com.apple.quarantine ding scanner
sudo ./ding
```

Open **http://localhost:8081**.

**3. Pass configuration**

```bash
sudo DING_SCAN_INTERVAL=60s DING_HTTP_ADDR=:8081 ./ding
```

**4. Run as a launchd service (auto-start on login)**

Save as `~/Library/LaunchAgents/com.ding.scanner.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>        <string>com.ding.scanner</string>
  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/bin/ding</string>
  </array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>DING_SCAN_INTERVAL</key> <string>60s</string>
    <key>DING_HTTP_ADDR</key>     <string>:8081</string>
    <key>DING_DATA_PATH</key>     <string>/usr/local/var/ding/ding.db</string>
  </dict>
  <key>RunAtLoad</key>    <true/>
  <key>KeepAlive</key>    <true/>
  <key>StandardOutPath</key> <string>/usr/local/var/log/ding.log</string>
  <key>StandardErrorPath</key><string>/usr/local/var/log/ding.log</string>
</dict>
</plist>
```

```bash
sudo mkdir -p /usr/local/bin /usr/local/var/ding /usr/local/var/log
sudo cp ding scanner /usr/local/bin/
launchctl load ~/Library/LaunchAgents/com.ding.scanner.plist
```

**5. Upgrade**

```bash
launchctl unload ~/Library/LaunchAgents/com.ding.scanner.plist
# replace binaries
launchctl load ~/Library/LaunchAgents/com.ding.scanner.plist
```

---

### Windows

> Docker Desktop on Windows **will not work for scanning** — containers run in a Linux VM and ARP packets can't reach your physical network. Use the native binary.

**Prerequisites**

- Windows 10 / 11 (x86-64)
- [Npcap](https://npcap.com) — free packet-capture driver required by the scanner. Download and install before running Ding.
  - During install, check **"Install Npcap in WinPcap API-compatible mode"**

**1. Download and extract**

Download `ding-windows-amd64.zip` from the [latest release](../../releases/latest) and extract it anywhere, e.g. `C:\ding\`.

**2. Find your interface name**

```powershell
Get-NetAdapter | Select-Object Name, InterfaceDescription, Status
```

**3. Run as Administrator**

```powershell
cd C:\ding
$env:DING_INTERFACE    = "Wi-Fi"      # replace with your adapter name
$env:DING_SCAN_INTERVAL= "60s"
$env:DING_HTTP_ADDR    = ":8081"
.\ding.exe
```

Open **http://localhost:8081**.

**4. Run as a Windows Service (auto-start on boot)**

```powershell
# Download nssm, then:
nssm install Ding C:\ding\ding.exe
nssm set Ding AppEnvironmentExtra DING_INTERFACE=Wi-Fi DING_SCAN_INTERVAL=60s DING_HTTP_ADDR=:8081
nssm set Ding AppDirectory C:\ding
nssm set Ding ObjectName LocalSystem    # run as SYSTEM for raw socket access
nssm start Ding
```

Or with the built-in `sc`:

```powershell
sc.exe create Ding binPath= "C:\ding\ding.exe" start= auto obj= LocalSystem
sc.exe start Ding
```

Configure environment variables for the `Ding` service key in the registry at  
`HKLM\SYSTEM\CurrentControlSet\Services\Ding\Environment`.

**5. Firewall**

```powershell
New-NetFirewallRule -DisplayName "Ding" -Direction Inbound -Protocol TCP -LocalPort 8081 -Action Allow
```

**6. Upgrade**

```powershell
sc.exe stop Ding
# replace ding.exe and scanner.exe
sc.exe start Ding
```

---

### Docker image tags

| Tag | When to use |
|---|---|
| `1.2.3` | **Production.** Pinned, immutable, no surprise upgrades. |
| `1.2` | Latest patch of `1.2.x` — auto-upgrades on bug fixes |
| `1` | Latest `1.x.x` release |
| `latest` | Demos and quick tests. Moves under you — not for production. |
| `main-<sha>` | Bleeding-edge build from `main`. Unstable. |

A single tag is multi-arch — Docker and Podman pick `amd64` or `arm64` automatically.

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
| **Telegram alerts** | Each user stores their own Telegram bot token and chat ID |
| **Email alerts** | SMTP-based alerts — supports Gmail, Outlook, Yahoo, iCloud, Fastmail, Brevo, or any self-hosted relay |
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
| `DING_SECRET_KEY` | _(empty)_ | Encrypts SMTP passwords stored in SQLite (AES-256-GCM). Generate with `openssl rand -base64 32`. If unset, passwords are stored in plaintext and a warning is logged. |
| `DING_BASE_URL` | _(empty)_ | Public URL — required behind a reverse proxy for OAuth |
| `DING_GOOGLE_CLIENT_ID` | _(empty)_ | Google OAuth client ID |
| `DING_GOOGLE_CLIENT_SECRET` | _(empty)_ | Google OAuth client secret |
| `DING_GITHUB_CLIENT_ID` | _(empty)_ | GitHub OAuth client ID |
| `DING_GITHUB_CLIENT_SECRET` | _(empty)_ | GitHub OAuth client secret |
| `DING_LOG_LEVEL` | `info` | Log verbosity: `debug`, `info`, `warn`, `error` |
| `DING_LOG_FORMAT` | `text` | Log format: `text` (human-readable) or `json` (structured, for log aggregators) |

---

## Logging

Ding writes all logs to **stderr** — it does not open a log file itself.

| Mode | How to view live logs |
|---|---|
| Docker / Docker Compose | `docker compose logs -f ding` |
| Podman | `podman logs -f ding` |
| systemd (native binary) | `journalctl -u ding -f` |
| Direct terminal run | stderr is printed to your terminal |

### Log level and format

```bash
DING_LOG_LEVEL=info    # debug | info | warn | error
DING_LOG_FORMAT=text   # text (human-readable) | json (structured)
```

Use `json` format when piping logs into a collector (Loki, Elasticsearch, CloudWatch, etc.).

### Save logs to a file

**Docker Compose** — add a logging driver in `docker-compose.yml`:

```yaml
services:
  ding:
    image: hamed0406/ding:latest
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "5"
```

**systemd (native binary)** — redirect the journal to a plain file:

```ini
[Service]
StandardError=append:/var/log/ding.log
```

**Direct / manual run:**

```bash
sudo ding 2>> /var/log/ding.log
```

---

## REST API

All endpoints except the auth ones require a valid session (`Authorization: Bearer <token>` header or `ding_session` cookie).

| Method | Path | Description |
|---|---|---|
| GET | `/api/status` | Interface, subnet, last scan time |
| GET | `/api/devices` | All known devices |
| GET | `/api/devices/{ip}/history` | Last 100 scans for one device |
| GET | `/api/history` | Last 20 full scan records |
| GET | `/api/topology` | Network graph (nodes + edges) |
| POST | `/api/scan` | Trigger a new scan (202 Accepted; results via SSE) |
| POST | `/api/devices/{ip}/scan` | Scan one device's ports immediately |
| POST | `/api/devices/{ip}/wake` | Send Wake-on-LAN magic packet |
| PUT | `/api/devices/{ip}/label` | Set a custom name `{"name": "Living Room TV"}` |
| DELETE | `/api/devices/{ip}/label` | Remove a custom name |
| PUT | `/api/devices/{ip}/notify` | Toggle per-device alerts `{"enabled": true}` |
| GET | `/api/changes` | Persistent change log (last 200 events, newest first) |
| GET | `/api/arpwatch` | IPs seen with more than one MAC (spoofing candidates) |
| GET | `/api/devices/export` | Export all devices as CSV (default) or JSON (`?format=json`) |
| GET | `/api/settings/telegram` | Get current user's Telegram config |
| PUT | `/api/settings/telegram` | Save current user's Telegram token + chat ID |
| POST | `/api/settings/telegram/test` | Send a test Telegram message |
| GET | `/api/settings/email` | Get current user's SMTP email config (password masked) |
| PUT | `/api/settings/email` | Save current user's SMTP config |
| POST | `/api/settings/email/test` | Send a test email |
| GET | `/api/settings/webhook` | Get current user's webhook URL |
| PUT | `/api/settings/webhook` | Save current user's webhook URL |
| POST | `/api/settings/webhook/test` | Send a test webhook payload |
| POST | `/api/speedtest` | Run an internet speed test (blocks 5–30 s; 409 if already running) |
| GET | `/api/speedtest/history` | Last 20 speed test results, newest first |
| GET | `/api/auth/providers` | Available OAuth providers (public) |
| POST | `/api/auth/register` | Email/password registration |
| POST | `/api/auth/login` | Email/password login |
| POST | `/api/auth/logout` | End session |
| POST | `/api/auth/exchange` | Consume one-time OAuth exchange token |
| GET | `/api/events` | SSE stream (scan_start / scan_result / scan_error / device_seen) |

SSE event types: `connected`, `scan_start`, `scan_result`, `scan_error`, `device_seen`.

### Example curl calls

```bash
TOKEN="your-session-token"

curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/status
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/devices
curl -X POST -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/scan
curl -X POST -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/devices/192.168.1.42/scan
curl -X POST -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/devices/192.168.1.42/wake
curl -X PUT  -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/devices/192.168.1.42/label \
     -H 'Content-Type: application/json' -d '{"name":"Living Room TV"}'
curl -N -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/events
```

---

## Alerts

Ding sends alerts when a tracked device joins, leaves, or changes ports.

| Tier | When | Opt-out? |
|---|---|---|
| **Always-on** | Brand-new device (first time ever seen) | No |
| **Always-on** | ARP MAC change on a known IP (possible ARP spoofing) | No |
| **Per-device** | Device returns, goes offline, or changes open ports | Yes — bell icon |

Per-device notifications are **disabled by default**. Enable them by clicking the bell icon on a device card (turns cyan when active).

### Telegram

1. Create a bot via [@BotFather](https://t.me/BotFather) on Telegram and copy the token.
2. Get your chat ID from [@userinfobot](https://t.me/userinfobot).
3. Enter them in Settings → Notifications and click **Send test message** to confirm.
4. Enable the bell icon on each device you want to track.

Two ways to configure:
- **Per-user (recommended):** Log in → Settings → Notifications. Each user's token and chat ID are stored in their account.
- **Global fallback:** Set `DING_TELEGRAM_TOKEN` and `DING_TELEGRAM_CHAT_ID` environment variables.

### Email

Settings → Email → configure your SMTP provider.

**External providers (Gmail, Outlook, Yahoo, iCloud, Fastmail, Brevo)**

Pick your provider from the preset dropdown — host and port are filled in automatically. Enter your username, App Password, From address, and the address to send alerts to.

**Self-hosted relay (Postfix, Mailpit)**

Set host to `localhost`, port to `25` (Postfix) or `1025` (Mailpit), and leave username/password blank if no auth is required.

> **Testing with Mailpit:**
> ```bash
> docker compose --profile mailpit up -d
> ```
> Configure host=`localhost`, port=`1025`. Caught emails appear at **http://\<host\>:8025**.

#### Protecting your SMTP password (`DING_SECRET_KEY`)

By default Ding stores your SMTP password in SQLite in plain text.

**Step 1 — generate a key:**
```bash
openssl rand -base64 32
```

**Step 2 — add it to your `.env`:**
```bash
DING_SECRET_KEY=4X3mK9vPqRzL2YwN8TdJcHbF7sAeUiGo1nQxZyCpVkW=
```

**Step 3 — restart Ding.** Re-open Settings → Email, re-enter your SMTP password, and click Save. The password is now stored encrypted (AES-256-GCM, prefixed `enc:v1:`).

> Existing plain-text passwords keep working after you set `DING_SECRET_KEY` — they remain unencrypted in the DB until you re-save them through the Settings UI.

### Webhooks

Settings → Webhooks → paste any HTTPS URL. Ding will POST JSON on every change event:

```json
{
  "event": "network_change",
  "text": "Ding network changes:\n• NEW 192.168.1.42 …",
  "changes": [{ "kind": "NEW", "ip": "192.168.1.42", "desc": "…" }],
  "timestamp": "2026-04-30T12:00:00Z"
}
```

- **Slack** — paste your Incoming Webhook URL directly.
- **Discord** — append `/slack` to your Discord webhook URL for Slack-compatible mode.
- **ntfy.sh** — use `https://ntfy.sh/your-topic`.
- **Home Assistant / n8n / Make** — any URL; parse the full JSON.

---

## How It Works

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
  ├── NetBIOS lookup        → hostnames for Windows / NAS / printers (UDP 137)
  ├── MAC vendor lookup     → IEEE OUI database embedded in binary
  ├── device classify       → 50+ vendor + port rules → device category
  ├── HTTP banner probe     → Server header + <title> on port 80/8000/8080
  ├── RTSP probe            → OPTIONS handshake on port 554
  ├── mDNS overlay          → authoritative service-type categories
  ├── OS fingerprinting     → TTL + hostname patterns + device type → OS family
  ├── diffs results         → NEW / BACK / GONE / PORTS changes
  ├── saves to /data/ding.db (SQLite) + updates first/last-seen timestamps
  ├── pushes scan events to all SSE clients
  └── sends Telegram + email + webhook alerts

Passive ARP listener (always running)
  └── watches ARP traffic → device_seen SSE events without waiting for scan
```

### Data flow (detailed)

```
main.go
  → starts HTTP server (api.NewServer)
  → TriggerScan() on startup, then on DING_SCAN_INTERVAL ticker
    ┌── scanner.Run()        # ARP+ICMP+TCP scan, parse JSON stdout     ─┐
    └── scanner.RunMDNS()    # mDNS PTR queries, 3 s window             ─┤ parallel
    → enrich.Hostnames()        # reverse-DNS, 16 workers, 300 ms timeout  ←┘
    → enrich.NetBIOSNames()     # NetBIOS UDP 137, fills gaps DNS misses (16 workers, 1 s)
    → vendor.Annotate()         # MAC → manufacturer (embedded OUI DB)
    → classify.Annotate()       # vendor + ports → device category (50+ rules)
    → enrich.BannerDeviceType() # HTTP banner on port 80/8000/8080 (8 workers, 800 ms)
    → enrich.RTSPDeviceType()   # RTSP OPTIONS on port 554 (8 workers, 600 ms)
    → enrich.ApplyMDNS()        # mDNS service type → device category (authoritative)
    → enrich.AnnotateOS()       # TTL + hostname + device type → OS family
    → store.Latest()            # read previous scan from SQLite
    → store.AllKnownIPs()       # all IPs ever seen (for NEW vs BACK detection)
    → diff.Compare()            # produce []Change (NEW / GONE / PORTS / BACK)
    → store.Save()              # write to SQLite; upserts device_seen_at (first/last seen)
    → alert.Send()              # Telegram + webhooks; NEW+MAC_CHANGE always; BACK/GONE/PORTS only if bell enabled
    → email.Send()              # SMTP email; same change filter; all users with host+to configured
    → broker.Publish()          # push SSE scan_result with store.AllDevices() registry
```

---

## Building from Source

### Docker (recommended)

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
| Npcap SDK | any | Windows only |

**1. Build the Rust scanner**

```bash
cd scanner
cargo build --release
```

Smoke test:
```bash
sudo ./target/release/scanner --interface eth0 --subnet 192.168.1.0/24
```

**2. Build the React UI**

```bash
cd ui && npm install
npm run dev    # hot-reload on :5173, proxies /api → :8081
npm run build  # production build → dist/
```

**3. Run the Go controller**

```bash
cd controller
cp -r ../ui/dist/* ./internal/api/static/

sudo DING_SCANNER_BIN=../scanner/target/release/scanner \
     DING_HTTP_ADDR=:8081 \
     go run ./cmd/ding
```

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

## Scan Data

Results are stored in `./data/ding.db` (SQLite). Key tables:

| Table | Contents |
|---|---|
| `scans` | One row per scan run (timestamp) |
| `devices` | One row per device per scan — IP, MAC, hostname, ports, alive, OS, etc. |
| `device_seen_at` | First and last seen timestamps per IP |
| `device_labels` | User-assigned custom names — survive scan cycles |
| `device_notify` | Per-device alert opt-in flags |
| `users` | Accounts — email, bcrypt hash, Telegram config, webhook URL |
| `user_email_config` | Per-user SMTP settings |
| `speedtest_results` | Historical internet speed test results |

Schema migrations run automatically on startup — no manual steps needed when upgrading.

---

## Feature Status & Roadmap

### Implemented

| Feature | Where |
|---|---|
| ARP-based device discovery | `scanner/src/arp.rs` |
| ICMP liveness + TTL capture | `scanner/src/ping.rs` |
| TCP port scanning | `scanner/src/tcp.rs` |
| mDNS / Bonjour service discovery | `scanner/src/mdns.rs` |
| Passive ARP listener (instant device detection) | `scanner/src/arp.rs`, `cmd/ding/main.go` |
| Cross-platform scanner (Linux / macOS / Windows) | `scanner/src/arp.rs`, `scanner/src/gateway.rs` |
| Reverse-DNS hostname lookup (16 workers, 300 ms) | `internal/enrich/dns.go` |
| NetBIOS Node Status enricher (UDP 137) | `internal/enrich/netbios.go` |
| MAC vendor lookup (IEEE OUI, embedded, offline) | `internal/vendor/vendor.go` |
| Device type classification — 50+ vendor + port rules | `internal/classify/classify.go` |
| HTTP banner fingerprinting | `internal/enrich/http.go` |
| SNMP sysDescr + sysName probe (UDP 161) | `internal/enrich/snmp.go` |
| RTSP OPTIONS probe on port 554 | `internal/enrich/rtsp.go` |
| OS fingerprinting (TTL + hostname + device type) | `internal/enrich/os.go` |
| mDNS service-type overlay | `internal/enrich/mdns.go` |
| Scan-to-scan diff: NEW / BACK / GONE / PORTS | `internal/diff/diff.go` |
| Persistent change log (SQLite-backed) | `internal/storage/sqlite_store.go` |
| SQLite scan history (normalized, per-device queries) | `internal/storage/sqlite_store.go` |
| First-seen / last-seen timestamps per device | `internal/storage/sqlite_store.go` |
| Device labelling (custom names, persisted) | `internal/api/handlers.go` |
| Per-device port scan (instant, no full network scan) | `internal/api/handlers.go` |
| Wake-on-LAN (magic packet via UDP broadcast) | `internal/api/handlers.go` |
| Per-device notification opt-in (bell toggle) | `internal/storage/sqlite_store.go` |
| Telegram alerts — per-user token + chat ID | `internal/alert/alert.go` |
| Email alerts — SMTP, auto-TLS, provider presets | `internal/email/email.go` |
| Webhook alerts (Slack / Discord / ntfy.sh compatible) | `internal/alert/alert.go` |
| Internet speed test (ping / download / upload) | `internal/speedtest/speedtest.go` |
| CSV / JSON device export | `internal/api/handlers.go` |
| ARP spoofing detection | `internal/storage/sqlite_store.go` |
| Network topology map (SVG star layout) | `internal/topology/topology.go` |
| Per-device history view (dot timeline, scan log) | `ui/src/components/DeviceHistory.tsx` |
| Device search and status filter | `ui/src/App.tsx` |
| Real-time SSE event stream | `internal/api/sse.go` |
| Email/password authentication (bcrypt) | `internal/api/handlers.go` |
| Google OAuth login | `internal/api/server.go` |
| GitHub OAuth login | `internal/api/server.go` |
| React PWA — installable on Android / desktop | `ui/` |
| Docker multi-arch images (linux/amd64 + linux/arm64) | `Dockerfile` |
| Podman support (podman-compose + Quadlet) | `docker-compose.yml` |
| Native binary releases (Linux, macOS, Windows) | `.github/workflows/release.yml` |

### Roadmap

#### Quick wins

- **Port service labels on device cards** — display "SSH · HTTP · HTTPS" instead of "22 · 80 · 443". The mapping already exists in `ui/src/utils/ports.ts`.
- **Dark / light mode toggle** — Tailwind `dark:` variants; preference stored in `localStorage`.
- **Alert cooldown / dedup** — suppress repeated GONE alerts for a device that flaps.
- **Quiet hours for alerts** — time-window rule: "don't send alerts between 11pm–7am".

#### Medium complexity

- **Device uptime / availability stats** — per-device alive% over last 7 / 30 days; data already in SQLite scan history.
- **Device groups / tags** — named groups ("IoT", "Servers", "Cameras") with one new `device_groups` table.
- **Ping latency history** — store ICMP RTT per scan (Rust already measures it), show sparkline on device card.
- **Multi-subnet scanning** — accept comma-separated `DING_SUBNET` values.
- **DHCP hostname snooping** — listen on UDP 67/68 to capture hostnames from DHCP Option 12.
- **Settings page: scheduled scan config** — UI control for `DING_SCAN_INTERVAL` without restarting.

#### Advanced

- **Smart alert rules engine** — user-defined conditions ("Alert if device X is offline for more than N minutes", "Alert if port 22 opens on any unlabelled device"). One `alert_rules` table + rule-builder UI.

- **Network security audit** — post-scan scoring: open Telnet (23) / FTP (21), HTTP without HTTPS, unidentified devices. Green / amber / red badge on each device card.

- **Browser push notifications (Web Push)** — the app is already a PWA. Generate a VAPID key pair on first run; store `PushSubscription` per user in SQLite; fan out push messages after each scan.

- **IPv6 support** — ICMPv6 neighbor discovery in Rust; extend `types.rs` and `ScanResult` for IPv6.

- **Bandwidth monitoring** — per-device traffic stats via iptables conntrack, eBPF, or SNMP ifInOctets / ifOutOctets poll.

- **Configurable scan profiles** — named profiles: "quick" (ARP only), "standard" (default), "deep" (full port range), "stealth" (slow, randomised). Stored in SQLite; selectable from UI.

- **Postgres / MySQL backend** — implement `storage.Store` and `storage.UserStore`; swap the constructor in `main.go`. No other changes needed.

- **macOS / Windows installer** — bundle into a `.dmg` or `.msi` with Npcap bundled.

- **Home Assistant / MQTT integration** — publish device presence to an MQTT broker after each scan.

---

## Developer Guide

### Directory layout

```
scanner/                  Rust crate — standalone binary
  src/
    main.rs               CLI entry point (clap); three modes: scan, listen, mdns
    arp.rs                ARP discovery + passive ARP listener (pnet datalink — AF_PACKET/BPF/Npcap)
    ping.rs               ICMP liveness check + TTL capture (raw ICMP socket via socket2)
    tcp.rs                TCP port scan via std::net::TcpStream::connect_timeout
    mdns.rs               mDNS/Bonjour discovery — multicast PTR queries, DNS correlation
    gateway.rs            Default gateway detection for topology
    types.rs              ScanResult, ArpEvent, MdnsEvent structs (serde Serialize/Deserialize)

controller/               Go module (github.com/ding/ding)
  cmd/ding/main.go        Binary entry point — config, HTTP server, parallel scan+mDNS loop
  internal/
    scanner/runner.go     Spawns Rust binary; Run(), Listen(), RunMDNS()
    storage/
      store.go            Store interface + JSONStore (JSON-file fallback, kept for reference)
      sqlite_store.go     SQLiteStore — active backend (modernc.org/sqlite, pure Go, no CGO)
      users.go            UserStore interface + SQLiteStore impl — accounts, Telegram, webhook config
    enrich/
      dns.go              Reverse-DNS lookup — 16-worker pool, 300 ms per-host timeout
      netbios.go          NetBIOS Node Status (UDP 137) — fills hostnames DNS missed
      http.go             HTTP banner fingerprinting — Server header + <title> on port 80/8000/8080
      rtsp.go             RTSP OPTIONS probe on port 554 — confirms IP cameras
      os.go               OS inference — TTL rounding + hostname patterns + device type
      mdns.go             ApplyMDNS() — overlays mDNS service data onto scan results
    classify/classify.go  Device category from MAC vendor + open ports (50+ rules)
    vendor/vendor.go      MAC vendor lookup — embedded IEEE OUI database via go:embed
    topology/topology.go  Builds node+edge graph from scan results (star topology)
    diff/diff.go          Compares []Result slices → []Change (NEW / GONE / PORTS / BACK)
    speedtest/speedtest.go  Internet speed test — ping/download/upload via speed.cloudflare.com
    alert/alert.go        Telegram + webhook alerts; Send() fans out to all configured channels
    email/email.go        SMTP email alerts; auto-TLS; no-auth for local relays
    iface/detect.go       Auto-detects LAN interface and subnet
    api/
      server.go           HTTP server, go:embed, SPA fallback, TriggerScan()
      handlers.go         REST handlers
      sse.go              SSE broker — fans out scan events to all connected clients
      static/             Populated at Docker build time from ui/dist (do not commit built files)

ui/                       React + TypeScript + Tailwind PWA
  src/
    App.tsx               Root component — SSE state, device registry, label edits, history navigation
    types.ts              TypeScript mirrors of Go JSON types
    api/client.ts         fetch wrappers for all REST endpoints
    hooks/useEvents.ts    SSE hook with exponential-backoff reconnect
    utils/ports.ts        Port number → service name lookup
    components/           UI components (DeviceCard, DeviceGrid, DeviceHistory, etc.)
```

### Key constraints

- **Raw socket access required** — Linux: `CAP_NET_RAW` + `CAP_NET_ADMIN`; macOS: `sudo`; Windows: Administrator + Npcap.
- **pnet datalink layer** — Linux uses AF_PACKET, macOS uses BPF, Windows uses Npcap.
- **No CGO in Go** — SQLite via `modernc.org/sqlite` (pure Go). No `gcc`, no system libs.
- **Go 1.25+ required** — `modernc.org/sqlite v1.50+` sets this minimum in `go.mod`.
- **Scanner speaks JSON on stdout, errors on stderr** — never mix them.
- **go:embed requires static/ to be non-empty at compile time** — `static/.gitkeep` satisfies this locally.
- **4-stage Dockerfile** — Node (UI) → Rust (scanner) → Go (controller) → debian:bookworm-slim runtime.
- **Store is an interface** — `storage.Store` in `store.go`. `SQLiteStore` is the active backend.
- **mDNS runs in parallel with ARP scan** — zero wall-clock latency penalty.
- **Docker/Podman on Windows does NOT work for scanning** — use native binaries on Windows.

### Enrichment pipeline order

Enrichers run in this exact sequence. Order matters — later enrichers can only fill fields earlier ones left blank:

1. `enrich.Hostnames` — reverse-DNS PTR lookup (16 workers, 300 ms)
2. `enrich.NetBIOSNames` — NetBIOS Node Status UDP 137 (16 workers, 1 s)
3. `vendor.Annotate` — MAC OUI → manufacturer name (pure in-memory, offline)
4. `classify.Annotate` — vendor + ports → device category (50+ rules)
5. `enrich.BannerDeviceType` — HTTP Server header + `<title>` on ports 80/8000/8080 (8 workers, 800 ms)
6. `enrich.SNMPDeviceType` — SNMP sysDescr + sysName via UDP 161 (16 workers, 800 ms)
7. `enrich.RTSPDeviceType` — RTSP OPTIONS on port 554 (8 workers, 600 ms)
8. `enrich.ApplyMDNS` — mDNS service-type overlay (authoritative, runs last before OS)
9. `enrich.AnnotateOS` — TTL rounding + hostname patterns + device type → OS family

### Adding features

- **New scan capability** (e.g. UDP) → add a file in `scanner/src/`, extend `ScanResult` in `types.rs`, add a `Run*` function in `scanner/runner.go`, call it in `main.go`.
- **New enricher** → add a file in `internal/enrich/`, implement `func Foo(results []scanner.Result, workers int, perHost time.Duration)`, call it in `main.go`. See `netbios.go` or `snmp.go` as templates.
- **New classification rule** → edit `portTypes` or `vendorTypes` in `internal/classify/classify.go`. No other changes needed.
- **New alert channel** → add a sender to `internal/alert/alert.go`, call from `alert.Send()`.
- **New API endpoint** → add handler in `api/handlers.go`, register route in `api/server.go`. Use `validPathIP()` for any `{ip}` path params.
- **New settings section** → append to the `SECTIONS` array in `ui/src/components/SettingsPage.tsx`; add GET/PUT/test handlers following the Telegram/webhook pattern.
- **New storage backend** (e.g. Postgres) → implement `storage.Store` (18 methods) and `storage.UserStore` (14 methods); swap `storage.NewSQLite` in `main.go`.

### Storage interfaces

`storage.Store` — 18 methods: `Save`, `Latest`, `LatestRecord`, `History`, `AllKnownIPs`, `AllDevices`, `DeviceHistory`, `SetLabel`, `DeleteLabel`, `GetLabels`, `SetNotify`, `SaveSpeedtest`, `SpeedtestHistory`, `SaveChanges`, `Changes`, `UpdateMACHistory`, `ARPConflicts`, `PruneOldData`.

`storage.UserStore` — 14 methods: `CreateUser`, `FindUserByEmail`, `FindUserByProvider`, `LinkProvider`, `UserCount`, `SaveTelegramConfig`, `GetTelegramConfig`, `GetAllTelegramConfigs`, `SaveWebhookURL`, `GetWebhookURL`, `GetAllWebhookURLs`, `SaveEmailConfig`, `GetEmailConfig`, `GetAllEmailConfigs`.

### SQLite schema migrations

Append `_, _ = db.Exec(...)` calls to `sqliteMigrate()` in `sqlite_store.go`. Always use `CREATE TABLE IF NOT EXISTS` and `ALTER TABLE ... ADD COLUMN`. Never drop columns — existing deployments upgrade automatically on startup.

### Notification opt-in storage

The `device_notify` table stores only overrides from the default. Absence of a row means notifications **disabled**. `COALESCE(n.enabled, 0)` in the `AllDevices` query implements this.

`NEW` and `MAC_CHANGE` events are always sent regardless of per-device preference. `BACK`, `GONE`, and `PORTS` events are sent only if `Notify = true`.

### SMTP password encryption

When `DING_SECRET_KEY` is set, `SaveEmailConfig` encrypts the password with AES-256-GCM (`internal/storage/crypto.go`) before writing. The stored value is prefixed `enc:v1:`. `GetEmailConfig` and `GetAllEmailConfigs` decrypt transparently on read. Values without the prefix are returned as-is (backwards compatible). The key is derived via SHA-256 so any string length works.

### OAuth behind a reverse proxy

The exchange token pattern (`/#exchange=TOKEN` URL fragment → `POST /api/auth/exchange`) works around Cloudflare Tunnel stripping `Set-Cookie` headers from redirect responses. Fragments are browser-only and never forwarded to proxies.

### Cross-platform scanner

pnet uses AF_PACKET on Linux, BPF on macOS, and Npcap on Windows. Interface name resolution uses a 3-step fallback: exact name → description substring match → first non-loopback private-IP interface. Gateway detection uses `/proc/net/route` on Linux and the `default-net` crate on macOS/Windows.

### Android detection limitation

Android 10+ randomises the MAC address per Wi-Fi network, defeating OUI-based vendor detection. Hostname patterns (`android-XXXX`) and Samsung/Google OUI prefix matching are the best available signals without DHCP snooping.

### Development commands

```bash
# Rust (from scanner/)
cargo build --release
cargo test
cargo clippy -- -D warnings

# Go (from controller/)
go build ./...
go test ./...
go vet ./...
go test ./internal/diff/...

# UI (from ui/)
npm install
npm run dev      # Vite dev server on :5173, proxies /api → :8081
npm run build    # output: dist/

# Docker (from repo root)
docker compose build
docker compose up
```
