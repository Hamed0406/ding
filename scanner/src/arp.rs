// ============================================================
// scanner/src/arp.rs — ARP-based device discovery
//
// ARP (Address Resolution Protocol) is how devices on a local
// network find each other's hardware (MAC) addresses.
//
// How it works:
//   1. We broadcast "Who has IP x.x.x.x? Tell me at my MAC."
//   2. If a device with that IP exists, it replies with its MAC.
//   3. We collect all replies within the timeout window.
//
// This uses raw sockets (AF_PACKET) via the `pnet` library.
// That's why the container needs CAP_NET_RAW permission.
// ============================================================

use anyhow::Result;
use pnet::datalink::{self, Channel::Ethernet, Config};
use pnet::packet::arp::{ArpHardwareTypes, ArpOperations, ArpPacket, MutableArpPacket};
use pnet::packet::ethernet::{EtherTypes, EthernetPacket, MutableEthernetPacket};
use pnet::packet::{MutablePacket, Packet};
use pnet::util::MacAddr;
use std::collections::HashMap;
use std::io::{self, Write};
use std::net::Ipv4Addr;
use std::time::{Duration, Instant};

use crate::types::{ArpEvent, ScanResult};

// Scan the network for all active devices by sending ARP requests.
// Returns only devices that replied — silent devices are excluded.
pub fn scan(iface_name: &str, hosts: &[Ipv4Addr], timeout_ms: u64) -> Result<Vec<ScanResult>> {
    // Find the network interface object by name (e.g. "eth0")
    let interfaces = datalink::interfaces();
    let iface = interfaces
        .into_iter()
        .find(|i| i.name == iface_name)
        .ok_or_else(|| anyhow::anyhow!("interface {} not found", iface_name))?;

    // Get our own IP address — we put it in the ARP request as the sender
    let source_ip = iface
        .ips
        .iter()
        .find(|ip| ip.is_ipv4())
        .map(|ip| match ip.ip() {
            std::net::IpAddr::V4(v4) => v4,
            _ => unreachable!(),
        })
        .ok_or_else(|| anyhow::anyhow!("no IPv4 address on {}", iface_name))?;

    // Get our own MAC address — also goes into the ARP request
    let source_mac = iface
        .mac
        .ok_or_else(|| anyhow::anyhow!("no MAC address on {}", iface_name))?;

    // Open a raw Ethernet channel on the interface.
    // read_timeout: how long to wait for each packet before trying again.
    let config = Config {
        read_timeout: Some(Duration::from_millis(50)),
        ..Default::default()
    };

    // tx = transmit (send packets), rx = receive (read packets)
    let (mut tx, mut rx) = match datalink::channel(&iface, config)? {
        Ethernet(tx, rx) => (tx, rx),
        _ => anyhow::bail!("unsupported channel type"),
    };

    // Fire off an ARP request for every IP in the subnet (very fast, just sending)
    for &target_ip in hosts {
        send_arp_request(&mut tx, source_mac, source_ip, target_ip)?;
    }

    // Now listen for replies until the timeout expires
    let deadline = Instant::now() + Duration::from_millis(timeout_ms);
    let mut discovered: HashMap<Ipv4Addr, String> = HashMap::new(); // IP → MAC

    while Instant::now() < deadline {
        match rx.next() {
            Ok(frame) => {
                // Parse the raw bytes as an Ethernet frame
                if let Some(eth) = EthernetPacket::new(frame) {
                    // Only care about ARP packets (ignore everything else)
                    if eth.get_ethertype() == EtherTypes::Arp {
                        if let Some(arp) = ArpPacket::new(eth.payload()) {
                            // Only care about replies (not requests from other machines)
                            if arp.get_operation() == ArpOperations::Reply {
                                let ip = arp.get_sender_proto_addr();
                                let mac = arp.get_sender_hw_addr().to_string();
                                discovered.insert(ip, mac);
                            }
                        }
                    }
                }
            }
            // read_timeout fired — that's normal, just loop and check deadline
            Err(e)
                if e.kind() == std::io::ErrorKind::TimedOut
                    || e.kind() == std::io::ErrorKind::WouldBlock =>
            {
                continue
            }
            // Real error — stop listening
            Err(_) => break,
        }
    }

    // Build ScanResult entries only for hosts that actually replied
    let results = hosts
        .iter()
        .filter_map(|&ip| {
            discovered.get(&ip).map(|mac| {
                let mut r = ScanResult::new(ip.to_string());
                r.mac = Some(mac.clone());
                r.alive = true;
                r
            })
        })
        .collect();

    Ok(results)
}

