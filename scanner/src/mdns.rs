// ============================================================
// scanner/src/mdns.rs — mDNS / Bonjour service discovery
//
// mDNS (Multicast DNS, RFC 6762) lets devices announce what
// services they offer without a central DNS server.
//
// How it works:
//   1. We bind to the mDNS multicast group 224.0.0.251:5353.
//   2. We send PTR queries for ~15 common service types
//      (e.g. "_googlecast._tcp.local.") to trigger devices to
//      announce themselves immediately rather than waiting for
//      their next periodic announcement.
//   3. Devices reply with:
//        PTR record  → service type  → instance name
//        SRV record  → instance name → hostname + port
//        A   record  → hostname      → IP address
//   4. We correlate those three record types to produce
//      { ip, service, name } tuples and emit them as JSON lines.
//
// The process exits after `timeout_ms` milliseconds, at which
// point the Go controller has collected all the results.
// ============================================================

use anyhow::Result;
use dns_parser::{Packet, RData};
use pnet::datalink;
use socket2::{Domain, Protocol, Socket, Type};
use std::collections::{HashMap, HashSet};
use std::io::{self, Write};
use std::net::{Ipv4Addr, SocketAddrV4, UdpSocket};
use std::time::{Duration, Instant};

use crate::types::MdnsEvent;

const MDNS_ADDR: Ipv4Addr = Ipv4Addr::new(224, 0, 0, 251);
const MDNS_PORT: u16 = 5353;

// Service types to actively query. Each one causes devices offering that
// service to send an announcement, giving us PTR + SRV + A records.
const SERVICE_TYPES: &[&str] = &[
    "_googlecast._tcp.local.",   // Chromecast, Google Cast TVs
    "_airplay._tcp.local.",      // Apple TV, AirPlay receivers
    "_raop._tcp.local.",         // AirPlay audio (HomePod, Apple TV)
    "_hap._tcp.local.",          // HomeKit accessories
    "_homekit._tcp.local.",      // HomeKit (older variant)
    "_spotify-connect._tcp.local.", // Spotify Connect speakers
    "_printer._tcp.local.",      // Generic network printers
    "_ipp._tcp.local.",          // IPP printers (most modern printers)
    "_pdl-datastream._tcp.local.", // PCL/PDL printers (HP JetDirect)
    "_workstation._tcp.local.",  // Linux/macOS workstations (Avahi)
    "_ssh._tcp.local.",          // SSH servers
    "_smb._tcp.local.",          // Windows/Samba file shares (NAS)
    "_afpovertcp._tcp.local.",   // Apple File Protocol (Mac)
    "_daap._tcp.local.",         // iTunes / Music sharing (media server)
    "_device-info._tcp.local.",  // Apple device model info
];

