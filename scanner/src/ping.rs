// ============================================================
// scanner/src/ping.rs — ICMP ping (liveness + TTL fingerprinting)
//
// Sends an ICMP echo request to each discovered device and waits
// for a reply. Two things come out of a successful reply:
//
//   alive = true
//   ttl   = the TTL value from the IPv4 header of the reply
//
// Why TTL matters: every OS starts packets with a fixed initial TTL.
// The Go controller rounds the received TTL up to the nearest
// standard value to infer the OS family (Linux=64, Windows=128).
//
// Implementation note — why socket2 instead of pnet transport:
//   pnet's icmp_packet_iter operates at Layer 4 and strips the IPv4
//   header before returning packets. The TTL lives in that header, so
//   it becomes inaccessible. By using a raw SOCK_RAW/IPPROTO_ICMP
//   socket via socket2, the kernel includes the full IPv4 header in
//   every received buffer, letting us read TTL directly.
//   The kernel still adds the IPv4 header for us on send, so we only
//   need to construct the 8-byte ICMP payload.
// ============================================================

use anyhow::Result;
use pnet::packet::icmp::{checksum, echo_request::MutableEchoRequestPacket, IcmpPacket, IcmpTypes};
use pnet::packet::ipv4::Ipv4Packet;
use socket2::{Domain, Protocol, Socket, Type};
use std::mem::MaybeUninit;
use std::net::{Ipv4Addr, SocketAddrV4};
use std::time::{Duration, Instant};

use crate::types::ScanResult;

/// Send ICMP echo requests and record `alive` + `ttl` for each device that replies.
/// `results` is modified in place.
pub fn check_alive(results: &mut [ScanResult], timeout_ms: u64) -> Result<()> {
    // SOCK_RAW + IPPROTO_ICMP:
    //   send → kernel prepends the IPv4 header (we only write the ICMP part)
    //   recv → kernel hands us IPv4 header + ICMP payload, so TTL is accessible
    let socket = Socket::new(Domain::IPV4, Type::RAW, Some(Protocol::ICMPV4))?;
    let timeout = Duration::from_millis(timeout_ms);

    // Use process ID as the ICMP identifier so we can ignore unrelated packets.
    let pid = (std::process::id() & 0xFFFF) as u16;

    for (i, result) in results.iter_mut().enumerate() {
        let ip: Ipv4Addr = match result.ip.parse() {
            Ok(ip) => ip,
            Err(_) => continue,
        };

        // Build an 8-byte ICMP echo request packet.
        // Two blocks: first set all fields (drops mutable borrow), then set checksum.
        let mut icmp_buf = [0u8; 8];
        {
            let mut req = MutableEchoRequestPacket::new(&mut icmp_buf).unwrap();
            req.set_icmp_type(IcmpTypes::EchoRequest);
            req.set_identifier(pid);
            req.set_sequence_number(i as u16 + 1);
        } // req (mutable borrow) dropped here
        let cs = checksum(&IcmpPacket::new(&icmp_buf).unwrap());
        MutableEchoRequestPacket::new(&mut icmp_buf)
            .unwrap()
            .set_checksum(cs);

        let dest = SocketAddrV4::new(ip, 0);
        if socket.send_to(&icmp_buf, &dest.into()).is_err() {
            continue;
        }

        // Loop receiving packets until we get one from our target IP or time out.
        // Other hosts may send ICMP traffic (e.g. late replies from a prior ping)
        // that we need to discard.
        let deadline = Instant::now() + timeout;
        let mut buf = [MaybeUninit::<u8>::uninit(); 512];

        while let Some(remaining) = deadline.checked_duration_since(Instant::now()) {
            // Update the socket timeout to the remaining budget.
            if socket
                .set_read_timeout(Some(remaining.max(Duration::from_millis(1))))
                .is_err()
            {
                break;
            }

            let (n, src) = match socket.recv_from(&mut buf) {
                Ok(r) => r,
                Err(_) => break, // timeout or error
            };

            // Fast path: check source IP from the SockAddr before parsing the buffer.
            let src_ip = match src.as_socket_ipv4() {
                Some(s) => *s.ip(),
                None => continue,
            };
            if src_ip != ip {
                continue; // discard — not from our target
            }

            // The received buffer is: 20-byte IPv4 header + ICMP payload.
            // SAFETY: recv_from guarantees the first n bytes are initialised.
            let data = unsafe { std::slice::from_raw_parts(buf.as_ptr() as *const u8, n) };

            if let Some(ip_pkt) = Ipv4Packet::new(data) {
                result.alive = true;
                result.ttl = Some(ip_pkt.get_ttl());
            }
            break;
        }
    }

    Ok(())
}
