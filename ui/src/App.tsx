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
import { AuthError, deleteDeviceLabel, exchangeToken, exportDevices, fetchARPWatch, fetchDevices, fetchStatus, logout, setDeviceLabel, setDeviceNotify, triggerScan } from './api/client'
import { ChangeLog } from './components/ChangeLog'
import { ChangesFeed } from './components/ChangesFeed'
import { DeviceGrid } from './components/DeviceGrid'
import { DeviceHistory } from './components/DeviceHistory'
import { LoginPage } from './components/LoginPage'
import { ScanButton } from './components/ScanButton'
import { SettingsPage } from './components/SettingsPage'
import { StatusBar } from './components/StatusBar'
import { TopologyMap } from './components/TopologyMap'
import { useEvents } from './hooks/useEvents'
import type { ARPWatchEntry, Change, Device, Status } from './types'

export default function App() {
  // --- State ---
  const [devices, setDevices] = useState<Device[]>([])      // all found devices
  const [changes, setChanges] = useState<Change[]>([])      // what changed in the last scan (NEW/GONE/PORTS/BACK)
  const [newIPs, setNewIPs] = useState<Set<string>>(new Set()) // IPs that are brand new (for the green badge)
  const [status, setStatus] = useState<Status | null>(null) // header info (interface, subnet, time)
  const [scanning, setScanning] = useState(false)           // true while a scan is running
  const [view, setView] = useState<'grid' | 'topology' | 'events'>('grid') // current view mode
  const [scanCount, setScanCount] = useState(0)             // increments after each scan, triggers topology refresh
  const [selectedDeviceIP, setSelectedDeviceIP] = useState<string | null>(null) // history view target
  const [query, setQuery] = useState('')                                        // text search
  const [statusFilter, setStatusFilter] = useState<'all' | 'online' | 'offline'>('all') // status pill
  // null = still checking (first load), false = not authed, true = authed
  const [authed, setAuthed] = useState<boolean | null>(null)
  const [authError, setAuthError] = useState('')
  const [page, setPage] = useState<'main' | 'settings'>('main')
  const [arpConflicts, setArpConflicts] = useState<ARPWatchEntry[]>([])

  // --- Initial data load ---
  // Fetch status + devices on mount. A 401 means auth is required — show login page.
  const loadData = useCallback(() => {
    Promise.all([
      fetchStatus().then(setStatus),
      fetchDevices().then(setDevices),
      fetchARPWatch().then(setArpConflicts).catch(() => {}),
    ])
      .then(() => setAuthed(true))
      .catch((err) => {
        if (err instanceof AuthError) setAuthed(false)
        else { setAuthed(true); console.error(err) }
      })
  }, [])

  useEffect(() => {
    // The OAuth callback passes the exchange token in the URL fragment (#exchange=TOKEN)
    // so that Cloudflare Tunnel cannot strip it (fragments are browser-only, never proxied).
    const hash = window.location.hash  // e.g. "#exchange=abc123"
    const hashParams = new URLSearchParams(hash.startsWith('#') ? hash.slice(1) : hash)
    const token = hashParams.get('exchange')
    if (token) {
      // Strip the fragment from the URL so it can't be bookmarked or replayed
      window.history.replaceState(null, '', window.location.pathname + window.location.search)
      exchangeToken(token)
        .then(() => loadData())
        .catch((err: unknown) => {
          const msg = err instanceof Error ? err.message : String(err)
          setAuthError('Sign-in failed: ' + msg + ' — please try again')
          setAuthed(false)
        })
    } else {
      loadData()
    }
  }, [loadData])

  // --- Real-time updates via SSE ---
  // Only connect when authenticated — closes the stream on logout automatically.
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
        // Refresh ARP watch data — MAC conflicts may have been detected
        fetchARPWatch().then(setArpConflicts).catch(() => {})
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
    }, []), // useCallback with [] means this function is created once and never recreated
    authed === true  // only open SSE when authenticated
  )

  // Called when the user sets or clears a custom device name.
  // Updates local state immediately; calls the API in the background.
  const handleLabelChange = useCallback((ip: string, label: string | null) => {
    setDevices((prev) => prev.map((d) => (d.ip === ip ? { ...d, label } : d)))
    const req = label ? setDeviceLabel(ip, label) : deleteDeviceLabel(ip)
    req.catch(console.error)
  }, [])

  // Called when the user toggles the notification bell on a device card.
  const handleNotifyChange = useCallback((ip: string, enabled: boolean) => {
    setDevices((prev) => prev.map((d) => (d.ip === ip ? { ...d, notify: enabled } : d)))
    setDeviceNotify(ip, enabled).catch(console.error)
  }, [])

  const handleLogout = () => {
    logout().catch(console.error)
    setAuthed(false)
    setDevices([])
    setChanges([])
    setStatus(null)
  }

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

  // Still checking auth — show blank screen to avoid flash of wrong content
  if (authed === null) return <div className="min-h-screen bg-slate-900" />

  // Not authenticated — show login page
  if (authed === false) return <LoginPage onLogin={loadData} initialError={authError} />

  if (page === 'settings') return <SettingsPage onBack={() => setPage('main')} />

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
          {/* Right side: interface, subnet, last scan time + settings + logout */}
          <div className="flex items-center gap-3">
            <StatusBar status={status} />
            <button
              onClick={() => setPage('settings')}
              className="text-slate-500 hover:text-slate-300 transition-colors"
              title="Settings"
            >
              <svg viewBox="0 0 16 16" fill="currentColor" className="w-4 h-4">
                <path d="M8 4.754a3.246 3.246 0 1 0 0 6.492 3.246 3.246 0 0 0 0-6.492zM5.754 8a2.246 2.246 0 1 1 4.492 0 2.246 2.246 0 0 1-4.492 0z"/>
                <path d="M9.796 1.343c-.527-1.79-3.065-1.79-3.592 0l-.094.319a.873.873 0 0 1-1.255.52l-.292-.16c-1.64-.892-3.433.902-2.54 2.541l.159.292a.873.873 0 0 1-.52 1.255l-.319.094c-1.79.527-1.79 3.065 0 3.592l.319.094a.873.873 0 0 1 .52 1.255l-.16.292c-.892 1.64.901 3.434 2.541 2.54l.292-.159a.873.873 0 0 1 1.255.52l.094.319c.527 1.79 3.065 1.79 3.592 0l.094-.319a.873.873 0 0 1 1.255-.52l.292.16c1.64.893 3.434-.902 2.54-2.541l-.159-.292a.873.873 0 0 1 .52-1.255l.319-.094c1.79-.527 1.79-3.065 0-3.592l-.319-.094a.873.873 0 0 1-.52-1.255l.16-.292c.893-1.64-.902-3.433-2.541-2.54l-.292.159a.873.873 0 0 1-1.255-.52l-.094-.319zm-2.633.283c.246-.835 1.428-.835 1.674 0l.094.319a1.873 1.873 0 0 0 2.693 1.115l.291-.16c.764-.415 1.6.42 1.184 1.185l-.159.292a1.873 1.873 0 0 0 1.116 2.692l.318.094c.835.246.835 1.428 0 1.674l-.319.094a1.873 1.873 0 0 0-1.115 2.693l.16.291c.415.764-.42 1.6-1.185 1.184l-.291-.159a1.873 1.873 0 0 0-2.693 1.116l-.094.318c-.246.835-1.428.835-1.674 0l-.094-.319a1.873 1.873 0 0 0-2.692-1.115l-.292.16c-.764.415-1.6-.42-1.184-1.185l.159-.291A1.873 1.873 0 0 0 1.945 8.93l-.319-.094c-.835-.246-.835-1.428 0-1.674l.319-.094A1.873 1.873 0 0 0 3.06 4.474l-.16-.292c-.415-.764.42-1.6 1.185-1.184l.292.159a1.873 1.873 0 0 0 2.692-1.115l.094-.319z"/>
              </svg>
            </button>
            <button
              onClick={handleLogout}
              className="text-xs text-slate-500 hover:text-slate-300 transition-colors"
              title="Sign out"
            >
              Sign out
            </button>
          </div>
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
                <button
                  onClick={() => setView('events')}
                  className={[
                    'px-3 py-1 text-xs font-medium rounded-md transition-colors',
                    view === 'events'
                      ? 'bg-slate-700 text-slate-100'
                      : 'text-slate-400 hover:text-slate-200',
                  ].join(' ')}
                >
                  Events
                </button>
              </div>
              {/* Export dropdown */}
              {devices.length > 0 && (
                <div className="relative group">
                  <button
                    className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg bg-slate-800 border border-slate-700 text-slate-400 hover:text-slate-200 hover:border-slate-500 transition-colors"
                    title="Export device list"
                  >
                    <svg viewBox="0 0 16 16" fill="currentColor" className="w-3.5 h-3.5">
                      <path d="M.5 9.9a.5.5 0 0 1 .5.5v2.5a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-2.5a.5.5 0 0 1 1 0v2.5a2 2 0 0 1-2 2H2a2 2 0 0 1-2-2v-2.5a.5.5 0 0 1 .5-.5z"/>
                      <path d="M7.646 11.854a.5.5 0 0 0 .708 0l3-3a.5.5 0 0 0-.708-.708L8.5 10.293V1.5a.5.5 0 0 0-1 0v8.793L5.354 8.146a.5.5 0 1 0-.708.708l3 3z"/>
                    </svg>
                    Export
                  </button>
                  <div className="absolute right-0 top-full mt-1 w-32 bg-slate-800 border border-slate-700 rounded-lg shadow-lg overflow-hidden opacity-0 invisible group-hover:opacity-100 group-hover:visible transition-all z-20">
                    <button
                      onClick={() => exportDevices('csv')}
                      className="w-full text-left px-3 py-2 text-xs text-slate-300 hover:bg-slate-700 transition-colors"
                    >
                      CSV (.csv)
                    </button>
                    <button
                      onClick={() => exportDevices('json')}
                      className="w-full text-left px-3 py-2 text-xs text-slate-300 hover:bg-slate-700 transition-colors"
                    >
                      JSON (.json)
                    </button>
                  </div>
                </div>
              )}
            </div>

            {/* Search + filter bar — only shown in grid view */}
            {view === 'grid' && devices.length > 0 && (
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

            {/* ARP spoof global warning — shown in all views when conflicts exist */}
            {arpConflicts.length > 0 && view !== 'events' && (
              <button
                onClick={() => setView('events')}
                className="w-full flex items-center gap-2 px-3 py-2 rounded-lg border border-orange-700/50 bg-orange-950/30 text-left hover:bg-orange-950/50 transition-colors"
              >
                <svg viewBox="0 0 16 16" fill="currentColor" className="w-3.5 h-3.5 text-orange-400 shrink-0">
                  <path d="M8.982 1.566a1.13 1.13 0 0 0-1.96 0L.165 13.233c-.457.778.091 1.767.98 1.767h13.713c.889 0 1.438-.99.98-1.767L8.982 1.566zM8 5c.535 0 .954.462.9.995l-.35 3.507a.552.552 0 0 1-1.1 0L7.1 5.995A.905.905 0 0 1 8 5zm.002 6a1 1 0 1 1 0 2 1 1 0 0 1 0-2z"/>
                </svg>
                <span className="text-xs text-orange-300 font-medium">
                  ARP spoofing warning — {arpConflicts.length} {arpConflicts.length === 1 ? 'device has' : 'devices have'} changed MAC address. Click to review.
                </span>
              </button>
            )}

            {/* Changes since last scan — only shown in grid / topology views */}
            {view !== 'events' && <ChangesFeed changes={changes} />}

            {/* View: device grid, topology map, or persistent event log */}
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
                  onNotifyChange={handleNotifyChange}
                  onSelect={setSelectedDeviceIP}
                />
              )
            ) : view === 'topology' ? (
              <TopologyMap scanCount={scanCount} />
            ) : (
              <>
                {/* ARP Watch summary — shown only when conflicts exist */}
                {arpConflicts.length > 0 && (
                  <div className="rounded-xl border border-orange-700/50 bg-orange-950/30 p-4 space-y-3">
                    <div className="flex items-center gap-2">
                      <svg viewBox="0 0 16 16" fill="currentColor" className="w-4 h-4 text-orange-400 shrink-0">
                        <path d="M8.982 1.566a1.13 1.13 0 0 0-1.96 0L.165 13.233c-.457.778.091 1.767.98 1.767h13.713c.889 0 1.438-.99.98-1.767L8.982 1.566zM8 5c.535 0 .954.462.9.995l-.35 3.507a.552.552 0 0 1-1.1 0L7.1 5.995A.905.905 0 0 1 8 5zm.002 6a1 1 0 1 1 0 2 1 1 0 0 1 0-2z"/>
                      </svg>
                      <h3 className="text-sm font-semibold text-orange-300">
                        ARP Watch — {arpConflicts.length} suspicious {arpConflicts.length === 1 ? 'address' : 'addresses'}
                      </h3>
                    </div>
                    <p className="text-xs text-orange-400/80">
                      These IPs have been seen with more than one MAC address. This may indicate ARP cache poisoning, a device swap, or DHCP reassignment.
                    </p>
                    <div className="space-y-2">
                      {arpConflicts.map((entry) => (
                        <div key={entry.ip} className="rounded-lg bg-slate-900/60 px-3 py-2 space-y-1.5">
                          <p className="font-mono text-sm font-semibold text-slate-200">{entry.ip}</p>
                          <div className="space-y-1">
                            {entry.history.map((h, i) => (
                              <div key={h.mac} className="flex items-center gap-2 text-xs">
                                <span className={`w-1.5 h-1.5 rounded-full shrink-0 ${i === 0 ? 'bg-orange-400' : 'bg-slate-600'}`} />
                                <span className="font-mono text-slate-300">{h.mac}</span>
                                <span className="text-slate-500">
                                  {i === 0 ? 'current' : `last seen ${new Date(h.last_seen).toLocaleDateString()}`}
                                </span>
                              </div>
                            ))}
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
                <ChangeLog scanCount={scanCount} />
              </>
            )}
          </>
        )}

      </main>
    </div>
  )
}
