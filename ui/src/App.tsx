// ============================================================
// ui/src/App.tsx — Root component (the whole application)
//
// This component owns all the important state:
//   - devices          : the current list of discovered network devices
//   - changes          : what changed since the last scan (NEW/GONE/PORTS/BACK)
//   - status           : interface name, subnet, last scan time
//   - scanning         : whether a scan is in progress right now
//   - selectedDeviceIP : when set, DeviceHistory replaces the grid/topology view
//
// Views:
//   Grid     — responsive card grid; click a card → DeviceHistory for that device
//   Topology — SVG star-layout graph of the network
//   History  — full-page view for one device (dot timeline + scan log)
//
// On startup it fetches the latest data from the server.
// While running it listens for real-time SSE events to keep
// the UI updated without you having to refresh the page.
// ============================================================

import { useCallback, useEffect, useMemo, useState } from 'react'
import { deleteDeviceLabel, fetchDevices, fetchStatus, setDeviceLabel, triggerScan } from './api/client'
import { ChangesFeed } from './components/ChangesFeed'
import { DeviceGrid } from './components/DeviceGrid'
import { DeviceHistory } from './components/DeviceHistory'
import { ScanButton } from './components/ScanButton'
import { StatusBar } from './components/StatusBar'
import { TopologyMap } from './components/TopologyMap'
import { useEvents } from './hooks/useEvents'
import type { Change, Device, Status } from './types'

