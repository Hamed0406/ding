# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Ding is a Fing-like network scanner. Rust handles low-level scanning (ARP, ICMP, TCP, mDNS); Go handles orchestration, enrichment, storage, alerting, and serves the React web UI. The two binaries communicate via JSON over stdout — no FFI, no shared memory.

## Directory layout

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
      netbios.go          NetBIOS Node Status (UDP 137) — fills hostnames DNS missed (Windows PCs, NAS, printers)
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
    email/email.go        SMTP email alerts; auto-TLS (port 465=implicit TLS, else STARTTLS); no-auth for local relays
    iface/detect.go       Auto-detects LAN interface and subnet
    api/
      server.go           HTTP server, go:embed, SPA fallback, TriggerScan()
      handlers.go         REST handlers (status, devices, history, topology, scan, labels, settings, speedtest)
      sse.go              SSE broker — fans out scan events to all connected clients
      static/             Populated at Docker build time from ui/dist (do not commit built files)

ui/                       React + TypeScript + Tailwind PWA
  src/
    App.tsx               Root component — SSE state, device registry, label edits, history navigation
    types.ts              TypeScript mirrors of Go JSON types
    api/client.ts         fetch wrappers for all REST endpoints
    hooks/useEvents.ts    SSE hook with exponential-backoff reconnect
    utils/ports.ts        Port number → service name lookup (SSH/22, HTTPS/443, etc.)
    components/
      DeviceCard.tsx      Clickable device card — type badge, inline label editor, port pills, first/last-seen; click → history
      DeviceGrid.tsx      Responsive grid of DeviceCards sorted by IP
      DeviceHistory.tsx   Full-page history view — dot timeline, stats, scan log with port-change markers
      ChangesFeed.tsx     Recent changes feed (NEW / GONE / PORTS / BACK)
      TopologyMap.tsx     SVG network topology map (star layout)
      ScanButton.tsx      Scan trigger button with spinner
      StatusBar.tsx       Header bar showing interface, subnet, last scan time
      SpeedTest.tsx       On-demand internet speed test — ping/download/upload via Cloudflare, history table
