# Ding — Feature Status & Roadmap

## Implemented

| Feature | Where |
|---|---|
| ARP-based device discovery | `scanner/src/arp.rs` |
| ICMP liveness + TTL capture | `scanner/src/ping.rs` |
| TCP port scanning | `scanner/src/tcp.rs` |
| mDNS / Bonjour service discovery | `scanner/src/mdns.rs` |
| Passive ARP listener (instant device detection) | `scanner/src/arp.rs`, `cmd/ding/main.go` |
| Cross-platform scanner (Linux / macOS / Windows) | `scanner/src/arp.rs`, `scanner/src/gateway.rs` |
| Reverse-DNS hostname lookup (16 workers, 300 ms) | `internal/enrich/dns.go` |
| NetBIOS Node Status enricher (UDP 137, fills gaps DNS misses) | `internal/enrich/netbios.go` |
| MAC vendor lookup (IEEE OUI, embedded, offline) | `internal/vendor/vendor.go` |
| Device type classification — 50+ vendor + port rules | `internal/classify/classify.go` |
| HTTP banner fingerprinting (Server header + `<title>`) | `internal/enrich/http.go` |
| SNMP sysDescr + sysName probe (UDP 161, community "public") | `internal/enrich/snmp.go` |
| RTSP OPTIONS probe on port 554 | `internal/enrich/rtsp.go` |
| OS fingerprinting (TTL + hostname + device type) | `internal/enrich/os.go` |
| mDNS service-type overlay (authoritative device categories) | `internal/enrich/mdns.go` |
| Scan-to-scan diff: NEW / BACK / GONE / PORTS | `internal/diff/diff.go` |
| Persistent change log (SQLite-backed, survives page reload) | `internal/storage/sqlite_store.go`, `ui/src/components/ChangeLog.tsx` |
| SQLite scan history (normalized, per-device queries) | `internal/storage/sqlite_store.go` |
| First-seen / last-seen timestamps per device | `internal/storage/sqlite_store.go`, `ui/src/components/DeviceCard.tsx` |
| Device labelling (custom names, persisted in SQLite) | `internal/api/handlers.go` |
| Per-device port scan (instant, no full network scan) | `internal/api/handlers.go` |
| Wake-on-LAN (magic packet via UDP broadcast) | `internal/api/handlers.go` |
| Per-device notification opt-in (bell toggle) | `internal/storage/sqlite_store.go`, `internal/api/handlers.go` |
| Telegram alerts — per-user token + chat ID | `internal/alert/alert.go`, `internal/storage/users.go` |
| Webhook alerts (Slack / Discord / ntfy.sh compatible) | `internal/alert/alert.go`, `internal/storage/users.go` |
| Internet speed test (ping / download / upload via Cloudflare) | `internal/speedtest/speedtest.go`, `ui/src/components/SpeedTest.tsx` |
| CSV / JSON device export (browser download) | `internal/api/handlers.go`, `ui/src/App.tsx` |
| Network topology map (SVG star layout) | `internal/topology/topology.go`, `ui/src/components/TopologyMap.tsx` |
| Per-device history view (dot timeline, scan log) | `ui/src/components/DeviceHistory.tsx` |
| Events view — filterable change log (NEW/BACK/GONE/PORTS) | `ui/src/components/ChangeLog.tsx` |
| Device search and status filter (online / offline) | `ui/src/App.tsx` |
| Real-time SSE event stream | `internal/api/sse.go` |
| Port number → service name mapping | `ui/src/utils/ports.ts` |
| Email/password authentication (bcrypt, session cookie + bearer token) | `internal/api/handlers.go`, `internal/storage/sqlite_store.go` |
| Google OAuth login | `internal/api/server.go` |
| GitHub OAuth login | `internal/api/server.go` |
| React PWA — installable on Android / desktop | `ui/` |
| Full-page Settings UI (Telegram, Webhooks, Speed test) | `ui/src/components/SettingsPage.tsx` |
| Auto-detect network interface and subnet | `internal/iface/detect.go` |
| Docker multi-arch images (linux/amd64 + linux/arm64) | `Dockerfile`, `.github/workflows/release.yml` |
| Podman support (podman-compose + Quadlet) | `docker-compose.yml` |
| Native binary releases (Linux, macOS, Windows — 5 platforms) | `.github/workflows/release.yml` |
| GitHub Actions CI + release pipeline | `.github/workflows/ci.yml`, `.github/workflows/release.yml` |

---

## Roadmap

### Quick wins

- **Port service labels on device cards** — display "SSH · HTTP · HTTPS" instead of "22 · 80 · 443". The mapping already exists in `ui/src/utils/ports.ts`; just render it on `DeviceCard.tsx`.
- **Dark / light mode toggle** — Tailwind `dark:` variants; preference stored in `localStorage`.
- **Alert cooldown / dedup** — suppress repeated GONE alerts for a device that flaps; configurable quiet period per device.
- **Quiet hours for alerts** — time-window rule: "don't send Telegram/webhook alerts between 11pm–7am".

