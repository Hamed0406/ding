// This file defines the data structure that the scanner produces for each device it finds.
// Rust's `serde` library automatically converts this struct to/from JSON,
// which is how the Go controller reads the results.

use serde::{Deserialize, Serialize};

// ScanResult holds everything we know about one device on the network.
// `Option<String>` means the field might be empty (we couldn't find it).
#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct ScanResult {
    pub ip: String,           // e.g. "192.168.1.42"
    pub mac: Option<String>,  // hardware address, e.g. "aa:bb:cc:dd:ee:ff" (None if unknown)
    pub hostname: Option<String>, // human-readable name (None — not yet implemented)
    pub open_ports: Vec<u16>, // list of open TCP ports, e.g. [22, 80, 443]
    pub alive: bool,          // true if the device responded to ARP or ICMP ping
}

impl ScanResult {
    // Create a blank result for an IP — fields get filled in by arp, ping, and tcp modules.
    pub fn new(ip: String) -> Self {
        Self {
            ip,
            mac: None,
            hostname: None,
            open_ports: Vec::new(),
            alive: false,
        }
    }
}
