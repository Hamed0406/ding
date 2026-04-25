use anyhow::Result;
use pnet::packet::icmp::{
    checksum, echo_request::MutableEchoRequestPacket, IcmpPacket, IcmpTypes,
};
use pnet::packet::ip::IpNextHeaderProtocols;
use pnet::transport::{
    icmp_packet_iter, transport_channel, TransportChannelType::Layer4,
    TransportProtocol::Ipv4,
};
use std::net::IpAddr;
use std::time::Duration;

use crate::types::ScanResult;

pub fn check_alive(results: &mut Vec<ScanResult>, timeout_ms: u64) -> Result<()> {
    let protocol = Layer4(Ipv4(IpNextHeaderProtocols::Icmp));
    let (mut tx, mut rx) = transport_channel(4096, protocol)?;
    let mut iter = icmp_packet_iter(&mut rx);
    let timeout = Duration::from_millis(timeout_ms);

    for result in results.iter_mut() {
        let ip: IpAddr = result.ip.parse()?;

        // Build ICMP echo request (8 bytes: no payload)
        let mut buf = [0u8; 8];
        {
            let mut req = MutableEchoRequestPacket::new(&mut buf).unwrap();
            req.set_icmp_type(IcmpTypes::EchoRequest);
            req.set_sequence_number(1);
            req.set_identifier(std::process::id() as u16);
        }
        let cs = checksum(&IcmpPacket::new(&buf).unwrap());
        {
            let mut req = MutableEchoRequestPacket::new(&mut buf).unwrap();
            req.set_checksum(cs);
            tx.send_to(req, ip)?;
        }

        match iter.next_with_timeout(timeout)? {
            Some((_, addr)) if addr == ip => result.alive = true,
            _ => {}
        }
    }

    Ok(())
}
