# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Ding is a Fing-like network scanner. Rust handles low-level scanning (ARP, ICMP, TCP, mDNS); Go handles orchestration, enrichment, storage, alerting, and serves the React web UI. The two binaries communicate via JSON over stdout — no FFI, no shared memory.

## Directory layout

```
scanner/                  Rust crate — standalone binary
  src/
    main.rs               CLI entry point (clap); three modes: scan, listen, mdns
    arp.rs                ARP discovery + passive ARP listener (AF_PACKET via pnet)
    ping.rs               ICMP liveness check via pnet transport
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
    enrich/
      dns.go              Reverse-DNS lookup — 16-worker pool, 300 ms per-host timeout
      mdns.go             ApplyMDNS() — overlays mDNS service data onto scan results
    classify/classify.go  Device category from MAC vendor + open ports (50+ rules)
    vendor/vendor.go      MAC vendor lookup — embedded IEEE OUI database via go:embed
    topology/topology.go  Builds node+edge graph from scan results (star topology)
    diff/diff.go          Compares []Result slices → []Change (NEW / GONE / PORTS / BACK)
    alert/alert.go        Telegram HTTP alert (add more channels here)
    iface/detect.go       Auto-detects LAN interface and subnet
    api/
      server.go           HTTP server, go:embed, SPA fallback, TriggerScan()
      handlers.go         REST handlers (status, devices, history, topology, scan, labels)
      sse.go              SSE broker — fans out scan events to all connected clients
      static/             Populated at Docker build time from ui/dist (do not commit built files)

ui/                       React + TypeScript + Tailwind PWA
  src/
    App.tsx               Root component — SSE state, device registry, label edits
    types.ts              TypeScript mirrors of Go JSON types
    api/client.ts         fetch wrappers for all REST endpoints
    hooks/useEvents.ts    SSE hook with exponential-backoff reconnect
    utils/ports.ts        Port number → service name lookup (SSH/22, HTTPS/443, etc.)
    components/
      DeviceCard.tsx      Device card with type badge, inline label editor, port pills
      DeviceGrid.tsx      Responsive grid of DeviceCards sorted by IP
      ChangesFeed.tsx     Recent changes feed (NEW / GONE / PORTS / BACK)
      TopologyMap.tsx     SVG network topology map (star layout)
      ScanButton.tsx      Scan trigger button with spinner
      StatusBar.tsx       Header bar showing interface, subnet, last scan time
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

### Docker — run from repo root
```bash
docker compose build
docker compose up        # requires Linux host; UI at http://localhost:8080
```

## Runtime configuration (env vars)

| Variable | Default | Purpose |
|---|---|---|
| `DING_INTERFACE` | _(auto)_ | Network interface for ARP/ICMP/mDNS |
| `DING_SUBNET` | _(auto)_ | Subnet to scan |
| `DING_PORTS` | `22,80,443,8080,8443` | TCP ports to probe |
| `DING_TIMEOUT_MS` | `500` | Per-host timeout for ARP/TCP (ms) |
| `DING_DATA_PATH` | `/data/ding.db` | SQLite database path |
| `DING_HTTP_ADDR` | `:8081` | Web server listen address |
| `DING_SCAN_INTERVAL` | `60s` | Auto-scan interval (`""` = on-demand only) |
| `DING_SCANNER_BIN` | `/usr/local/bin/scanner` | Override Rust binary path |
| `DING_TELEGRAM_TOKEN` | _(empty)_ | Telegram bot token |
| `DING_TELEGRAM_CHAT_ID` | _(empty)_ | Telegram chat/channel ID |

## Key constraints

- **Linux only** — `arp.rs` and `mdns.rs` use raw sockets (AF_PACKET / UDP multicast).
- **CAP_NET_RAW required** — for ARP and ICMP. In Docker: `cap_add: [NET_RAW, NET_ADMIN]` + `network_mode: host`.
- **pnet uses AF_PACKET, not libpcap** — no libpcap needed at build or runtime.
- **No CGO in Go** — SQLite via `modernc.org/sqlite` (pure Go, compiles the SQLite engine in). No `gcc`, no system libs.
- **Go 1.25+ required** — `modernc.org/sqlite v1.50+` sets this minimum in `go.mod`.
- **Scanner speaks JSON on stdout, errors on stderr** — never mix them.
- **go:embed requires static/ to be non-empty at compile time** — `static/.gitkeep` satisfies this locally; the Dockerfile overwrites it with the real UI build.
- **Docker-only deployment** — 4-stage Dockerfile: Node (UI) → Rust (scanner) → Go (controller) → debian:bookworm-slim runtime.
- **Store is an interface** — `storage.Store` in `store.go`. `SQLiteStore` is the active backend. `JSONStore` is kept but unused. Swap backends by changing one line in `main.go`.
- **mDNS runs in parallel with ARP scan** — `scanner.Run()` and `scanner.RunMDNS()` are goroutined together so mDNS adds zero wall-clock latency.

## Data flow

```
main.go
  → starts HTTP server (api.NewServer)
  → TriggerScan() on startup, then on DING_SCAN_INTERVAL ticker
    ┌── scanner.Run()        # ARP+ICMP+TCP scan, parse JSON stdout     ─┐
    └── scanner.RunMDNS()    # mDNS PTR queries, 3 s window             ─┤ parallel
    → enrich.Hostnames()     # reverse-DNS, 16 workers, 300 ms timeout  ←┘
    → vendor.Annotate()      # MAC → manufacturer (embedded OUI DB)
    → classify.Annotate()    # vendor + ports → device category (50+ rules)
    → enrich.ApplyMDNS()     # mDNS service type → device category (authoritative)
    → store.Latest()         # read previous scan from SQLite
    → store.AllKnownIPs()    # all IPs ever seen (for NEW vs BACK detection)
    → diff.Compare()         # produce []Change (NEW / GONE / PORTS / BACK)
    → store.Save()           # write to SQLite (scans + devices tables)
    → alert.Send()           # Telegram if token set
    → broker.Publish()       # push SSE scan_result with store.AllDevices() registry
