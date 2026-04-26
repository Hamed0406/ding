// ============================================================
// scanner/src/ping.rs — ICMP ping (liveness check)
//
// After ARP discovers devices, we send an ICMP "echo request"
// (the same thing the `ping` command does) to each one.
// If the device replies, we mark it as alive=true.
//
// This uses a raw ICMP transport socket via `pnet`.
// No system `ping` binary is needed.
// ============================================================

use anyhow::Result;
use pnet::packet::icmp::{checksum, echo_request::MutableEchoRequestPacket, IcmpPacket, IcmpTypes};
use pnet::packet::ip::IpNextHeaderProtocols;
use pnet::transport::{
    icmp_packet_iter, transport_channel, TransportChannelType::Layer4, TransportProtocol::Ipv4,
};
use std::net::IpAddr;
use std::time::Duration;

use crate::types::ScanResult;

// Send an ICMP ping to each device and mark it alive if it replies.
// `results` is modified in place — we update the `alive` field.
pub fn check_alive(results: &mut [ScanResult], timeout_ms: u64) -> Result<()> {
    // Open a raw ICMP socket — Layer4 means we handle ICMP ourselves
    let protocol = Layer4(Ipv4(IpNextHeaderProtocols::Icmp));
    let (mut tx, mut rx) = transport_channel(4096, protocol)?;

    // An iterator that reads incoming ICMP packets from the socket
    let mut iter = icmp_packet_iter(&mut rx);
    let timeout = Duration::from_millis(timeout_ms);

    for result in results.iter_mut() {
        let ip: IpAddr = result.ip.parse()?;

        // Build an ICMP echo request packet (8 bytes — the minimum size)
        let mut buf = [0u8; 8];
        {
            let mut req = MutableEchoRequestPacket::new(&mut buf).unwrap();
            req.set_icmp_type(IcmpTypes::EchoRequest); // type 8 = "ping"
            req.set_sequence_number(1);
            req.set_identifier(std::process::id() as u16); // use our process ID to identify replies
        }

        // ICMP requires a checksum so the receiver can verify the packet wasn't corrupted.
        // We compute it over the packet bytes we just built.
        let cs = checksum(&IcmpPacket::new(&buf).unwrap());
        {
            let mut req = MutableEchoRequestPacket::new(&mut buf).unwrap();
            req.set_checksum(cs);
            tx.send_to(req, ip)?; // send the ping
        }

        // Wait up to `timeout` for an ICMP reply from exactly this IP
        match iter.next_with_timeout(timeout)? {
            Some((_, addr)) if addr == ip => result.alive = true, // got a reply → alive!
            _ => {} // no reply within timeout → stays alive=false
        }
    }

    Ok(())
}