```

## Commands

### Rust — run from `scanner/`
```bash
cargo build --release        # output: target/release/scanner
cargo test
cargo clippy -- -D warnings
sudo ./target/release/scanner --interface eth0 --subnet 192.168.1.0/24
sudo ./target/release/scanner --interface eth0 --mode listen
sudo ./target/release/scanner --interface eth0 --mode mdns --timeout-ms 3000
```

### Go — run from `controller/`
```bash
go build ./...
go test ./...
go vet ./...
go test ./internal/diff/...          # single package
# Run locally (static/ must contain ui/dist first):
cp -r ../ui/dist ./internal/api/static/
DING_SCANNER_BIN=../scanner/target/release/scanner go run ./cmd/ding
```

### UI — run from `ui/`
```bash
npm install
npm run dev      # Vite dev server on :5173, proxies /api → :8080
npm run build    # output: dist/ (copy to controller/internal/api/static/ for local Go build)
```

### Docker / Podman — run from repo root
```bash
docker compose build
docker compose up        # requires Linux host (network_mode: host); UI at http://localhost:8081
# Podman users:
sudo podman-compose up --build
```

### Release — cut a new version
```bash
git tag v1.2.3 && git push origin v1.2.3
# GitHub Actions builds Docker (linux/amd64 + arm64) + native binaries for
# Linux, macOS, Windows and publishes them as GitHub Release assets.
```

## Runtime configuration (env vars)

| Variable | Default | Purpose |
|---|---|---|
| `DING_INTERFACE` | _(auto)_ | Network interface for ARP/ICMP/mDNS |
| `DING_SUBNET` | _(auto)_ | Subnet to scan |
| `DING_PORTS` | `22,80,443,554,8000,8080,8443` | TCP ports to probe |
| `DING_TIMEOUT_MS` | `500` | Per-host timeout for ARP/TCP (ms) |
| `DING_DATA_PATH` | `/data/ding.db` | SQLite database path |
| `DING_HTTP_ADDR` | `:8081` | Web server listen address |
| `DING_SCAN_INTERVAL` | `60s` | Auto-scan interval (`""` = on-demand only) |
| `DING_SCANNER_BIN` | `/usr/local/bin/scanner` | Override Rust binary path |
| `DING_TELEGRAM_TOKEN` | _(empty)_ | Global fallback Telegram bot token (per-user config takes precedence) |
| `DING_TELEGRAM_CHAT_ID` | _(empty)_ | Global fallback Telegram chat/channel ID |
| `DING_SECRET_KEY` | _(empty)_ | Passphrase for AES-256-GCM encryption of SMTP passwords in SQLite. Generate: `openssl rand -base64 32`. If unset, passwords stored in plaintext (warning logged). Encrypted values prefixed `enc:v1:`; plain values pass through unchanged (backwards compatible). See `internal/storage/crypto.go`. |

## Key constraints

- **Raw socket access required** — ARP and ICMP need elevated privileges on every OS:
  - Linux: `CAP_NET_RAW` + `CAP_NET_ADMIN`; in Docker: `cap_add: [NET_RAW, NET_ADMIN]` + `network_mode: host`
  - macOS: `sudo` (BPF access)
  - Windows: Administrator + [Npcap](https://npcap.com) installed
- **pnet datalink layer** — Linux uses AF_PACKET (no libpcap needed), macOS uses BPF, Windows uses Npcap. The Rust code is the same across all three; only the runtime driver differs.
- **Windows build requires Npcap SDK** — set `LIB=<npcap-sdk>\Lib\x64` before `cargo build`. The release pipeline downloads the SDK automatically.
- **Docker/Podman on Windows does NOT work for scanning** — containers run inside a Linux VM; `network_mode: host` gives the VM's virtual NIC, not the real LAN. Use native binaries on Windows instead.
- **No CGO in Go** — SQLite via `modernc.org/sqlite` (pure Go, compiles the SQLite engine in). No `gcc`, no system libs.
- **Go 1.25+ required** — `modernc.org/sqlite v1.50+` sets this minimum in `go.mod`.
- **Scanner speaks JSON on stdout, errors on stderr** — never mix them.
- **go:embed requires static/ to be non-empty at compile time** — `static/.gitkeep` satisfies this locally; the Dockerfile overwrites it with the real UI build.
- **4-stage Dockerfile** — Node (UI) → Rust (scanner) → Go (controller) → debian:bookworm-slim runtime. Docker images target Linux only; native binaries for macOS and Windows are released separately via GitHub Actions.
- **Store is an interface** — `storage.Store` in `store.go`. `SQLiteStore` is the active backend. `JSONStore` is kept but unused. Swap backends by changing one line in `main.go`.
- **mDNS runs in parallel with ARP scan** — `scanner.Run()` and `scanner.RunMDNS()` are goroutined together so mDNS adds zero wall-clock latency.

## Data flow

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

## REST API

| Method | Path | Description |
|---|---|---|
| GET | `/api/status` | Interface, subnet, last scan time |
| GET | `/api/devices` | All known devices (stable registry — alive reflects latest scan) |
| GET | `/api/devices/{ip}/history` | Last 100 scans for one device (oldest first) — alive + ports per scan |
| GET | `/api/history` | Last 20 full scan records |
| GET | `/api/topology` | Network graph (nodes + edges) |
| POST | `/api/scan` | Trigger a new scan (202 Accepted; results via SSE) |
| POST | `/api/devices/{ip}/scan` | Scan one device's ports immediately |
| POST | `/api/devices/{ip}/wake` | Send Wake-on-LAN magic packet |
| PUT | `/api/devices/{ip}/label` | Set a custom name `{"name": "Living Room TV"}` |
| DELETE | `/api/devices/{ip}/label` | Remove a custom name |
| PUT | `/api/devices/{ip}/notify` | Toggle per-device alerts `{"enabled": true}` |
| GET | `/api/settings/telegram` | Get current user's Telegram config |
| PUT | `/api/settings/telegram` | Save current user's Telegram token + chat ID |
| POST | `/api/settings/telegram/test` | Send a test Telegram message |
| GET | `/api/settings/email` | Get current user's SMTP email config (password masked) |
| PUT | `/api/settings/email` | Save current user's SMTP config (empty password = keep existing) |
| POST | `/api/settings/email/test` | Send a test email via saved SMTP config |
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

## Adding features

- **New scan capability** (e.g. UDP) → add a file in `scanner/src/`, extend `ScanResult` in `types.rs`, add a `Run*` function in `scanner/runner.go`, call it in `main.go`.
- **New scan capability** (e.g. UDP) → add a file in `scanner/src/`, extend `ScanResult` in `types.rs`, add a `Run*` function in `scanner/runner.go`, call it in `main.go`.
- **New enrichment** → add a file in `internal/enrich/`, call it in `main.go` after existing enrichers. See `netbios.go` as a template.
- **New device classification rules** → edit the `portTypes` or `vendorTypes` tables in `classify/classify.go`.
- **New alert channel** (e.g. Slack native) → add a sender in `alert/alert.go`, add its config to `Config`, call from `Send()`.
- **New API endpoint** → add handler in `api/handlers.go`, register route in `api/server.go`.
- **New settings section** → add a panel to `SettingsPage.tsx`, add GET/PUT/test handlers + routes following the Telegram/webhook pattern.
- **New UI component** → add under `ui/src/components/`, wire into `App.tsx`.
- **Scheduling / daemon mode** → already implemented; tune `DING_SCAN_INTERVAL`.
- **New storage backend** (e.g. Postgres) → implement `storage.Store` (18 methods: `Save`, `Latest`, `LatestRecord`, `History`, `AllKnownIPs`, `AllDevices`, `DeviceHistory`, `SetLabel`, `DeleteLabel`, `GetLabels`, `SetNotify`, `SaveSpeedtest`, `SpeedtestHistory`, `SaveChanges`, `Changes`, `UpdateMACHistory`, `ARPConflicts`, `PruneOldData`) and `storage.UserStore` (14 methods: `CreateUser`, `FindUserByEmail`, `FindUserByProvider`, `LinkProvider`, `UserCount`, `SaveTelegramConfig`, `GetTelegramConfig`, `GetAllTelegramConfigs`, `SaveWebhookURL`, `GetWebhookURL`, `GetAllWebhookURLs`, `SaveEmailConfig`, `GetEmailConfig`, `GetAllEmailConfigs`), then swap `storage.NewSQLite` for your constructor in `main.go`.
- **SQLite schema migrations** → append `_, _ = db.Exec(...)` calls to `sqliteMigrate()` in `sqlite_store.go`; existing databases are migrated automatically on startup. Never use `CREATE TABLE` without `IF NOT EXISTS` and never drop columns.
