// ============================================================
// scanner/src/tcp.rs — TCP port scanner
//
// For each discovered device, we try to open a TCP connection
// to each port in our list. If the connection succeeds, the
// port is open (some service is listening there).
//
// Default ports probed (passed in from Go via --ports):
//   22   — SSH (remote terminal access)
//   80   — HTTP (web interface)
//   443  — HTTPS (web interface, encrypted)
//   554  — RTSP (IP camera video stream)
//   8000 — HTTP alt (Hikvision camera management)
//   8080 — HTTP alt (QNAP, proxy, NAS)
//   8443 — HTTPS alt
// ============================================================

use anyhow::Result;
use std::net::{SocketAddr, TcpStream};
use std::time::Duration;

use crate::types::ScanResult;

// Try each port on each discovered device and record which ones are open.
// `results` is modified in place — we fill in the `open_ports` field.
pub fn scan_ports(results: &mut [ScanResult], ports: &[u16], timeout_ms: u64) -> Result<()> {
    let timeout = Duration::from_millis(timeout_ms);

    for result in results.iter_mut() {
        for &port in ports {
            // Build the full address, e.g. "192.168.1.42:80"
            let addr: SocketAddr = format!("{}:{}", result.ip, port).parse()?;

            // Try to connect — if it works within the timeout, the port is open.
            // We immediately close the connection; we only care that it opened.
            if TcpStream::connect_timeout(&addr, timeout).is_ok() {
                result.open_ports.push(port);
            }
        }
    }

    Ok(())
}
