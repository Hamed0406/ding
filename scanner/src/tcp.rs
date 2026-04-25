use anyhow::Result;
use std::net::{SocketAddr, TcpStream};
use std::time::Duration;

use crate::types::ScanResult;

pub fn scan_ports(results: &mut Vec<ScanResult>, ports: &[u16], timeout_ms: u64) -> Result<()> {
    let timeout = Duration::from_millis(timeout_ms);

    for result in results.iter_mut() {
        for &port in ports {
            let addr: SocketAddr = format!("{}:{}", result.ip, port).parse()?;
            if TcpStream::connect_timeout(&addr, timeout).is_ok() {
                result.open_ports.push(port);
            }
        }
    }

    Ok(())
}
