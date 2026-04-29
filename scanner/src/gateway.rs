// ============================================================
// scanner/src/gateway.rs — Default gateway detection
//
// On Linux: reads /proc/net/route (no extra dependencies).
// On Windows/macOS: uses the `default-net` crate which queries
// the OS routing table via platform-native APIs.
// ============================================================

use anyhow::Result;
use std::net::Ipv4Addr;

/// Detect the default gateway IP for the given network interface.
/// Returns None if no default route is found.
pub fn detect(iface: &str) -> Result<Option<Ipv4Addr>> {
    #[cfg(target_os = "linux")]
    return detect_linux(iface);

    #[cfg(not(target_os = "linux"))]
    {
        let _ = iface; // not needed — default-net returns the system default
        return detect_cross_platform();
    }
}

#[cfg(target_os = "linux")]
fn detect_linux(iface: &str) -> Result<Option<Ipv4Addr>> {
    let content = std::fs::read_to_string("/proc/net/route")?;

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
        if fields[1] != "00000000" {
            continue;
        }
        // Gateway is hex-encoded IPv4 in little-endian byte order
        if let Ok(gw_hex) = u32::from_str_radix(fields[2], 16) {
            let gw = Ipv4Addr::from(gw_hex.to_be());
            return Ok(Some(gw));
        }
    }

    Ok(None)
}

#[cfg(not(target_os = "linux"))]
fn detect_cross_platform() -> Result<Option<Ipv4Addr>> {
    use std::net::IpAddr;
    match default_net::get_default_gateway() {
        Ok(gw) => match gw.ip_addr {
            IpAddr::V4(v4) => Ok(Some(v4)),
            IpAddr::V6(_) => Ok(None),
        },
        Err(_) => Ok(None),
    }
}
