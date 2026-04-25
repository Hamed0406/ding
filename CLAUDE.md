# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Ding is a Fing-like network scanner. Rust handles low-level scanning (ARP, ICMP, TCP); Go handles orchestration, storage, alerting, and serves the React web UI. The two binaries communicate via JSON over stdout — no FFI, no shared memory.

## Directory layout

```
scanner/                  Rust crate — standalone binary
  src/
    main.rs               CLI entry point (clap), subnet parsing, scan orchestration
    arp.rs                ARP discovery via pnet datalink (AF_PACKET)
    ping.rs               ICMP liveness check via pnet transport
    tcp.rs                TCP port scan via std::net::TcpStream::connect_timeout
    types.rs              ScanResult struct (serde Serialize/Deserialize)

controller/               Go module (github.com/ding/ding)
  cmd/ding/main.go        Binary entry point — config, HTTP server, scan loop
  internal/
    scanner/runner.go     Spawns Rust binary, captures stdout, unmarshals JSON
    storage/store.go      JSON-file persistence (no CGO, no external deps)
    diff/diff.go          Compares []Result slices → []Change (NEW / GONE / PORTS)
    alert/alert.go        Telegram HTTP alert (add more channels here)
    iface/detect.go       Auto-detects LAN interface and subnet
    api/
      server.go           HTTP server, go:embed, SPA fallback, TriggerScan()
      handlers.go         REST handlers (/api/status, /api/devices, /api/history, /api/scan)
      sse.go              SSE broker — fans out scan events to all connected clients
      static/             Populated at Docker build time from ui/dist (do not commit built files)

ui/                       React + TypeScript + Tailwind PWA
  src/
    App.tsx               Top-level component, SSE state management
    types.ts              TypeScript mirrors of Go JSON types
    api/client.ts         fetch wrappers for all API endpoints
    hooks/useEvents.ts    SSE hook with exponential-backoff reconnect
    components/           DeviceCard, DeviceGrid, ChangesFeed, ScanButton, StatusBar
```

## Commands

### Rust — run from `scanner/`
```bash
cargo build --release        # output: target/release/scanner
cargo test
cargo clippy -- -D warnings
sudo ./target/release/scanner --interface eth0 --subnet 192.168.1.0/24
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

### Docker — run from repo root
```bash
docker compose build
docker compose up        # requires Linux host; UI at http://localhost:8080
```

## Runtime configuration (env vars)

| Variable | Default | Purpose |
|---|---|---|
| `DING_INTERFACE` | _(auto)_ | Network interface for ARP/ICMP |
| `DING_SUBNET` | _(auto)_ | Subnet to scan |
| `DING_PORTS` | `22,80,443,8080,8443` | TCP ports to probe |
| `DING_TIMEOUT_MS` | `500` | Per-host timeout (ms) |
| `DING_DATA_PATH` | `/data/ding.json` | Scan history file |
| `DING_HTTP_ADDR` | `:8081` | Web server listen address |
| `DING_SCAN_INTERVAL` | `60s` | Auto-scan interval (`""` = on-demand only) |
| `DING_SCANNER_BIN` | `/usr/local/bin/scanner` | Override Rust binary path |
| `DING_TELEGRAM_TOKEN` | _(empty)_ | Telegram bot token |
| `DING_TELEGRAM_CHAT_ID` | _(empty)_ | Telegram chat/channel ID |

## Key constraints

- **Linux only** — `arp.rs` uses `AF_PACKET` raw sockets.
- **CAP_NET_RAW required** — for ARP and ICMP. In Docker: `cap_add: [NET_RAW, NET_ADMIN]` + `network_mode: host`.
- **pnet uses AF_PACKET, not libpcap** — no libpcap needed at build or runtime.
- **No CGO in Go** — storage uses plain JSON files.
- **Scanner speaks JSON on stdout, errors on stderr** — never mix them.
- **go:embed requires static/ to be non-empty at compile time** — `static/.gitkeep` satisfies this locally; the Dockerfile overwrites it with the real UI build.
- **Docker-only deployment** — 4-stage Dockerfile: Node (UI) → Rust (scanner) → Go (controller) → debian:bookworm-slim runtime.

## Data flow

```
main.go
  → starts HTTP server (api.NewServer)
  → TriggerScan() on startup, then on DING_SCAN_INTERVAL ticker
    → scanner.Run()       # exec Rust binary, parse JSON stdout
    → store.Latest()      # read previous scan
    → diff.Compare()      # produce []Change
    → store.Save()        # append to ding.json
    → alert.Send()        # Telegram if token set
    → broker.Publish()    # push SSE events to all connected clients
```

## Adding features

- **New scan capability** (e.g. UDP, mDNS) → add a file in `scanner/src/`, extend `ScanResult` in `types.rs`.
- **New alert channel** (e.g. Slack) → add to `alert/alert.go`, call from `alert.Send()`.
- **New API endpoint** → add handler in `api/handlers.go`, register route in `api/server.go`.
- **New UI page** → add component under `ui/src/components/`, wire into `App.tsx`.
- **Scheduling / daemon mode** → already implemented; tune `DING_SCAN_INTERVAL`.
