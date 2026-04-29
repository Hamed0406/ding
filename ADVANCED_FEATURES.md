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
| Reverse-DNS hostname lookup | `internal/enrich/dns.go` |
| MAC vendor lookup (IEEE OUI, embedded, offline) | `internal/vendor/vendor.go` |
| Device type classification — 50+ vendor + port rules | `internal/classify/classify.go` |
| HTTP banner fingerprinting (Server header + `<title>`) | `internal/enrich/http.go` |
| RTSP OPTIONS probe on port 554 | `internal/enrich/rtsp.go` |
| OS fingerprinting (TTL + hostname + device type) | `internal/enrich/os.go` |
| mDNS service-type overlay (authoritative device categories) | `internal/enrich/mdns.go` |
| Scan-to-scan diff: NEW / BACK / GONE / PORTS | `internal/diff/diff.go` |
| SQLite scan history (normalized, per-device queries) | `internal/storage/sqlite_store.go` |
| Device labelling (custom names, persisted in SQLite) | `internal/api/handlers.go` |
| Per-device port scan (instant, no full network scan) | `internal/api/handlers.go` |
| Wake-on-LAN (magic packet via UDP broadcast) | `internal/api/handlers.go` |
| Per-device notification opt-in (bell toggle, disabled by default) | `internal/storage/sqlite_store.go`, `internal/api/handlers.go` |
| Network topology map (SVG star layout) | `internal/topology/topology.go`, `ui/src/components/TopologyMap.tsx` |
| Per-device history view (dot timeline, scan log) | `ui/src/components/DeviceHistory.tsx` |
| Device search and status filter (online / offline) | `ui/src/App.tsx` |
| Real-time SSE event stream | `internal/api/sse.go` |
| Telegram alerts — opt-in per device | `internal/alert/alert.go`, `internal/storage/sqlite_store.go` |
| Per-user Telegram config (token + chat ID stored per account) | `internal/storage/users.go`, `internal/api/handlers.go` |
| Port number → service name labels in UI | `ui/src/utils/ports.ts` |
| Email/password authentication (bcrypt, session cookie + bearer token) | `internal/api/handlers.go`, `internal/storage/sqlite_store.go` |
| Google OAuth login | `internal/api/auth.go` |
| GitHub OAuth login | `internal/api/auth.go` |
| React PWA — installable on Android | `ui/` |
| Full-page Settings UI with sidebar navigation | `ui/src/components/SettingsPage.tsx` |
| Auto-detect network interface and subnet | `internal/iface/detect.go` |
| Docker multi-arch images (linux/amd64 + linux/arm64) | `Dockerfile`, `.github/workflows/release.yml` |
| Podman support (podman-compose + Quadlet) | `docker-compose.yml` |
| Native binary releases (Linux, macOS, Windows) | `.github/workflows/release.yml` |
| GitHub Actions CI + release pipeline | `.github/workflows/ci.yml`, `.github/workflows/release.yml` |

---

## Potential Next Features

### Quick wins
- **Email / Slack alerts** — add alongside Telegram in `internal/alert/alert.go`
- **CSV / JSON export** — new handler in `internal/api/handlers.go`
- **Dark / light mode toggle** — Tailwind `dark:` variants; preference stored in `localStorage`

### Medium complexity
- **Uptime graphs** — per-device availability % over time; data already in SQLite
- **Multi-subnet scanning** — accept comma-separated `DING_SUBNET` values
- **NetBIOS / LLMNR names** — additional enricher alongside DNS lookup
- **DHCP hostname snooping** — read `/proc/net/arp` or listen on UDP 67 for DHCP hostnames
- **Alert cooldown / dedup** — suppress repeated GONE alerts for a device that flaps
- **Settings page: additional sections** — the sidebar in `SettingsPage.tsx` is built for extensibility; add entries to `SECTIONS` and a matching panel

### Advanced
- **IPv6 support** — ICMPv6 neighbor discovery in Rust; extend `types.rs` for IPv6
- **ARP spoofing detection** — compare MAC-to-IP mappings across scans; alert on conflicts
- **Bandwidth monitoring** — per-device traffic stats (requires iptables or eBPF integration)
- **Configurable scan profiles** — "quick" (ARP only), "deep" (full port range), "stealth" (slow TCP)
- **Postgres / MySQL backend** — implement `storage.Store` interface with a different driver
- **macOS / Windows installer** — bundle scanner + ding into a `.dmg` / `.msi` with Npcap bundled

---

## Technical Notes

- **Adding a new enricher** — create a file in `internal/enrich/`, add the call in `cmd/ding/main.go` after the scanner results are returned. Pipeline order: DNS → Vendor → Classify → HTTP banner → RTSP → mDNS overlay → OS.
- **Adding a new device rule** — edit `portTypes` or `vendorTypes` in `internal/classify/classify.go`. No other code changes needed.
- **Adding a new alert channel** — add a function to `internal/alert/alert.go` and call it from `alert.Send()`. The changes slice is already pre-filtered to devices with notifications enabled.
- **Adding a new settings section** — append to the `SECTIONS` array in `ui/src/components/SettingsPage.tsx` and add a matching panel component below.
- **Swapping the storage backend** — implement the `storage.Store` interface (13 methods: `Save`, `Latest`, `LatestRecord`, `History`, `AllKnownIPs`, `AllDevices`, `DeviceHistory`, `SetLabel`, `DeleteLabel`, `GetLabels`, `SetNotify`, plus `UserStore` methods), then change one line in `cmd/ding/main.go`.
- **Notification opt-in storage** — the `device_notify` table stores only overrides. Absence of a row means notifications disabled (the default). `COALESCE(n.enabled, 0)` in the `AllDevices` query implements this.
- **Per-user Telegram config** — stored as `telegram_token` + `telegram_chat_id` columns on the `users` table. `GetAllTelegramConfigs()` fetches all configured users at alert time; falls back to the `DING_TELEGRAM_TOKEN` env var if none are configured.
- **OAuth behind a reverse proxy** — the exchange token pattern (`/#exchange=TOKEN` URL fragment) works around Cloudflare Tunnel stripping `Set-Cookie` headers from redirect responses. Fragments are browser-only and never forwarded to proxies.
- **Cross-platform scanner** — pnet uses AF_PACKET on Linux, BPF on macOS, and Npcap on Windows. Interface name resolution uses a 3-step fallback: exact name → description substring match → first non-loopback private-IP interface. Gateway detection uses `/proc/net/route` on Linux and `default-net` crate on macOS/Windows.
- **Windows build requirement** — set `LIB=<npcap-sdk>\Lib\x64` before `cargo build` on Windows so pnet can link against `wpcap`. The CI pipeline downloads the Npcap SDK automatically. End-users need Npcap runtime installed.
- **Docker/Podman on Windows limitation** — containers run inside a Linux VM; `network_mode: host` attaches the VM's virtual NIC, not the host's physical NIC. ARP scanning cannot reach the real LAN. Use native Windows binaries instead.
- **Android detection limitation** — Android 10+ randomizes the MAC address per Wi-Fi network, defeating vendor-based detection. Hostname patterns (`android-XXXX`) and Samsung OUI rules are the best available signals without DHCP snooping.
- **Cutting a release** — `git tag v1.2.3 && git push origin v1.2.3`. The release pipeline builds Docker images (linux/amd64 + arm64) and native binaries for 5 platforms (Linux amd64/arm64, macOS Intel/ARM, Windows amd64), packages them as archives with SHA-256 checksums, and creates a GitHub Release.
