use anyhow::Result;
use clap::Parser;
use std::net::Ipv4Addr;

mod arp;
mod ping;
mod tcp;
mod types;

#[derive(Parser)]
#[command(name = "scanner", about = "Ding low-level network scanner")]
struct Args {
    /// Network interface (e.g. eth0)
    #[arg(short, long)]
    interface: String,

    /// Target subnet in CIDR notation (e.g. 192.168.1.0/24)
    #[arg(short, long)]
    subnet: String,

    /// Comma-separated TCP ports to scan
    #[arg(short, long, default_value = "22,80,443,8080,8443")]
    ports: String,

    /// Per-host timeout in milliseconds
    #[arg(short, long, default_value = "500")]
    timeout_ms: u64,
}

fn main() -> Result<()> {
    let args = Args::parse();

    let ports: Vec<u16> = args
        .ports
        .split(',')
        .filter_map(|p| p.trim().parse().ok())
        .collect();

    let hosts = subnet_hosts(&args.subnet)?;

    // ARP discovery — only alive hosts are returned
    let mut results = arp::scan(&args.interface, &hosts, args.timeout_ms)?;

    // ICMP ping for hosts that didn't respond to ARP (e.g. different subnet)
    ping::check_alive(&mut results, args.timeout_ms)?;

    // TCP port scan on every discovered host
    tcp::scan_ports(&mut results, &ports, args.timeout_ms)?;

    println!("{}", serde_json::to_string(&results)?);
    Ok(())
}

fn subnet_hosts(cidr: &str) -> Result<Vec<Ipv4Addr>> {
    let (base_str, prefix_str) = cidr
        .split_once('/')
        .ok_or_else(|| anyhow::anyhow!("invalid CIDR: {}", cidr))?;

    let base: Ipv4Addr = base_str.parse()?;
    let prefix: u32 = prefix_str.parse()?;

    anyhow::ensure!(prefix <= 30, "prefix must be ≤ 30 (got {})", prefix);

    let mask = !0u32 << (32 - prefix);
    let network = u32::from(base) & mask;
    let broadcast = network | !mask;

    // Exclude network address and broadcast
    Ok((network + 1..broadcast).map(Ipv4Addr::from).collect())
}
