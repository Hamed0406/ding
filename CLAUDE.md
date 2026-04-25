# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Ding is a Fing-like network scanner. Rust handles low-level scanning (ARP, ICMP, TCP); Go handles orchestration, storage, alerting, and user-facing surfaces. The two binaries communicate via JSON over stdout — no FFI, no shared memory.

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
  cmd/ding/main.go        Binary entry point — reads env, runs scan, diffs, alerts
  internal/
    scanner/runner.go     Spawns Rust binary, captures stdout, unmarshals JSON
    storage/store.go      JSON-file persistence (no CGO, no external deps)
    diff/diff.go          Compares []Result slices → []Change (NEW / GONE / PORTS)
    alert/alert.go        Telegram HTTP alert (add more channels here)
```

## Commands

### Rust — run from `scanner/`
```bash
cargo build --release        # output: target/release/scanner
cargo test
cargo clippy -- -D warnings
# Run manually (needs CAP_NET_RAW):
sudo ./target/release/scanner --interface eth0 --subnet 192.168.1.0/24
```

### Go — run from `controller/`
```bash
go build ./...
go test ./...
go vet ./...
# Run a single package's tests:
go test ./internal/diff/...
# Run controller locally (scanner binary must be on PATH or DING_SCANNER_BIN set):
DING_SCANNER_BIN=../scanner/target/release/scanner go run ./cmd/ding
```

### Docker — run from repo root
```bash
docker compose build
docker compose up            # requires Linux host
```

## Runtime configuration (env vars)

| Variable | Default | Purpose |
|---|---|---|
| `DING_INTERFACE` | `eth0` | Network interface for ARP/ICMP |
| `DING_SUBNET` | `192.168.1.0/24` | Subnet to scan |
| `DING_PORTS` | `22,80,443,8080,8443` | TCP ports to probe |
| `DING_TIMEOUT_MS` | `500` | Per-host timeout (ms) |
| `DING_DATA_PATH` | `/data/ding.json` | Scan history file |
| `DING_SCANNER_BIN` | `/usr/local/bin/scanner` | Override Rust binary path |
| `DING_TELEGRAM_TOKEN` | _(empty)_ | Telegram bot token |
| `DING_TELEGRAM_CHAT_ID` | _(empty)_ | Telegram chat/channel ID |

## Key constraints

- **Linux only** — `arp.rs` uses `AF_PACKET` raw sockets. Will not work on macOS or Windows.
- **CAP_NET_RAW required** — for both ARP (datalink) and ICMP (transport). In Docker: `cap_add: [NET_RAW, NET_ADMIN]` + `network_mode: host`.
- **pnet uses AF_PACKET, not libpcap** — no libpcap install needed anywhere (build or runtime).
- **No CGO in Go** — storage uses plain JSON files. If SQLite is added later, use `modernc.org/sqlite` (pure Go).
- **Scanner speaks JSON on stdout, errors on stderr** — never mix them. Go reads stdout, logs stderr.
- **Docker-only deployment** — multi-stage Dockerfile builds both binaries from source; never assume host toolchains.

## Data flow

```
main.go
  → scanner.Run()          # exec Rust binary, parse JSON stdout
  → store.Latest()         # read previous scan from ding.json
  → diff.Compare()         # produce []Change
  → store.Save()           # append current scan to ding.json
  → alert.Send()           # fire Telegram if token set
```

## Adding features

- **New scan capability** (e.g. UDP, mDNS) → add a file in `scanner/src/`, call it from `main.rs`, extend `ScanResult` in `types.rs`.
- **New alert channel** (e.g. email, Slack) → add a function in `alert/alert.go`, call it from `alert.Send()`.
- **Scheduling / daemon mode** → implement in Go (`controller/`); the Rust binary is always a one-shot subprocess.
- **REST API** → add under `controller/internal/api/` and wire into `cmd/ding/main.go`.
