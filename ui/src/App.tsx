// ============================================================
// ui/src/App.tsx — Root component (the whole application)
//
// This component owns all the important state:
//   - devices   : the current list of discovered network devices
//   - changes   : what changed since the last scan (NEW/GONE/PORTS)
//   - status    : interface name, subnet, last scan time
//   - scanning  : whether a scan is in progress right now
//
// On startup it fetches the latest data from the server.
// While running it listens for real-time SSE events to keep
// the UI updated without you having to refresh the page.
// ============================================================

import { useCallback, useEffect, useState } from 'react'
import { fetchDevices, fetchStatus, triggerScan } from './api/client'
import { ChangesFeed } from './components/ChangesFeed'
import { DeviceGrid } from './components/DeviceGrid'
import { ScanButton } from './components/ScanButton'
import { StatusBar } from './components/StatusBar'
import { TopologyMap } from './components/TopologyMap'
import { useEvents } from './hooks/useEvents'
import type { Change, Device, Status } from './types'

export default function App() {
  // --- State ---
  const [devices, setDevices] = useState<Device[]>([])      // all found devices
  const [changes, setChanges] = useState<Change[]>([])      // what changed in the last scan
  const [newIPs, setNewIPs] = useState<Set<string>>(new Set()) // IPs that are brand new (for the green badge)
  const [status, setStatus] = useState<Status | null>(null) // header info (interface, subnet, time)
  const [scanning, setScanning] = useState(false)           // true while a scan is running
  const [view, setView] = useState<'grid' | 'topology'>('grid') // current view mode
  const [scanCount, setScanCount] = useState(0)             // increments after each scan, triggers topology refresh

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

  // Called when the user clicks "Scan now"
  const handleScan = () => {
    triggerScan().catch(console.error) // tell the server to start a scan
    // We don't set scanning=true here — we wait for the SSE "scan_start" event
    // so the spinner is only shown when the server actually starts working
  }

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

        {/* Scan button + device count + view toggle */}
        <div className="flex items-center gap-4">
          <ScanButton scanning={scanning} onScan={handleScan} />
          <span className="text-slate-500 text-sm">
            {devices.length > 0
              ? (() => {
                  const online = devices.filter((d) => d.alive).length
                  const total = devices.length
                  return online === total
                    ? `${total} online`
                    : `${online} online · ${total} total`
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

        {/* Changes since last scan (hidden when empty) */}
        <ChangesFeed changes={changes} />

        {/* View: either device grid or topology map */}
        {view === 'grid' ? (
          <DeviceGrid devices={devices} newIPs={newIPs} />
        ) : (
          <TopologyMap scanCount={scanCount} />
        )}

      </main>
    </div>
  )
}