export default function App() {
  // --- State ---
  const [devices, setDevices] = useState<Device[]>([])      // all found devices
  const [changes, setChanges] = useState<Change[]>([])      // what changed in the last scan (NEW/GONE/PORTS/BACK)
  const [newIPs, setNewIPs] = useState<Set<string>>(new Set()) // IPs that are brand new (for the green badge)
  const [status, setStatus] = useState<Status | null>(null) // header info (interface, subnet, time)
  const [scanning, setScanning] = useState(false)           // true while a scan is running
  const [view, setView] = useState<'grid' | 'topology'>('grid') // current view mode
  const [scanCount, setScanCount] = useState(0)             // increments after each scan, triggers topology refresh
  const [selectedDeviceIP, setSelectedDeviceIP] = useState<string | null>(null) // history view target
  const [query, setQuery] = useState('')                                        // text search
  const [statusFilter, setStatusFilter] = useState<'all' | 'online' | 'offline'>('all') // status pill

  // --- Initial data load ---
  // When the page first loads, fetch the current status and device list from the server.
  useEffect(() => {
    fetchStatus().then(setStatus).catch(console.error)
    fetchDevices().then(setDevices).catch(console.error)
  }, []) // [] means "run once when the component first appears"

  // --- Real-time updates via SSE ---
  // The server pushes these events when a scan runs (either on timer or on demand).
  useEvents(
    useCallback((event) => {
      if (event.type === 'scan_start') {
        // Scan just started — show the spinner on the button
        setScanning(true)
      } else if (event.type === 'scan_result') {
        // Scan finished — update everything with fresh data
        setScanning(false)
        setDevices(event.devices)
        setChanges(event.changes)
        // Track which IPs are brand new so we can show the green "NEW" badge
        setNewIPs(new Set(event.changes.filter((c) => c.kind === 'NEW').map((c) => c.ip)))
        // Update the "last scan" time in the header
        setStatus((s) => (s ? { ...s, last_scan: event.scanned_at } : s))
        // Bump counter so TopologyMap re-fetches the latest graph
        setScanCount((n) => n + 1)
      } else if (event.type === 'scan_error') {
        // Something went wrong — hide the spinner and log the error
        setScanning(false)
        console.error('scan error:', event.error)
      } else if (event.type === 'device_seen') {
        // Passive ARP detection — device appeared between full scans.
        // Add it to the changes feed; the next full scan will complete the picture.
        setChanges((prev) => [
          { kind: event.kind, ip: event.ip, desc: `passive ARP — mac=${event.mac}` },
          ...prev.slice(0, 19),
        ])
      }
      // 'connected' events are ignored — they just confirm the SSE stream is working
    }, []) // useCallback with [] means this function is created once and never recreated
  )

  // Called when the user sets or clears a custom device name.
  // Updates local state immediately; calls the API in the background.
  const handleLabelChange = useCallback((ip: string, label: string | null) => {
    setDevices((prev) => prev.map((d) => (d.ip === ip ? { ...d, label } : d)))
    const req = label ? setDeviceLabel(ip, label) : deleteDeviceLabel(ip)
    req.catch(console.error)
  }, [])

  // Called when the user clicks "Scan now"
  const handleScan = () => {
    triggerScan().catch(console.error) // tell the server to start a scan
    // We don't set scanning=true here — we wait for the SSE "scan_start" event
    // so the spinner is only shown when the server actually starts working
  }

  // Filtered device list — recomputed whenever devices, query, or statusFilter changes.
  const filteredDevices = useMemo(() => {
    let result = devices
    if (statusFilter === 'online')  result = result.filter((d) => d.alive)
    if (statusFilter === 'offline') result = result.filter((d) => !d.alive)
    const q = query.trim().toLowerCase()
    if (q) {
      result = result.filter((d) =>
        d.ip.includes(q) ||
        d.label?.toLowerCase().includes(q) ||
        d.hostname?.toLowerCase().includes(q) ||
        d.vendor?.toLowerCase().includes(q) ||
        d.device_type?.toLowerCase().includes(q) ||
        d.os?.toLowerCase().includes(q) ||
        d.mac?.toLowerCase().includes(q)
      )
    }
    return result
  }, [devices, query, statusFilter])

  // The device currently being viewed in the history page (null = grid/topology view).
  const selectedDevice = selectedDeviceIP
    ? devices.find((d) => d.ip === selectedDeviceIP) ?? null
    : null

  return (
    <div className="min-h-screen bg-slate-900 text-slate-100">

      {/* ---- Header bar (sticky — stays at top when you scroll) ---- */}
      <header className="sticky top-0 z-10 bg-slate-900/90 backdrop-blur border-b border-slate-800 px-4 py-3">
        <div className="max-w-6xl mx-auto flex items-center justify-between gap-4">
          {/* App title with a pulsing dot */}
          <div className="flex items-center gap-2.5">
            <span className="w-2.5 h-2.5 rounded-full bg-cyan-500 animate-pulse" />
            <h1 className="text-lg font-bold tracking-tight">Ding</h1>
          </div>
          {/* Right side: interface, subnet, last scan time */}
          <StatusBar status={status} />
        </div>
      </header>

      {/* ---- Main content ---- */}
      <main className="max-w-6xl mx-auto px-4 py-6 space-y-6">

        {selectedDevice ? (
          /* ---- Full-page device history view ---- */
          <DeviceHistory
            device={selectedDevice}
            onBack={() => setSelectedDeviceIP(null)}
          />
        ) : (
          /* ---- Normal grid / topology view ---- */
          <>
            {/* Scan button + device count + view toggle */}
            <div className="flex items-center gap-4">
              <ScanButton scanning={scanning} onScan={handleScan} />
              <span className="text-slate-500 text-sm">
                {devices.length > 0
                  ? (() => {
                      const online = devices.filter((d) => d.alive).length
                      const total = devices.length
                      const base = online === total
                        ? `${total} online`
                        : `${online} online · ${total} total`
                      return filteredDevices.length !== devices.length
                        ? `${filteredDevices.length} shown · ${base}`
                        : base
                    })()
                  : 'No scan data yet'}
              </span>
              {/* View toggle: Grid ↔ Topology */}
              <div className="ml-auto flex rounded-lg bg-slate-800 border border-slate-700 p-0.5">
                <button
                  onClick={() => setView('grid')}
                  className={[
                    'px-3 py-1 text-xs font-medium rounded-md transition-colors',
                    view === 'grid'
                      ? 'bg-slate-700 text-slate-100'
                      : 'text-slate-400 hover:text-slate-200',
                  ].join(' ')}
                >
                  Grid
                </button>
                <button
                  onClick={() => setView('topology')}
                  className={[
                    'px-3 py-1 text-xs font-medium rounded-md transition-colors',
                    view === 'topology'
                      ? 'bg-slate-700 text-slate-100'
                      : 'text-slate-400 hover:text-slate-200',
                  ].join(' ')}
                >
                  Topology
                </button>
              </div>
            </div>

            {/* Search + filter bar — only shown in grid view */}
            {view === 'grid' && (
              <div className="flex flex-wrap items-center gap-2">
                {/* Text search input */}
                <div className="relative flex-1 min-w-[180px]">
                  <svg className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-slate-500 pointer-events-none" viewBox="0 0 16 16" fill="currentColor">
                    <path d="M11.742 10.344a6.5 6.5 0 1 0-1.397 1.398h-.001c.03.04.062.078.098.115l3.85 3.85a1 1 0 0 0 1.415-1.414l-3.85-3.85a1.007 1.007 0 0 0-.115-.099zm-5.242 1.656a5.5 5.5 0 1 1 0-11 5.5 5.5 0 0 1 0 11z"/>
                  </svg>
                  <input
                    type="text"
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    placeholder="Search by name, IP, vendor…"
                    className="w-full bg-slate-800 border border-slate-700 rounded-lg pl-8 pr-8 py-1.5 text-sm text-slate-100 placeholder-slate-500 outline-none focus:ring-1 focus:ring-cyan-500 focus:border-cyan-500"
                  />
                  {query && (
                    <button
                      onClick={() => setQuery('')}
                      className="absolute right-2.5 top-1/2 -translate-y-1/2 text-slate-500 hover:text-slate-300"
                      title="Clear search"
                    >
                      <svg viewBox="0 0 16 16" fill="currentColor" className="w-3.5 h-3.5">
                        <path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06z"/>
                      </svg>
                    </button>
                  )}
                </div>
                {/* Status filter pills */}
                <div className="flex rounded-lg bg-slate-800 border border-slate-700 p-0.5">
                  {(['all', 'online', 'offline'] as const).map((f) => (
                    <button
                      key={f}
                      onClick={() => setStatusFilter(f)}
                      className={[
                        'px-3 py-1 rounded-md text-xs font-medium capitalize transition-colors',
                        statusFilter === f
                          ? 'bg-slate-700 text-slate-100'
                          : 'text-slate-400 hover:text-slate-200',
                      ].join(' ')}
                    >
                      {f}
                    </button>
                  ))}
                </div>
              </div>
            )}

            {/* Changes since last scan (hidden when empty) */}
            <ChangesFeed changes={changes} />

            {/* View: either device grid or topology map */}
            {view === 'grid' ? (
              filteredDevices.length === 0 && devices.length > 0 ? (
                <p className="text-slate-500 text-sm text-center py-12">
                  No devices match your search.
                </p>
              ) : (
                <DeviceGrid
                  devices={filteredDevices}
                  newIPs={newIPs}
                  onLabelChange={handleLabelChange}
                  onSelect={setSelectedDeviceIP}
                />
              )
            ) : (
              <TopologyMap scanCount={scanCount} />
            )}
          </>
        )}

      </main>
    </div>
  )
}
