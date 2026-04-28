# Ding — Feature Status & Roadmap

## Implemented

| Feature | Where |
|---|---|
| ARP-based device discovery | `scanner/src/arp.rs` |
| ICMP liveness + TTL capture | `scanner/src/ping.rs` |
| TCP port scanning | `scanner/src/tcp.rs` |
| mDNS / Bonjour service discovery | `scanner/src/mdns.rs` |
| Passive ARP listener (instant device detection) | `scanner/src/arp.rs`, `cmd/ding/main.go` |
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
| Port number → service name labels in UI | `ui/src/utils/ports.ts` |
| Email/password authentication (bcrypt, session cookie + bearer token) | `internal/api/handlers.go`, `internal/storage/sqlite_store.go` |
| Google OAuth login | `internal/api/auth.go` |
| GitHub OAuth login | `internal/api/auth.go` |
| React PWA — installable on Android | `ui/` |
| Auto-detect network interface and subnet | `internal/iface/detect.go` |
| Docker multi-arch images (amd64 + arm64) | `Dockerfile` |

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

### Advanced
- **IPv6 support** — ICMPv6 neighbor discovery in Rust; extend `types.rs` for IPv6
- **ARP spoofing detection** — compare MAC-to-IP mappings across scans; alert on conflicts
- **Bandwidth monitoring** — per-device traffic stats (requires iptables or eBPF integration)
- **Configurable scan profiles** — "quick" (ARP only), "deep" (full port range), "stealth" (slow TCP)
- **Postgres / MySQL backend** — implement `storage.Store` interface with a different driver

---

## Technical Notes

- **Adding a new enricher** — create a file in `internal/enrich/`, add the call in `cmd/ding/main.go` after the scanner results are returned. The pipeline runs: DNS → Vendor → Classify → HTTP banner → RTSP → mDNS overlay → OS.
- **Adding a new device rule** — edit `portTypes` or `vendorTypes` in `internal/classify/classify.go`. No other code changes needed.
- **Adding a new alert channel** — add a function to `internal/alert/alert.go` and call it from `alert.Send()`. The changes slice passed in is already pre-filtered to only include devices with notifications enabled.
- **Swapping the storage backend** — implement the `storage.Store` interface (13 methods: `Save`, `Latest`, `LatestRecord`, `History`, `AllKnownIPs`, `AllDevices`, `DeviceHistory`, `SetLabel`, `DeleteLabel`, `GetLabels`, `SetNotify`, `UserStore` methods), then change one line in `cmd/ding/main.go`.
- **Notification opt-in storage** — the `device_notify` table stores only overrides. Absence of a row means notifications disabled (the default). `COALESCE(n.enabled, 0)` in the `AllDevices` query implements this default.
- **OAuth behind a reverse proxy** — the exchange token pattern (`/#exchange=TOKEN` URL fragment) works around Cloudflare Tunnel stripping `Set-Cookie` headers from redirect responses. Fragments are browser-only and never forwarded to proxies.
- **Android detection limitation** — Android 10+ randomizes the MAC address per Wi-Fi network, defeating vendor-based detection. Hostname patterns (`android-XXXX`) and Samsung OUI rules are the best available signals without DHCP snooping.
