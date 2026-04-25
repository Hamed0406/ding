use anyhow::Result;
use pnet::datalink::{self, Channel::Ethernet, Config};
use pnet::packet::arp::{ArpHardwareTypes, ArpOperations, ArpPacket, MutableArpPacket};
use pnet::packet::ethernet::{EtherTypes, EthernetPacket, MutableEthernetPacket};
use pnet::packet::{MutablePacket, Packet};
use pnet::util::MacAddr;
use std::collections::HashMap;
use std::net::Ipv4Addr;
use std::time::{Duration, Instant};

use crate::types::ScanResult;

pub fn scan(iface_name: &str, hosts: &[Ipv4Addr], timeout_ms: u64) -> Result<Vec<ScanResult>> {
    let interfaces = datalink::interfaces();
    let iface = interfaces
        .into_iter()
        .find(|i| i.name == iface_name)
        .ok_or_else(|| anyhow::anyhow!("interface {} not found", iface_name))?;

    let source_ip = iface
        .ips
        .iter()
        .find(|ip| ip.is_ipv4())
        .map(|ip| match ip.ip() {
            std::net::IpAddr::V4(v4) => v4,
            _ => unreachable!(),
        })
        .ok_or_else(|| anyhow::anyhow!("no IPv4 address on {}", iface_name))?;

    let source_mac = iface
        .mac
        .ok_or_else(|| anyhow::anyhow!("no MAC address on {}", iface_name))?;

    let config = Config {
        read_timeout: Some(Duration::from_millis(50)),
        ..Default::default()
    };

    let (mut tx, mut rx) = match datalink::channel(&iface, config)? {
        Ethernet(tx, rx) => (tx, rx),
        _ => anyhow::bail!("unsupported channel type"),
    };

    for &target_ip in hosts {
        send_arp_request(&mut tx, source_mac, source_ip, target_ip)?;
    }

    let deadline = Instant::now() + Duration::from_millis(timeout_ms);
    let mut discovered: HashMap<Ipv4Addr, String> = HashMap::new();

    while Instant::now() < deadline {
        match rx.next() {
            Ok(frame) => {
                if let Some(eth) = EthernetPacket::new(frame) {
                    if eth.get_ethertype() == EtherTypes::Arp {
                        if let Some(arp) = ArpPacket::new(eth.payload()) {
                            if arp.get_operation() == ArpOperations::Reply {
                                let ip = arp.get_sender_proto_addr();
                                let mac = arp.get_sender_hw_addr().to_string();
                                discovered.insert(ip, mac);
                            }
                        }
                    }
                }
            }
            Err(e) if e.kind() == std::io::ErrorKind::TimedOut
                || e.kind() == std::io::ErrorKind::WouldBlock =>
            {
                continue
            }
            Err(_) => break,
        }
    }

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

fn send_arp_request(
    tx: &mut Box<dyn pnet::datalink::DataLinkSender>,
    source_mac: MacAddr,
    source_ip: Ipv4Addr,
    target_ip: Ipv4Addr,
) -> Result<()> {
    // 14 bytes Ethernet header + 28 bytes ARP payload
    let mut buf = vec![0u8; 42];

    {
        let mut eth = MutableEthernetPacket::new(&mut buf).unwrap();
        eth.set_destination(MacAddr::broadcast());
        eth.set_source(source_mac);
        eth.set_ethertype(EtherTypes::Arp);

        let mut arp = MutableArpPacket::new(eth.payload_mut()).unwrap();
        arp.set_hardware_type(ArpHardwareTypes::Ethernet);
        arp.set_protocol_type(EtherTypes::Ipv4);
        arp.set_hw_addr_len(6);
        arp.set_proto_addr_len(4);
        arp.set_operation(ArpOperations::Request);
        arp.set_sender_hw_addr(source_mac);
        arp.set_sender_proto_addr(source_ip);
        arp.set_target_hw_addr(MacAddr::zero());
        arp.set_target_proto_addr(target_ip);
    }

    tx.send_to(&buf, None);
    Ok(())
}
