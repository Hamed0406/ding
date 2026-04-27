// Full-page history view for a single device.
// Shows summary stats, a dot timeline of online/offline status across all
// recorded scans, and a log table (newest first) with port-change highlights.

import { useEffect, useState } from 'react'
import { fetchDeviceHistory } from '../api/client'
import type { Device, DeviceHistoryEntry } from '../types'
import { portLabel } from '../utils/ports'

interface Props {
  device: Device
  onBack: () => void
}

// Maps device category → [background, text] Tailwind classes (same as DeviceCard).
const TYPE_COLOURS: Record<string, [string, string]> = {
  'Router':           ['bg-violet-900/60', 'text-violet-300'],
  'Firewall':         ['bg-rose-900/60',   'text-rose-300'],
  'Smart Device':     ['bg-amber-900/60',  'text-amber-300'],
  'Google Device':    ['bg-amber-900/60',  'text-amber-300'],
  'Smart Speaker':    ['bg-amber-900/60',  'text-amber-300'],
  'IoT Hub':          ['bg-amber-900/60',  'text-amber-300'],
  'Appliance':        ['bg-stone-800',     'text-stone-300'],
  'Computer':         ['bg-sky-900/60',    'text-sky-300'],
  'NAS':              ['bg-indigo-900/60', 'text-indigo-300'],
  'Media Server':     ['bg-indigo-900/60', 'text-indigo-300'],
  'Phone':            ['bg-blue-900/60',   'text-blue-300'],
  'Apple Device':     ['bg-blue-900/60',   'text-blue-300'],
  'Samsung Device':   ['bg-blue-900/60',   'text-blue-300'],
  'Smart TV':         ['bg-teal-900/60',   'text-teal-300'],
  'Apple TV':         ['bg-teal-900/60',   'text-teal-300'],
  'Streaming Dongle': ['bg-cyan-900/60',   'text-cyan-300'],
  'Speaker':          ['bg-green-900/60',  'text-green-300'],
  'Printer':          ['bg-slate-700',     'text-slate-300'],
  'IP Camera':        ['bg-red-900/60',    'text-red-300'],
  'Gaming Console':   ['bg-purple-900/60', 'text-purple-300'],
}

function formatAge(iso: string): string {
  const s = Math.floor((Date.now() - new Date(iso).getTime()) / 1000)
  if (s < 60) return 'just now'
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  return `${Math.floor(h / 24)}d ago`
}

function formatFull(iso: string): string {
  return new Date(iso).toLocaleString()
}

function formatDateOnly(iso: string): string {
  return new Date(iso).toLocaleDateString()
}

// Returns true if two sorted port arrays differ.
function portsChanged(a: number[], b: number[]): boolean {
  if (a.length !== b.length) return true
  const sa = [...a].sort((x, y) => x - y)
  const sb = [...b].sort((x, y) => x - y)
  return sa.some((v, i) => v !== sb[i])
}

