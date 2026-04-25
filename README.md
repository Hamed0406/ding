# Ding

A fast network scanner that answers: who is on your network, what are they, and what ports are open. Inspired by [Fing](https://www.fing.com/).

**Architecture:** Rust handles low-level scanning (ARP, ICMP, TCP). Go handles orchestration, change detection, alerting, and storage. Docker ships both binaries — no host dependencies required.

---

## Requirements

- Docker + Docker Compose (Linux host only — ARP scanning requires `AF_PACKET` raw sockets)
- `NET_RAW` / `NET_ADMIN` capabilities (granted automatically via compose)

---

## Quick start

```bash
git clone <repo>
cd ding
```

Find your LAN interface and subnet:

```bash
ip -4 addr show
```

Edit `docker-compose.yml` and set:

```yaml
DING_INTERFACE: eth0          # your LAN interface
DING_SUBNET: 192.168.1.0/24  # your subnet
```

Build and run:

```bash
docker compose build
docker compose up
```

Or run a one-shot scan without compose:

```bash
docker run --rm \
  --network host \
  --cap-add NET_RAW \
  --cap-add NET_ADMIN \
  -v "$(pwd)/data:/data" \
  -e DING_INTERFACE=eth0 \
  -e DING_SUBNET=192.168.1.0/24 \
  ding-ding
```

---

## Example output

```
[NEW] 192.168.1.1 — mac=aa:bb:cc:dd:ee:ff ports=[80 443]
[NEW] 192.168.1.42 — mac=11:22:33:44:55:66 ports=[22]

192.168.1.1       aa:bb:cc:dd:ee:ff     alive=true   ports=[80 443]
192.168.1.42      11:22:33:44:55:66     alive=true   ports=[22]
192.168.1.100     de:ad:be:ef:00:01     alive=true   ports=[]
```

On subsequent runs only changes are printed (`[NEW]`, `[GONE]`, `[PORTS]`). All scans are saved to `./data/ding.json`.

---

## Configuration

All options are set via environment variables.

| Variable | Default | Description |
|---|---|---|
| `DING_INTERFACE` | `eth0` | Network interface to scan on |
| `DING_SUBNET` | `192.168.1.0/24` | Target subnet in CIDR notation |
| `DING_PORTS` | `22,80,443,8080,8443` | TCP ports to probe on each host |
| `DING_TIMEOUT_MS` | `500` | Per-host timeout in milliseconds |
| `DING_DATA_PATH` | `/data/ding.json` | Where scan history is stored |
| `DING_SCANNER_BIN` | `/usr/local/bin/scanner` | Path to Rust scanner binary |
| `DING_TELEGRAM_TOKEN` | _(empty)_ | Telegram bot token for alerts |
| `DING_TELEGRAM_CHAT_ID` | _(empty)_ | Telegram chat or channel ID |

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

## Running continuously

`docker-compose.yml` sets `restart: unless-stopped`, so the container restarts after each scan. To add a delay between scans, wrap the entrypoint in a loop:

```yaml
entrypoint: ["/bin/sh", "-c", "while true; do /usr/local/bin/ding; sleep 60; done"]
```

---

## Scan data

Results are stored in `./data/ding.json` (mounted into the container). The file holds the last 100 scans in JSON format and is human-readable:

```json
[
  {
    "scanned_at": "2026-04-25T10:00:00Z",
    "results": [
      { "ip": "192.168.1.1", "mac": "aa:bb:cc:dd:ee:ff", "open_ports": [80, 443], "alive": true }
    ]
  }
]
```

---

## How it works

```
docker run
  └── Go controller (ding)
        ├── spawns Rust binary (scanner) as subprocess
        │     ├── ARP broadcast → discovers IPs + MACs
        │     ├── ICMP echo    → confirms liveness
        │     └── TCP connect  → finds open ports
        │     └── prints JSON to stdout
        ├── diffs results against last scan → detects NEW / GONE / PORTS changes
        ├── saves results to /data/ding.json
        └── sends Telegram alert (if configured)
```

---

## Building from source

Rust and Go toolchains are not required on the host — the multi-stage Dockerfile handles everything.

```bash
docker compose build
```

To build locally (requires Rust ≥ 1.75 and Go ≥ 1.22):

```bash
# Rust scanner
cd scanner && cargo build --release

# Go controller
cd controller && go build ./cmd/ding
```
