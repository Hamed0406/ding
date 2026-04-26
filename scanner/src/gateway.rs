// ============================================================
// scanner/src/gateway.rs — Default gateway detection
//
// Reads /proc/net/route to find the default gateway for a given
// network interface. The default route has destination 00000000.
// The gateway IP is stored in hex (little-endian on x86).
//
// This is Linux-only, which matches the rest of the scanner
// (AF_PACKET raw sockets are also Linux-only).
// ============================================================

use anyhow::Result;
use std::fs;
use std::net::Ipv4Addr;

/// Detect the default gateway IP for the given network interface.
/// Returns None if no default route is found for this interface.
pub fn detect(iface: &str) -> Result<Option<Ipv4Addr>> {
    let content = fs::read_to_string("/proc/net/route")?;

    // Each line: Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT
    // Default route has Destination = 00000000
    for line in content.lines().skip(1) {
        let fields: Vec<&str> = line.split_whitespace().collect();
        if fields.len() < 3 {
            continue;
        }
        if fields[0] != iface {
            continue;
        }
        // Default route: destination is 0.0.0.0
        if fields[1] != "00000000" {
            continue;
        }
        // Gateway is a hex-encoded IPv4 in little-endian byte order
        if let Ok(gw_hex) = u32::from_str_radix(fields[2], 16) {
            let gw = Ipv4Addr::from(gw_hex.to_be());
            return Ok(Some(gw));
        }
    }

    Ok(None)
}