### Medium complexity

- **Device uptime / availability stats** — per-device alive% over last 7 / 30 days; data already in SQLite scan history. Show as "99.1% uptime (7d)" on device card and a dot-graph in the history view.
- **Device groups / tags** — let users create named groups ("IoT", "Servers", "Cameras") and assign devices. One new `device_groups` table, tag pills on device cards, group filter in the search bar.
- **Ping latency history** — store ICMP RTT per scan (Rust already measures it), show a sparkline on the device card and a latency chart in the history view.
- **Multi-subnet scanning** — accept comma-separated `DING_SUBNET` values; fan out scanner goroutines and merge results.
- **DHCP hostname snooping** — listen on UDP 67/68 or parse `/proc/net/arp` to capture hostnames Android and IoT devices advertise in DHCP Option 12.
- **Settings page: scheduled scan config** — UI control for `DING_SCAN_INTERVAL` without restarting the container; stored in SQLite and re-read at runtime.

### Advanced

- **Smart alert rules engine** — user-defined conditions evaluated after each diff:
  - "Alert if device X is offline for more than N minutes"
  - "Alert only if a NEW unknown device appears between 11pm–6am"
  - "Alert if port 22 opens on any device without a label"
  - One `alert_rules` table, a rule evaluator in Go, a rule-builder UI in Settings.

- **Network security audit** — post-scan scoring against a risk checklist:
  - Open Telnet (port 23) or FTP (port 21) — unencrypted protocols
  - HTTP admin panel with no HTTPS (80/8080 open, no 443)
  - Unidentified device (no hostname, no vendor, no classification)
  - Device appeared for the first time (potential intruder)
  - Show a green / amber / red security badge on each device card and a summary banner: "3 devices need attention." Pure logic on existing scan data — no new network probes needed.

- **Browser push notifications (Web Push)** — the app is already a PWA. Adding Web Push lets the browser notify users of new devices even when the tab is closed:
  - Generate a VAPID key pair on first run; store in SQLite.
  - Store `PushSubscription` per user-device in SQLite.
  - After each scan, fan out push messages alongside Telegram / webhooks.
  - Works on desktop Chrome/Firefox and Android — no Telegram bot needed.

- **IPv6 support** — ICMPv6 neighbor discovery in Rust; extend `types.rs` and `ScanResult` for IPv6 addresses; dual-stack enrichment pipeline.

- **ARP spoofing detection** — compare MAC-to-IP mappings across consecutive scans; alert when the same IP is claimed by a different MAC (classic ARP poisoning signal).

- **Bandwidth monitoring** — per-device traffic stats; requires iptables conntrack, eBPF, or a SNMP ifInOctets / ifOutOctets poll (OIDs 1.3.6.1.2.1.2.2.1.10 / .16). The SNMP stack is already in place.

- **Configurable scan profiles** — named profiles: "quick" (ARP only, no port scan), "standard" (default), "deep" (full 1–65535 port range), "stealth" (slow TCP with randomised timing). Stored in SQLite; selectable from the UI before triggering a scan.

- **Postgres / MySQL backend** — implement `storage.Store` (15 methods) and `storage.UserStore` (11 methods) with a different driver; swap the constructor in `main.go`. No other changes needed.

- **macOS / Windows installer** — bundle `scanner` + `ding` into a `.dmg` (macOS) or `.msi` / WiX installer (Windows) with Npcap bundled, so non-technical users don't need a terminal.

- **Home Assistant / MQTT integration** — publish device presence events (`ding/devices/<ip>/state = online|offline`) to an MQTT broker after each scan; enables HA automations triggered by device arrival/departure.

---

## Technical Notes

### Enrichment pipeline order
Enrichers run in this sequence in `cmd/ding/main.go`. Order matters — later enrichers can only fill fields the earlier ones left blank:

1. `enrich.Hostnames` — reverse-DNS PTR lookup (16 workers, 300 ms)
2. `enrich.NetBIOSNames` — NetBIOS Node Status UDP 137 (16 workers, 1 s) — fills gaps DNS misses
3. `vendor.Annotate` — MAC OUI → manufacturer name (pure in-memory, offline)
4. `classify.Annotate` — vendor + ports → device category (50+ rules)
5. `enrich.BannerDeviceType` — HTTP Server header + `<title>` on ports 80/8000/8080 (8 workers, 800 ms)
6. `enrich.SNMPDeviceType` — SNMP sysDescr + sysName via UDP 161 (16 workers, 800 ms)
7. `enrich.RTSPDeviceType` — RTSP OPTIONS on port 554 (8 workers, 600 ms)
8. `enrich.ApplyMDNS` — mDNS service-type overlay (authoritative, runs last before OS)
9. `enrich.AnnotateOS` — TTL rounding + hostname patterns + device type → OS family