export function DeviceHistory({ device, onBack }: Props) {
  const [entries, setEntries] = useState<DeviceHistoryEntry[] | null>(null)

  useEffect(() => {
    fetchDeviceHistory(device.ip).then(setEntries).catch(console.error)
  }, [device.ip])

  const displayName = device.label ?? device.hostname ?? device.ip
  const firstSeen   = entries && entries.length > 0 ? entries[0].scanned_at : null
  const lastSeen    = entries && entries.length > 0 ? entries[entries.length - 1].scanned_at : null
  const onlineCount = entries ? entries.filter((e) => e.alive).length : 0
  const uptimePct   = entries && entries.length > 0
    ? Math.round((onlineCount / entries.length) * 100)
    : null

  // Newest-first for the log table.
  const reversed = entries ? [...entries].reverse() : []

  return (
    <div className="space-y-6">

      {/* ---- Header: back button + device name ---- */}
      <div className="flex items-center gap-4 flex-wrap">
        <button
          onClick={onBack}
          className="flex items-center gap-1.5 text-sm text-slate-400 hover:text-slate-200 transition-colors flex-shrink-0"
        >
          <svg viewBox="0 0 16 16" fill="currentColor" className="w-4 h-4">
            <path d="M9.78 12.78a.75.75 0 0 1-1.06 0L4.47 8.53a.75.75 0 0 1 0-1.06l4.25-4.25a.751.751 0 0 1 1.042.018.751.751 0 0 1 .018 1.042L6.06 8l3.72 3.72a.75.75 0 0 1 0 1.06z" />
          </svg>
          Back
        </button>

        <div className="flex items-center gap-3 min-w-0 flex-1">
          <h2 className="text-xl font-bold text-slate-100 truncate">{displayName}</h2>
          {device.device_type && (() => {
            const [bg, text] = TYPE_COLOURS[device.device_type] ?? ['bg-slate-700', 'text-slate-400']
            return (
              <span className={`px-1.5 py-0.5 rounded text-[10px] font-medium flex-shrink-0 ${bg} ${text}`}>
                {device.device_type}
              </span>
            )
          })()}
          <span
            className={`w-2.5 h-2.5 rounded-full flex-shrink-0 ${device.alive ? 'bg-green-400 animate-pulse' : 'bg-slate-600'}`}
            title={device.alive ? 'online' : 'offline'}
          />
        </div>
      </div>

      {/* ---- Device identifiers ---- */}
      <div className="flex flex-wrap gap-x-6 gap-y-1 text-xs text-slate-400">
        <span className="font-mono">{device.ip}</span>
        {device.mac      && <span className="font-mono">{device.mac}</span>}
        {device.vendor   && <span>{device.vendor}</span>}
        {device.hostname && <span className="font-mono">{device.hostname}</span>}
      </div>

      {/* ---- Summary stats ---- */}
      {entries && (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
          {([
            { label: 'First seen', value: firstSeen ? formatAge(firstSeen) : '—', title: firstSeen ? formatFull(firstSeen) : undefined },
            { label: 'Last seen',  value: lastSeen  ? formatAge(lastSeen)  : '—', title: lastSeen  ? formatFull(lastSeen)  : undefined },
            { label: 'Total scans', value: String(entries.length) },
            { label: 'Uptime',      value: uptimePct !== null ? `${uptimePct}%` : '—' },
          ] as { label: string; value: string; title?: string }[]).map(({ label, value, title }) => (
            <div
              key={label}
              className="bg-slate-800 border border-slate-700 rounded-lg px-4 py-3"
              title={title}
            >
              <div className="text-xs text-slate-500 mb-1">{label}</div>
              <div className="text-base font-semibold text-slate-100">{value}</div>
            </div>
          ))}
        </div>
      )}

      {/* ---- Dot timeline ---- */}
      {entries && entries.length > 0 && (
        <div className="bg-slate-800 border border-slate-700 rounded-lg p-4 space-y-3">
          <div className="text-xs text-slate-500 uppercase tracking-wider">Scan timeline</div>
          <div className="flex flex-wrap gap-1">
            {entries.map((e, i) => (
              <span
                key={i}
                className={`w-3 h-3 rounded-full flex-shrink-0 cursor-default transition-opacity hover:opacity-75 ${
                  e.alive ? 'bg-green-500' : 'bg-slate-600'
                }`}
                title={formatFull(e.scanned_at)}
              />
            ))}
          </div>
          <div className="flex justify-between text-[10px] text-slate-600 select-none">
            <span>{firstSeen ? formatDateOnly(firstSeen) : ''}</span>
            <span>{lastSeen  ? formatDateOnly(lastSeen)  : ''}</span>
          </div>
        </div>
      )}

      {/* ---- History log ---- */}
      {entries === null ? (
        <p className="text-slate-500 text-sm text-center py-12">Loading…</p>
      ) : entries.length === 0 ? (
        <p className="text-slate-500 text-sm text-center py-12">No scan history found for this device.</p>
      ) : (
        <div className="bg-slate-800 border border-slate-700 rounded-lg overflow-hidden">
          <div className="px-4 py-3 border-b border-slate-700 text-xs text-slate-500 uppercase tracking-wider">
            Scan records — newest first
          </div>
          <div className="divide-y divide-slate-700/50">
            {reversed.map((e, i) => {
              // The scan immediately before this one in time is at i+1 in the reversed array.
              const prev = reversed[i + 1]
              const changed = prev !== undefined && portsChanged(e.open_ports, prev.open_ports)
              return (
                <div
                  key={i}
                  className="flex items-center gap-4 px-4 py-2.5 text-sm hover:bg-slate-700/30 transition-colors"
                >
                  {/* Timestamp */}
                  <span
                    className="text-slate-500 text-xs w-20 flex-shrink-0 tabular-nums"
                    title={formatFull(e.scanned_at)}
                  >
                    {formatAge(e.scanned_at)}
                  </span>

                  {/* Online / Offline */}
                  <span className={`flex items-center gap-1.5 text-xs w-16 flex-shrink-0 ${e.alive ? 'text-green-400' : 'text-slate-500'}`}>
                    <span className={`w-1.5 h-1.5 rounded-full flex-shrink-0 ${e.alive ? 'bg-green-400' : 'bg-slate-600'}`} />
                    {e.alive ? 'Online' : 'Offline'}
                  </span>

                  {/* Port pills */}
                  <div className="flex flex-wrap gap-1 flex-1 min-w-0">
                    {e.open_ports.length > 0
                      ? e.open_ports.map((p) => (
                          <span key={p} className="px-1.5 py-0.5 rounded text-[10px] font-mono bg-slate-700 text-cyan-300">
                            {portLabel(p)}
                          </span>
                        ))
                      : <span className="text-slate-600 text-xs">—</span>
                    }
                  </div>

                  {/* Port-change marker */}
                  {changed && (
                    <span className="text-[10px] text-amber-400 flex-shrink-0 font-medium">
                      ports changed
                    </span>
                  )}
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}