/// Runs an mDNS discovery scan for `timeout_ms` milliseconds.
/// Sends PTR queries, collects responses, and writes one JSON line per
/// resolved device to stdout. Called by Go as a subprocess.
pub fn scan(iface_name: &str, timeout_ms: u64) -> Result<()> {
    let iface_ip = iface_ipv4(iface_name)
        .ok_or_else(|| anyhow::anyhow!("no IPv4 address on {}", iface_name))?;

    // SO_REUSEADDR + SO_REUSEPORT via socket2 so we can bind 5353
    // even if avahi-daemon or another mDNS responder is running.
    let raw = Socket::new(Domain::IPV4, Type::DGRAM, Some(Protocol::UDP))?;
    raw.set_reuse_address(true)?;
    #[cfg(target_os = "linux")]
    raw.set_reuse_port(true)?;
    raw.bind(&SocketAddrV4::new(Ipv4Addr::UNSPECIFIED, MDNS_PORT).into())?;
    let socket: UdpSocket = raw.into();
    socket.join_multicast_v4(&MDNS_ADDR, &iface_ip)?;
    socket.set_multicast_loop_v4(false)?;
    // Short read timeout keeps the loop responsive without busy-spinning.
    socket.set_read_timeout(Some(Duration::from_millis(200)))?;

    // Send PTR queries for each service type to trigger immediate announcements.
    let dest = SocketAddrV4::new(MDNS_ADDR, MDNS_PORT);
    for svc in SERVICE_TYPES {
        if let Ok(q) = build_ptr_query(svc) {
            let _ = socket.send_to(&q, dest);
        }
    }

    // Correlation caches: PTR tells us (instance → service + name),
    // SRV tells us (instance → hostname), A tells us (hostname → IP).
    let mut ptr_map: HashMap<String, (String, String)> = HashMap::new();
    let mut srv_map: HashMap<String, String> = HashMap::new();
    let mut a_map: HashMap<String, Ipv4Addr> = HashMap::new();
    let mut emitted: HashSet<String> = HashSet::new();

    let deadline = Instant::now() + Duration::from_millis(timeout_ms);
    let mut buf = [0u8; 9000]; // mDNS messages can be up to 9000 bytes
    let stdout = io::stdout();

    while Instant::now() < deadline {
        let (len, _) = match socket.recv_from(&mut buf) {
            Ok(r) => r,
            Err(e)
                if e.kind() == io::ErrorKind::WouldBlock
                    || e.kind() == io::ErrorKind::TimedOut =>
            {
                continue
            }
            Err(e) => return Err(e.into()),
        };

        let packet = match Packet::parse(&buf[..len]) {
            Ok(p) => p,
            Err(_) => continue,
        };

        // mDNS answers often include SRV + A in the "additional" section
        // of the same packet as the PTR answer — so check all sections.
        for record in packet
            .answers
            .iter()
            .chain(packet.nameservers.iter())
            .chain(packet.additional.iter())
        {
            let name = record.name.to_string();
            match &record.data {
                RData::PTR(ptr) => {
                    let instance = ptr.0.to_string();
                    // name is the service type, e.g. "_googlecast._tcp.local"
                    let service = strip_local(&name);
                    let friendly = instance_name(&instance, &name);
                    ptr_map.insert(instance, (service, friendly));
                }
                RData::SRV(srv) => {
                    // name = instance, target = hostname
                    srv_map.insert(name, srv.target.to_string());
                }
                RData::A(a) => {
                    // name = hostname
                    a_map.insert(name, a.0);
                }
                _ => {}
            }
        }

        // After processing each packet, try to emit any fully-correlated events.
        for (instance, (service, friendly)) in &ptr_map {
            if emitted.contains(instance) {
                continue;
            }
            if let Some(hostname) = srv_map.get(instance) {
                if let Some(&ip) = a_map.get(hostname) {
                    let ev = MdnsEvent {
                        ip: ip.to_string(),
                        service: service.clone(),
                        name: friendly.clone(),
                    };
                    if let Ok(line) = serde_json::to_string(&ev) {
                        let mut out = stdout.lock();
                        let _ = writeln!(out, "{}", line);
                        let _ = out.flush();
                    }
                    emitted.insert(instance.clone());
                }
            }
        }
    }

    Ok(())
}

// ---- Helpers ---------------------------------------------------------------

/// Strip the `.local` suffix from a DNS name.
/// "_googlecast._tcp.local" → "_googlecast._tcp"
fn strip_local(name: &str) -> String {
    name.strip_suffix(".local")
        .unwrap_or(name)
        .trim_end_matches('.')
        .to_string()
}

/// Extract the human-readable part of an mDNS instance name.
/// "Bedroom TV._googlecast._tcp.local"  (service "_googlecast._tcp.local") → "Bedroom TV"
fn instance_name(instance: &str, service_owner: &str) -> String {
    // Try exact suffix stripping first.
    let suffix = format!(".{}", service_owner.trim_end_matches('.'));
    if let Some(name) = instance.trim_end_matches('.').strip_suffix(&suffix) {
        return name.to_string();
    }
    // Fallback: everything before the first "._something" label.
    if let Some(pos) = instance.find("._") {
        return instance[..pos].to_string();
    }
    instance.trim_end_matches('.').to_string()
}

/// Return the first IPv4 address of the named interface.
fn iface_ipv4(name: &str) -> Option<Ipv4Addr> {
    for iface in datalink::interfaces() {
        if iface.name != name {
            continue;
        }
        for ip in &iface.ips {
            if let std::net::IpAddr::V4(v4) = ip.ip() {
                if !v4.is_loopback() {
                    return Some(v4);
                }
            }
        }
    }
    None
}

/// Build a minimal DNS PTR query packet for the given service type.
/// Layout: 12-byte header + encoded QNAME + QTYPE(PTR) + QCLASS(IN).
fn build_ptr_query(service: &str) -> Result<Vec<u8>> {
    let mut pkt = Vec::with_capacity(64);
    // Header: ID=0, standard query, QDCOUNT=1, rest=0
    pkt.extend_from_slice(&[
        0x00, 0x00, // Transaction ID
        0x00, 0x00, // Flags (standard query, no recursion)
        0x00, 0x01, // QDCOUNT = 1
        0x00, 0x00, // ANCOUNT = 0
        0x00, 0x00, // NSCOUNT = 0
        0x00, 0x00, // ARCOUNT = 0
    ]);
    // QNAME in DNS label format: <len><label><len><label>...<0>
    for label in service.split('.') {
        if label.is_empty() {
            continue;
        }
        pkt.push(label.len() as u8);
        pkt.extend_from_slice(label.as_bytes());
    }
    pkt.push(0x00); // root label (end of name)
    pkt.extend_from_slice(&[0x00, 0x0C]); // QTYPE  = PTR (12)
    pkt.extend_from_slice(&[0x00, 0x01]); // QCLASS = IN  (1)
    Ok(pkt)
}
