use serde::{Deserialize, Serialize};

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct ScanResult {
    pub ip: String,
    pub mac: Option<String>,
    pub hostname: Option<String>,
    pub open_ports: Vec<u16>,
    pub alive: bool,
}

impl ScanResult {
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