// Passively listen for ARP traffic on the interface and stream events to stdout.
// Each event is one JSON line: {"ip":"...","mac":"..."}.
// Runs forever — the Go controller kills this process on shutdown.
// Captures both ARP requests and replies, so devices are detected the moment
// they send any ARP packet (on connect, DHCP renewal, or gateway ping).
pub fn listen(iface_name: &str) -> Result<()> {
    let interfaces = datalink::interfaces();
    let iface = interfaces
        .into_iter()
        .find(|i| i.name == iface_name)
        .ok_or_else(|| anyhow::anyhow!("interface {} not found", iface_name))?;

    let source_mac = iface
        .mac
        .ok_or_else(|| anyhow::anyhow!("no MAC address on {}", iface_name))?;

    // Short read timeout so the loop stays responsive without spinning at 100% CPU.
    let config = Config {
        read_timeout: Some(Duration::from_millis(100)),
        ..Default::default()
    };

    let (_tx, mut rx) = match datalink::channel(&iface, config)? {
        Ethernet(tx, rx) => (tx, rx),
        _ => anyhow::bail!("unsupported channel type"),
    };

    let stdout = io::stdout();

    loop {
        match rx.next() {
            Ok(frame) => {
                if let Some(eth) = EthernetPacket::new(frame) {
                    if eth.get_ethertype() != EtherTypes::Arp {
                        continue;
                    }
                    if let Some(arp) = ArpPacket::new(eth.payload()) {
                        let sender_mac = arp.get_sender_hw_addr();
                        // Ignore our own interface, broadcast, and zero MACs
                        if sender_mac == source_mac
                            || sender_mac == MacAddr::broadcast()
                            || sender_mac == MacAddr::zero()
                        {
                            continue;
                        }
                        let ip = arp.get_sender_proto_addr();
                        // 0.0.0.0 = ARP probe during DHCP — skip, address not yet assigned
                        if ip.is_unspecified() {
                            continue;
                        }
                        let event = ArpEvent {
                            ip: ip.to_string(),
                            mac: sender_mac.to_string(),
                        };
                        if let Ok(json) = serde_json::to_string(&event) {
                            let mut out = stdout.lock();
                            let _ = writeln!(out, "{}", json);
                            let _ = out.flush(); // flush immediately — Go reads line by line
                        }
                    }
                }
            }
            // Read timeout — normal, just loop again
            Err(e)
                if e.kind() == io::ErrorKind::TimedOut
                    || e.kind() == io::ErrorKind::WouldBlock =>
            {
                continue;
            }
            // Any other error — keep running (interface blip, etc.)
            Err(_) => continue,
        }
    }
}

// Build and send a single ARP request packet asking "Who has `target_ip`?"
fn send_arp_request(
    tx: &mut Box<dyn pnet::datalink::DataLinkSender>,
    source_mac: MacAddr,
    source_ip: Ipv4Addr,
    target_ip: Ipv4Addr,
) -> Result<()> {
    // An Ethernet frame is 14 bytes of header + 28 bytes of ARP = 42 bytes total
    let mut buf = vec![0u8; 42];

    {
        // Outer layer: Ethernet frame addressed to broadcast (ff:ff:ff:ff:ff:ff)
        // so every device on the network receives it
        let mut eth = MutableEthernetPacket::new(&mut buf).unwrap();
        eth.set_destination(MacAddr::broadcast()); // send to everyone
        eth.set_source(source_mac); // from us
        eth.set_ethertype(EtherTypes::Arp); // payload type = ARP

        // Inner layer: the actual ARP request
        let mut arp = MutableArpPacket::new(eth.payload_mut()).unwrap();
        arp.set_hardware_type(ArpHardwareTypes::Ethernet); // we're on Ethernet
        arp.set_protocol_type(EtherTypes::Ipv4); // asking about IPv4
        arp.set_hw_addr_len(6); // MAC = 6 bytes
        arp.set_proto_addr_len(4); // IPv4 = 4 bytes
        arp.set_operation(ArpOperations::Request); // this is a question
        arp.set_sender_hw_addr(source_mac); // our MAC
        arp.set_sender_proto_addr(source_ip); // our IP
        arp.set_target_hw_addr(MacAddr::zero()); // unknown (that's what we're asking)
        arp.set_target_proto_addr(target_ip); // the IP we're asking about
    }

    // Send the raw bytes out on the wire
    tx.send_to(&buf, None);
    Ok(())
}
