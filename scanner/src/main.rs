// ============================================================
// scanner/src/main.rs — Entry point for the Rust scanner binary
//
// Two operating modes selected by --mode:
//
//   scan (default) — active scan: ARP sweep + ICMP + TCP port probe.
//     Prints one JSON array to stdout and exits. Called by the Go
//     controller on each scheduled scan interval.
//
//   listen — passive ARP monitor: opens AF_PACKET, watches all ARP
//     traffic on the LAN, and emits one JSON line per event to stdout.
//     Runs forever until the Go controller kills the process on shutdown.
//
// The Go controller reads that JSON and does the rest.
// ============================================================

use anyhow::Result;
use clap::{Parser, ValueEnum};
use std::net::Ipv4Addr;

mod arp;
mod gateway;
mod ping;
mod tcp;
mod types;

#[derive(ValueEnum, Clone, PartialEq)]
enum Mode {
    /// Active ARP + ICMP + TCP scan (default)
    Scan,
    /// Passive ARP listener — streams events to stdout indefinitely
    Listen,
}

// Command-line arguments — clap fills these in automatically from what Go passes.
// Example (scan):   scanner --interface eth0 --subnet 192.168.1.0/24
// Example (listen): scanner --interface eth0 --mode listen
#[derive(Parser)]
#[command(name = "scanner", about = "Ding low-level network scanner")]
struct Args {
    /// Network interface (e.g. eth0)
    #[arg(short, long)]
    interface: String,

    /// Target subnet in CIDR notation — required for scan mode (e.g. 192.168.1.0/24)
    #[arg(short, long)]
    subnet: Option<String>,

    /// Comma-separated TCP ports to scan (scan mode only)
    #[arg(short, long, default_value = "22,80,443,8080,8443")]
    ports: String,

    /// Per-host timeout in milliseconds (scan mode only)
    #[arg(short, long, default_value = "500")]
    timeout_ms: u64,

    /// Operating mode: scan (default) or listen (passive ARP monitor)
    #[arg(long, value_enum, default_value = "scan")]
    mode: Mode,
}

fn main() -> Result<()> {
    let args = Args::parse();

    match args.mode {
        Mode::Listen => {
            // Passive mode: watch ARP traffic and stream events forever.
            // Go kills this process on shutdown via context cancellation.
            arp::listen(&args.interface)?;
        }

        Mode::Scan => {
            let subnet = args
                .subnet
                .ok_or_else(|| anyhow::anyhow!("--subnet is required for scan mode"))?;

            // Convert the ports string "22,80,443" into a list of numbers [22, 80, 443]
            let ports: Vec<u16> = args
                .ports
                .split(',')
                .filter_map(|p| p.trim().parse().ok()) // skip anything that isn't a valid number
                .collect();

            // Turn "192.168.1.0/24" into a list of every possible host IP in that range
            let hosts = subnet_hosts(&subnet)?;

            // Step 1 — ARP scan: send a broadcast message asking each IP "are you there?"
            // Only devices that reply are included in `results`.
            let mut results = arp::scan(&args.interface, &hosts, args.timeout_ms)?;

            // Step 2 — ICMP ping: double-check each found device is still alive
            ping::check_alive(&mut results, args.timeout_ms)?;

            // Step 3 — TCP port scan: try to connect to each port on each device
            tcp::scan_ports(&mut results, &ports, args.timeout_ms)?;

            // Step 4 — Gateway detection: find the default gateway for this interface
            // and tag every result with it (topology uses this to build the graph)
            let gw = gateway::detect(&args.interface)?;
            if let Some(gw_ip) = gw {
                let gw_str = gw_ip.to_string();
                for r in results.iter_mut() {
                    r.gateway = Some(gw_str.clone());
                }
            }

            // Print the final results as a JSON array to stdout — Go reads this
            println!("{}", serde_json::to_string(&results)?);
        }
    }

    Ok(())
}

// Convert a subnet like "192.168.1.0/24" into a list of individual IP addresses.
// /24 means the last number can be 1–254, giving 254 hosts.
fn subnet_hosts(cidr: &str) -> Result<Vec<Ipv4Addr>> {
    // Split "192.168.1.0/24" into base="192.168.1.0" and prefix="24"
    let (base_str, prefix_str) = cidr
        .split_once('/')
        .ok_or_else(|| anyhow::anyhow!("invalid CIDR: {}", cidr))?;

    let base: Ipv4Addr = base_str.parse()?;
    let prefix: u32 = prefix_str.parse()?;

    // /31 and /32 have no usable hosts, so we require at most /30
    anyhow::ensure!(prefix <= 30, "prefix must be ≤ 30 (got {})", prefix);

    // Bit-math to find the first and last address in the subnet
    let mask = !0u32 << (32 - prefix); // e.g. /24 → 255.255.255.0
    let network = u32::from(base) & mask; // first address (e.g. 192.168.1.0)
    let broadcast = network | !mask; // last address  (e.g. 192.168.1.255)

    // Return every address between the network address and broadcast (exclusive)
    Ok((network + 1..broadcast).map(Ipv4Addr::from).collect())
}
