# Advanced Feature Suggestions for Ding

## Quick Wins ⭐
1. **Reverse DNS lookup** — Show device hostnames (not just IPs)
2. **MAC vendor lookup** — Identify device type from MAC prefix (e.g., "Apple Inc.", "Cisco")
3. **Device naming/tagging** — Let users label devices ("Living Room TV", "Mom's Laptop") for easier tracking
4. **Dark mode** — UI improvement, saves battery on mobile
5. **Export scans** — CSV/JSON export for auditing or sharing results

## Medium Complexity 🔧
6. **Custom alert rules** — Alert only on specific devices or port changes (not everything)
7. **Email/Slack alerts** — Beyond just Telegram
8. **Uptime tracking** — Show device availability % over time (graphs)
9. **Port service identification** — Map port 22→SSH, 80→HTTP, etc. (via `/etc/services`)
10. **Multi-subnet scanning** — Scan multiple networks simultaneously
11. **Bandwidth monitoring** — Per-device traffic stats (requires iptables/netstat integration)
12. **API authentication** — Secure the API endpoints with basic auth or tokens

## Advanced 🚀
13. **Wake-on-LAN (WoL)** — Power on/off devices from the UI
14. **Network topology visualization** — Visual map of device relationships
15. **ARP spoofing detection** — Security feature to alert on suspicious ARP activity
16. **IPv6 support** — Currently IPv4-only (ICMPv6, IPv6 neighbor discovery)
17. **Device fingerprinting** — OS detection based on TTL, open ports, TCP window size
18. **Concurrent port scanning** — Speed up TCP scans with goroutines
19. **Configurable scan profiles** — "Quick scan", "Deep scan", "Stealth scan"
20. **Historical analytics** — Trends, device churn, port change patterns

---

## Implementation Notes

### Priority Recommendations
- **High ROI**: Reverse DNS, MAC vendor lookup, device naming, dark mode (quick to implement, high value)
- **Medium Effort**: Alert rules, uptime tracking, port service IDs (moderate complexity, great UX)
- **Long-term**: Fingerprinting, topology viz, IPv6 (complex, require architectural changes)

### Technical Considerations
- **DNS lookups**: Use Go's `net.LookupAddr()` (non-blocking)
- **MAC vendor DB**: Embed OUI database (MAC prefix → vendor) or fetch from API (nvlpubs.nist.gov/oui)
- **Device persistence**: Extend `ding.json` schema to store user labels + first/last seen timestamps
- **Uptime graphs**: Use Recharts in React, calculate availability from historical scans
- **Alert rules**: Add JSON config format or UI builder for rule conditions
- **OAuth/JWT**: Simple token auth in Go, store in environment or config file
- **Concurrent scanning**: Goroutine pool with `semaphore` pattern or `golang.org/x/sync/semaphore`
- **IPv6**: Add ICMPv6 in Rust, extend types.rs for IPv6 addresses

### Estimated Effort
- Quick wins: 1-2 hours each
- Medium features: 4-8 hours each
- Advanced: 8-20+ hours each

---

## User Questions to Consider
- **Use case**: Home lab? Security monitoring? IoT tracking? Device discovery?
- **Priority**: UX improvements? Better visibility? Automation? Security? Performance?
- **Scale**: Single subnet or enterprise multi-subnet?
- **Integrations**: Which alert channels matter most?
