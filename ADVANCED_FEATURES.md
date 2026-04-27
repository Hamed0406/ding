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
| Network topology map (SVG star layout) | `internal/topology/topology.go`, `ui/src/components/TopologyMap.tsx` |
| Per-device history view (dot timeline, scan log) | `ui/src/components/DeviceHistory.tsx` |
| Real-time SSE event stream | `internal/api/sse.go` |
| Telegram alerts (NEW / GONE / PORTS / BACK) | `internal/alert/alert.go` |
| Port number → service name labels in UI | `ui/src/utils/ports.ts` |
| React PWA — installable on Android | `ui/` |
| Auto-detect network interface and subnet | `internal/iface/detect.go` |
| Docker multi-arch images (amd64 + arm64) | `Dockerfile` |

---

## Potential Next Features

### Quick wins
- **Email / Slack alerts** — add alongside Telegram in `internal/alert/alert.go`
- **Dark mode** — Tailwind `dark:` variants; toggle stored in `localStorage`
- **CSV / JSON export** — new handler in `internal/api/handlers.go`
- **Custom alert rules** — alert only on specific IPs or port changes

### Medium complexity
- **Uptime graphs** — per-device availability % over time; data already in SQLite
- **Multi-subnet scanning** — accept comma-separated `DING_SUBNET` values
- **API authentication** — HTTP Basic or bearer token middleware in `internal/api/server.go`
- **NetBIOS / LLMNR names** — additional enricher alongside DNS lookup
- **Wake-on-LAN** — new API endpoint; sends magic packet via `net.UDPConn`

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
- **Adding a new alert channel** — add a function to `internal/alert/alert.go` and call it from `alert.Send()`.
- **Swapping the storage backend** — implement the `storage.Store` interface (10 methods), then change one line in `cmd/ding/main.go`.
- **Android detection limitation** — Android 10+ randomizes the MAC address per Wi-Fi network, defeating vendor-based detection. Hostname patterns (`android-XXXX`) and Samsung OUI rules are the best available signals without DHCP snooping.