### Adding a new enricher
Create a file in `internal/enrich/`, implement a function with signature `func Foo(results []scanner.Result, workers int, perHost time.Duration)`, and call it in `cmd/ding/main.go` after the scanner returns. See `netbios.go` or `snmp.go` as templates.

### Adding a new device classification rule
Edit `portTypes` or `vendorTypes` in `internal/classify/classify.go`. No other code changes needed.

### Adding a new alert channel
Add a sender function to `internal/alert/alert.go` and call it from `alert.Send()`. The `changes` slice passed to `Send()` is already filtered by `main.go` using these rules:
- `KindNew` — always included (first-ever appearance; no per-device preference exists yet)
- `KindMACChange` — always included (security event)
- All other kinds — included only if the device has `Notify = true`

### Adding a new settings section
Append to the `SECTIONS` array in `ui/src/components/SettingsPage.tsx` and add a matching panel component below. Follow the Telegram or Webhook section as a template (GET/PUT/test handler pattern).

### Storage interface
`storage.Store` has 15 methods: `Save`, `Latest`, `LatestRecord`, `History`, `AllKnownIPs`, `AllDevices`, `DeviceHistory`, `SetLabel`, `DeleteLabel`, `GetLabels`, `SetNotify`, `SaveSpeedtest`, `SpeedtestHistory`, `SaveChanges`, `Changes`.

`storage.UserStore` has 11 methods: `CreateUser`, `FindUserByEmail`, `FindUserByProvider`, `LinkProvider`, `UserCount`, `SaveTelegramConfig`, `GetTelegramConfig`, `GetAllTelegramConfigs`, `SaveWebhookURL`, `GetWebhookURL`, `GetAllWebhookURLs`.

Both are implemented by `*SQLiteStore`. Swap backends by changing one constructor call in `main.go`.

### SQLite schema migrations
Append `_, _ = db.Exec(...)` calls to `sqliteMigrate()` in `sqlite_store.go`. Use `CREATE TABLE IF NOT EXISTS` and `ALTER TABLE ... ADD COLUMN` (errors are intentionally ignored for existing columns). Never drop columns — existing deployments upgrade automatically on startup.

### Notification opt-in storage
The `device_notify` table stores only overrides from the default. Absence of a row means notifications **disabled**. `COALESCE(n.enabled, 0)` in the `AllDevices` query implements this.

This per-device flag only controls `BACK`, `GONE`, and `PORTS` events. `NEW` and `MAC_CHANGE` events are always sent regardless — see the alert filtering logic in `cmd/ding/main.go`.

### Per-user config storage
Telegram token/chat ID and webhook URL are columns on the `users` table. `GetAllTelegramConfigs()` and `GetAllWebhookURLs()` collect all configured users at alert time; fall back to `DING_TELEGRAM_TOKEN` env var if none are set.

### OAuth behind a reverse proxy
The exchange token pattern (`/#exchange=TOKEN` URL fragment → `POST /api/auth/exchange`) works around Cloudflare Tunnel stripping `Set-Cookie` headers from redirect responses. Fragments are browser-only and never forwarded to proxies.

### Cross-platform scanner
pnet uses AF_PACKET on Linux, BPF on macOS, and Npcap on Windows. Interface name resolution uses a 3-step fallback: exact name → description substring match → first non-loopback private-IP interface. Gateway detection uses `/proc/net/route` on Linux and the `default-net` crate on macOS/Windows.

### Windows build requirement
Set `LIB=<npcap-sdk>\Lib\x64` before `cargo build` on Windows so pnet can link against `wpcap`. The CI pipeline downloads the Npcap SDK automatically. End-users need the Npcap runtime installed separately.

### Docker/Podman on Windows limitation
Containers run inside a Linux VM; `network_mode: host` attaches the VM's virtual NIC, not the host's physical NIC. ARP scanning cannot reach the real LAN. Use native Windows binaries instead.

### Android detection limitation
Android 10+ randomises the MAC address per Wi-Fi network, defeating OUI-based vendor detection. Hostname patterns (`android-XXXX`) and Samsung/Google OUI prefix matching are the best available signals without DHCP snooping.

### Cutting a release
```bash
git tag v1.2.3 && git push origin v1.2.3
```
The release pipeline builds Docker images (linux/amd64 + arm64) and native binaries for 5 platforms (Linux amd64/arm64, macOS Intel/ARM, Windows amd64), packages them as `.tar.gz` / `.zip` archives with SHA-256 checksums, and creates a GitHub Release with auto-generated notes. Tags containing a `-` (e.g. `v1.2.0-beta.1`) are automatically marked as pre-releases.
