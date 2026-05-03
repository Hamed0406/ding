# Ding

A fast network scanner that answers: who is on your network, what are they, and what ports are open.

**Architecture:** Rust handles low-level scanning (ARP, ICMP, TCP, mDNS). Go handles orchestration, enrichment, change detection, alerting, storage, and serves the web UI. The two communicate via JSON over stdout — no FFI, no shared memory.

---

## Install

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

Pick your platform:

- [Linux — Docker / Docker Compose](#linux--docker--docker-compose-recommended)
- [Linux — native binary](#linux--native-binary)
- [Linux — Podman](#linux--podman)
- [Raspberry Pi](#raspberry-pi)
- [macOS](#macos)
- [Windows](#windows)

> **Why Docker on Windows won't work for scanning:** Docker Desktop runs inside a Linux VM. `network_mode: host` attaches the VM's virtual NIC, not your real Wi-Fi/Ethernet adapter — ARP packets never reach your LAN. Use the native Windows binary instead.

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

Open **http://<pi-ip>:8081** from any device on your network.

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

Or create a `.env`-style shell script:

```bash
#!/bin/sh
export DING_SCAN_INTERVAL=60s
export DING_HTTP_ADDR=:8081
export DING_DATA_PATH=/usr/local/var/ding/ding.db
exec sudo -E /usr/local/bin/ding
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

> The plist runs the binary; the binary itself uses raw sockets which need root — macOS will prompt for your password on first launch.

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

Open PowerShell and run:

```powershell
Get-NetAdapter | Select-Object Name, InterfaceDescription, Status
```

Note the **Name** of your active adapter (e.g. `Wi-Fi`, `Ethernet`).

**3. Run as Administrator**

Right-click PowerShell → **Run as Administrator**, then:

```powershell
cd C:\ding
$env:DING_INTERFACE    = "Wi-Fi"      # replace with your adapter name
$env:DING_SCAN_INTERVAL= "60s"
$env:DING_HTTP_ADDR    = ":8081"
.\ding.exe
```

Open **http://localhost:8081**.

**4. Run as a Windows Service (auto-start on boot)**

Use the built-in `sc` command or [NSSM](https://nssm.cc) (Non-Sucking Service Manager):

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

If you want to access the UI from another machine on the network, allow inbound traffic on port 8081:

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
| **Notification opt-in** | Bell icon on each card — off by default; click to enable alerts per device. Brand-new devices (first-ever appearance) always alert regardless of this setting |
| **Topology map** | Interactive SVG star-topology map; switch between Grid and Topology views |
| **Changes feed** | NEW / GONE / BACK / PORTS changes with colour coding |
| **Passive detection** | Devices that send ARP traffic appear instantly without waiting for a scan |
| **Authentication** | Email/password + Google and GitHub OAuth; session persists via HttpOnly cookie and bearer token |
| **Telegram alerts** | Each user stores their own Telegram bot token and chat ID; alerts fire per-device when the bell is enabled |
| **Email alerts** | SMTP-based alerts — supports Gmail, Outlook, Yahoo, iCloud, Fastmail, Brevo, or any self-hosted relay (Postfix, Mailpit); configured per-user in Settings → Email |
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
| `DING_SECRET_KEY` | _(empty)_ | Encrypts SMTP passwords stored in SQLite (AES-256-GCM). Generate with `openssl rand -base64 32`. If unset, passwords are stored in plaintext and a warning is logged. See [Protecting your SMTP password](#protecting-your-smtp-password-ding_secret_key). |
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

# Persistent change log (last 200 events, newest first)
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/changes

# ARP watch — IPs seen with more than one MAC (spoofing candidates)
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/arpwatch

# Export all devices as CSV (default) or JSON
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/devices/export
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/devices/export?format=json"

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

Ding sends alerts when a tracked device joins, leaves, or changes ports.

**Two alert tiers:**

| Tier | When | Opt-out? |
|---|---|---|
| **Always-on** | Brand-new device (first time ever seen on your network) | No — you always want to know |
| **Always-on** | ARP MAC change on a known IP (possible ARP spoofing) | No — security event |
| **Per-device** | Device returns, goes offline, or changes open ports | Yes — bell icon on each card |

Per-device notifications are **disabled by default**. Enable them by clicking the bell icon on a device card (turns cyan when active).

### Telegram

**Two ways to configure:**

1. **Per-user (recommended):** Log in → gear icon → Settings → Notifications. Each user's token and chat ID are stored in their account in the database.
2. **Global fallback:** Set `DING_TELEGRAM_TOKEN` and `DING_TELEGRAM_CHAT_ID` environment variables. Used only when no per-user config exists.

**Setup:**

1. Create a bot via [@BotFather](https://t.me/BotFather) on Telegram and copy the token.
2. Get your chat ID from [@userinfobot](https://t.me/userinfobot).
3. Enter them in Settings → Notifications and click **Send test message** to confirm.
4. Enable the bell icon on each device you want to track.

### Email

Settings → Email → configure your SMTP provider. Two modes:

**External (Gmail, Outlook, Yahoo, iCloud, Fastmail, Brevo, or any SMTP)**

Pick your provider from the preset dropdown — host and port are filled in automatically, and a hint shows how to get an App Password for that provider. Then enter your username, App Password, From address, and the address to send alerts to.

**Self-hosted (Postfix, Mailpit, or any local relay)**

Set host to `localhost` (or the relay hostname), port to `25` (Postfix) or `1025` (Mailpit), and leave username/password blank if no auth is required.

> **Testing with Mailpit:** Mailpit is a local mail catcher — it accepts all SMTP but never delivers to real inboxes. Use it to verify Ding's email plumbing before switching to a real provider. Add it alongside Ding with:
> ```bash
> docker compose --profile mailpit up -d
> ```
> Then configure host=`localhost`, port=`1025`. Caught emails appear in Mailpit's web UI at **http://\<host\>:8025**.

Click **Save**, then **Send test email** to confirm delivery before relying on it for alerts.

#### Protecting your SMTP password (`DING_SECRET_KEY`)

By default Ding stores your SMTP password in the SQLite database in plain text. If someone copies the database file they can read it.

Setting `DING_SECRET_KEY` tells Ding to encrypt the password with AES-256-GCM before saving it. The key never leaves your server.

**Step 1 — generate a key** (run once, keep it safe):

```bash
openssl rand -base64 32
# example output: 4X3mK9vPqRzL2YwN8TdJcHbF7sAeUiGo1nQxZyCpVkW=
```

**Step 2 — add it to your `.env`** (Docker) or environment:

```bash
DING_SECRET_KEY=4X3mK9vPqRzL2YwN8TdJcHbF7sAeUiGo1nQxZyCpVkW=
```

**Step 3 — restart Ding.** Re-open Settings → Email, re-enter your SMTP password, and click Save. The password is now stored encrypted.

> **Already have a saved password?**  
> Existing plain-text passwords keep working after you set `DING_SECRET_KEY` — Ding detects unencrypted values automatically. However, they remain unencrypted in the DB until you re-save them through the Settings UI.

> **Lost the key?**  
> Without the key, stored passwords cannot be decrypted. Ding will log an error and email alerts will stop. Re-enter your SMTP password in Settings → Email after restoring the key.

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
  └── sends Telegram + email + webhook alerts (per-user config; NEW device always; bell-enabled devices for BACK/GONE/PORTS)

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
| `user_email_config` | Per-user SMTP settings (host, port, credentials, from/to addresses) |
| `speedtest_results` | Historical internet speed test results |

Schema migrations run automatically on startup — no manual steps needed when upgrading. No external database service is required; the SQLite engine is compiled into the binary.