```

## REST API

| Method | Path | Description |
|---|---|---|
| GET | `/api/status` | Interface, subnet, last scan time |
| GET | `/api/devices` | All known devices (stable registry — alive reflects latest scan) |
| GET | `/api/history` | Last 20 scan records |
| GET | `/api/topology` | Network graph (nodes + edges) |
| POST | `/api/scan` | Trigger a new scan (202 Accepted; results via SSE) |
| PUT | `/api/devices/{ip}/label` | Set a custom name `{"name": "Living Room TV"}` |
| DELETE | `/api/devices/{ip}/label` | Remove a custom name |
| GET | `/api/events` | SSE stream (scan_start / scan_result / scan_error / device_seen) |

## Adding features

- **New scan capability** (e.g. UDP) → add a file in `scanner/src/`, extend `ScanResult` in `types.rs`, add a `Run*` function in `scanner/runner.go`, call it in `main.go`.
- **New enrichment** (e.g. NetBIOS names) → add a file in `internal/enrich/`, call it in `main.go` after the scan.
- **New device classification rules** → edit the `portTypes` or `vendorTypes` tables in `classify/classify.go`.
- **New alert channel** (e.g. Slack) → add to `alert/alert.go`, call from `alert.Send()`.
- **New API endpoint** → add handler in `api/handlers.go`, register route in `api/server.go`.
- **New UI component** → add under `ui/src/components/`, wire into `App.tsx`.
- **Scheduling / daemon mode** → already implemented; tune `DING_SCAN_INTERVAL`.
- **New storage backend** (e.g. Postgres) → implement the `storage.Store` interface (7 methods: `Save`, `Latest`, `LatestRecord`, `History`, `AllKnownIPs`, `AllDevices`, `SetLabel`, `DeleteLabel`, `GetLabels`), then swap `storage.NewSQLite` for your constructor in `main.go`.
