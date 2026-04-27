// This file defines the data structures the scanner produces.
// Rust's `serde` library automatically converts them to/from JSON,
// which is how the Go controller reads the results.

use serde::{Deserialize, Serialize};

// ScanResult holds everything we know about one device on the network.
// `Option<String>` means the field might be empty (we couldn't find it).
#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct ScanResult {
    pub ip: String,               // e.g. "192.168.1.42"
    pub mac: Option<String>,      // hardware address, e.g. "aa:bb:cc:dd:ee:ff" (None if unknown)
    pub hostname: Option<String>, // human-readable name (None — not yet implemented)
    pub open_ports: Vec<u16>,     // list of open TCP ports, e.g. [22, 80, 443]
    pub alive: bool,              // true if the device responded to ARP or ICMP ping
    pub gateway: Option<String>,  // default gateway IP, e.g. "192.168.1.1" (same for all hosts)
    pub ttl: Option<u8>, // TTL from ICMP reply — Go controller rounds this to infer OS (64→Linux/macOS/Android/iOS, 128→Windows)
}

// ArpEvent is emitted by the scanner in --mode listen (one JSON line per event).
// It carries only the information visible in a passive ARP packet — IP and MAC.
// Port data is not available until a full active scan runs.
#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct ArpEvent {
    pub ip: String,  // sender IP from the ARP packet
    pub mac: String, // sender MAC from the ARP packet
}

// MdnsEvent is emitted by --mode mdns, one JSON line per resolved service.
// It pairs an IP address with the mDNS service type the device advertises and
// the friendly instance name the device chose for itself.
#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct MdnsEvent {
    pub ip: String,      // e.g. "192.168.1.5"
    pub service: String, // e.g. "_googlecast._tcp"
    pub name: String,    // e.g. "Bedroom TV"
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
            gateway: None,
            ttl: None,
        }
    }
}
