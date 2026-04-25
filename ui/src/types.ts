// ============================================================
// ui/src/types.ts — TypeScript type definitions
//
// These types mirror the JSON shapes that the Go server sends.
// TypeScript uses them to catch mistakes at compile time —
// if you try to access a field that doesn't exist, it errors.
// ============================================================

// One device found on the network (matches scanner.Result in Go)
export interface Device {
  ip: string           // e.g. "192.168.1.42"
  mac: string | null   // e.g. "aa:bb:cc:dd:ee:ff" — null if unknown
  hostname: string | null // human-readable name — null (not yet implemented)
  open_ports: number[] // e.g. [22, 80, 443]
  alive: boolean       // true if device responded to ping
}

// One change detected between scans (matches diff.Change in Go)
export interface Change {
  kind: 'NEW' | 'GONE' | 'PORTS' // what type of change it is
  ip: string                      // which device changed
  desc: string                    // human-readable description, e.g. "mac=aa:bb:... ports=[22]"
}

// Data returned by GET /api/status
export interface Status {
  iface: string          // network interface, e.g. "wlp0s20f3"
  subnet: string         // subnet being scanned, e.g. "192.168.1.0/24"
  last_scan: string | null // ISO timestamp of last scan, or null if none yet
}

// One entry in the scan history (returned by GET /api/history)
export interface HistoryEntry {
  scanned_at: string  // ISO timestamp, e.g. "2026-04-25T18:00:00Z"
  results: Device[]   // all devices found at that time
}

// Every possible SSE event the server can push to the browser.
// TypeScript's union type (|) means it's one of these shapes.
export type SseEvent =
  | { type: 'connected' }   // SSE stream just opened — everything is working
  | { type: 'scan_start' }  // a scan just started running
  | { type: 'scan_result'; devices: Device[]; changes: Change[]; scanned_at: string } // scan finished
  | { type: 'scan_error'; error: string }  // scan failed
