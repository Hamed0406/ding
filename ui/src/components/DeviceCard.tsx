// One card in the device grid — shows everything we know about a single device.
// Click anywhere on the card to open the full-page history view for that device.
// Hover to reveal the pencil button; click it (or just the pencil) to set a custom name inline.
// Alive devices show a "Scan ports" button that runs an instant targeted port scan.

import { useEffect, useRef, useState } from 'react'
import type { Device } from '../types'
import { scanDevice, wakeDevice } from '../api/client'
import { portLabel } from '../utils/ports'

interface Props {
  device: Device
  isNew?: boolean
  onLabelChange: (ip: string, label: string | null) => void
  onSelect: () => void
}

// Maps device category → [background, text] Tailwind classes.
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

function TypeBadge({ type }: { type: string }) {
  const [bg, text] = TYPE_COLOURS[type] ?? ['bg-slate-700', 'text-slate-400']
  return (
    <span className={`inline-block px-1.5 py-0.5 rounded text-[10px] font-medium ${bg} ${text}`}>
      {type}
    </span>
  )
}

// Minimal pencil SVG — no emoji, matches the dark palette.
function PencilIcon() {
  return (
    <svg viewBox="0 0 16 16" fill="currentColor" className="w-3 h-3">
      <path d="M11.013 1.427a1.75 1.75 0 0 1 2.474 0l1.086 1.086a1.75 1.75 0 0 1 0 2.474l-8.61 8.61c-.21.21-.47.364-.756.445l-3.251.93a.75.75 0 0 1-.927-.928l.929-3.25c.081-.286.235-.547.445-.758l8.61-8.61zm1.414 1.06a.25.25 0 0 0-.354 0L10.811 3.75l1.439 1.44 1.263-1.263a.25.25 0 0 0 0-.354l-1.086-1.086zM11.189 6.25 9.75 4.81 3.23 11.33a.25.25 0 0 0-.064.108l-.618 2.159 2.158-.619a.25.25 0 0 0 .108-.063L11.19 6.25z" />
    </svg>
  )
}

export function DeviceCard({ device, isNew, onLabelChange, onSelect }: Props) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(device.label ?? '')
  // scannedPorts: null = not yet scanned, [] = scan found nothing, [22,80,...] = results
  const [scannedPorts, setScannedPorts] = useState<number[] | null>(null)
  const [scanning, setScanning] = useState(false)
  const [scanError, setScanError] = useState(false)
  const [waking, setWaking] = useState(false)
  const [wakeSent, setWakeSent] = useState(false)
  const [wakeError, setWakeError] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  // Keep draft in sync with external label changes (e.g. from SSE) when not editing.
  useEffect(() => {
    if (!editing) setDraft(device.label ?? '')
  }, [device.label, editing])

  // Focus the input as soon as edit mode opens.
  useEffect(() => {
    if (editing) inputRef.current?.focus()
  }, [editing])

  const startEdit = () => setEditing(true)

  const save = () => {
    const trimmed = draft.trim()
    const current = device.label ?? ''
    if (trimmed !== current) {
      onLabelChange(device.ip, trimmed || null)
    }
    setEditing(false)
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') save()
    if (e.key === 'Escape') {
      setDraft(device.label ?? '')
      setEditing(false)
    }
  }

  const runWake = async (e: React.MouseEvent) => {
    e.stopPropagation()
    if (waking) return
    setWaking(true)
    setWakeError(false)
    try {
      await wakeDevice(device.ip)
      setWakeSent(true)
      setTimeout(() => setWakeSent(false), 3000)
    } catch {
      setWakeError(true)
      setTimeout(() => setWakeError(false), 3000)
    } finally {
      setWaking(false)
    }
  }

  const runScan = async (e: React.MouseEvent) => {
    e.stopPropagation()
    if (scanning) return
    setScanning(true)
    setScanError(false)
    try {
      const result = await scanDevice(device.ip)
      setScannedPorts(result.open_ports)
    } catch {
      setScanError(true)
    } finally {
      setScanning(false)
    }
  }

  return (
    <div
      onClick={() => { if (!editing) onSelect() }}
      className={[
        'rounded-xl p-4 space-y-2 border transition-colors group cursor-pointer',
        isNew
          ? 'bg-green-950/40 border-green-800/50 hover:border-green-700'
          : 'bg-slate-800 border-slate-700 hover:border-slate-600',
      ].join(' ')}
    >
      {/* Row 1: device type badge + alive dot */}
      <div className="flex items-center justify-between gap-2">
        <div className="flex-1 min-w-0">
          {device.device_type && <TypeBadge type={device.device_type} />}
        </div>
        <span
          className={[
            'w-2.5 h-2.5 rounded-full flex-shrink-0',
            device.alive ? 'bg-green-400 animate-pulse' : 'bg-slate-600',
          ].join(' ')}
          title={device.alive ? 'alive' : 'unreachable'}
        />
      </div>

      {/* Row 2: primary name (label or IP) + edit button */}
      {editing ? (
        <input
          ref={inputRef}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={save}
          onKeyDown={handleKeyDown}
          placeholder={device.ip}
          className="w-full bg-slate-700 text-slate-100 text-sm font-semibold rounded px-2 py-1 outline-none focus:ring-1 focus:ring-cyan-500"
        />
      ) : (
        <div className="flex items-center gap-1.5 min-w-0">
          <span
            className={[
              'truncate',
              device.label
                ? 'text-sm font-semibold text-slate-100'
                : 'font-mono text-base font-semibold text-slate-100 tracking-tight',
            ].join(' ')}
          >
            {device.label ?? device.ip}
          </span>
          <button
            onClick={(e) => { e.stopPropagation(); startEdit() }}
            className="opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0 text-slate-500 hover:text-slate-300"
            title="Set custom name"
          >
            <PencilIcon />
          </button>
        </div>
      )}

      {/* IP address — shown as secondary line when a label is set */}
      {device.label && (
        <p className="font-mono text-xs text-slate-500">{device.ip}</p>
      )}

      {/* MAC address */}
      <p className="font-mono text-xs text-slate-400 truncate">
        {device.mac ?? '—'}
      </p>

      {/* Vendor + OS */}
      {(device.vendor || device.os) && (
        <p className="text-xs text-slate-500 truncate">
          {device.vendor}
          {device.vendor && device.os && <span className="text-slate-600"> · </span>}
          {device.os && <span className="text-slate-400">{device.os}</span>}
        </p>
      )}

      {/* Hostname */}
      {device.hostname && (
        <p className="text-xs text-slate-400 truncate">{device.hostname}</p>
      )}

      {/* Open ports — shows live scan result if available, otherwise last known */}
      {(() => {
        const ports = scannedPorts ?? device.open_ports
        return ports.length > 0 ? (
          <div className="flex flex-wrap gap-1 pt-1">
            {ports.map((p) => (
              <span
                key={p}
                className="px-1.5 py-0.5 rounded text-[10px] font-mono bg-slate-700 text-cyan-300"
                title={String(p)}
              >
                {portLabel(p)}
              </span>
            ))}
          </div>
        ) : (
          <p className="text-[10px] text-slate-600 pt-1">
            {scannedPorts !== null ? 'no open ports found' : 'no open ports'}
          </p>
        )
      })()}

      {/* Scan ports button — only shown for online devices */}
      {device.alive && (
        <div className="pt-1 flex items-center gap-2">
          <button
            onClick={runScan}
            disabled={scanning}
            className={[
              'flex items-center gap-1 px-2 py-1 rounded text-[10px] font-medium transition-colors',
              scanning
                ? 'bg-slate-700 text-slate-500 cursor-not-allowed'
                : 'bg-slate-700 hover:bg-cyan-900/60 text-slate-400 hover:text-cyan-300',
            ].join(' ')}
          >
            {scanning ? (
              <>
                <svg className="w-3 h-3 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3">
                  <circle cx="12" cy="12" r="9" strokeOpacity="0.25"/>
                  <path d="M12 3a9 9 0 0 1 9 9" strokeLinecap="round"/>
                </svg>
                Scanning…
              </>
            ) : (
              <>
                <svg viewBox="0 0 16 16" fill="currentColor" className="w-3 h-3">
                  <path fillRule="evenodd" d="M8 1.5a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13zM0 8a8 8 0 1 1 16 0A8 8 0 0 1 0 8zm9 .5H7v-5h2v5zm0 2.5H7v-1.5h2V11z"/>
                </svg>
                Scan ports
              </>
            )}
          </button>
          {scanError && (
            <span className="text-[10px] text-red-400">scan failed</span>
          )}
          {scannedPorts !== null && !scanning && !scanError && (
            <span className="text-[10px] text-slate-500">
              {scannedPorts.length} port{scannedPorts.length !== 1 ? 's' : ''} open
            </span>
          )}
        </div>
      )}

      {/* Wake-on-LAN button — only shown for offline devices with a known MAC */}
      {!device.alive && device.mac && (
        <div className="pt-1 flex items-center gap-2">
          <button
            onClick={runWake}
            disabled={waking}
            className={[
              'flex items-center gap-1 px-2 py-1 rounded text-[10px] font-medium transition-colors',
              waking
                ? 'bg-slate-700 text-slate-500 cursor-not-allowed'
                : wakeSent
                ? 'bg-green-900/50 text-green-400'
                : 'bg-slate-700 hover:bg-violet-900/60 text-slate-400 hover:text-violet-300',
            ].join(' ')}
          >
            {waking ? (
              <>
                <svg className="w-3 h-3 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3">
                  <circle cx="12" cy="12" r="9" strokeOpacity="0.25"/>
                  <path d="M12 3a9 9 0 0 1 9 9" strokeLinecap="round"/>
                </svg>
                Sending…
              </>
            ) : wakeSent ? (
              <>
                <svg viewBox="0 0 16 16" fill="currentColor" className="w-3 h-3">
                  <path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0z"/>
                </svg>
                Sent!
              </>
            ) : (
              <>
                <svg viewBox="0 0 16 16" fill="currentColor" className="w-3 h-3">
                  <path fillRule="evenodd" d="M11.763 3.205A6 6 0 0 1 14 8a6 6 0 0 1-6 6 6 6 0 0 1-6-6 6 6 0 0 1 2.277-4.773.75.75 0 0 1 .963 1.149A4.5 4.5 0 0 0 3.5 8a4.5 4.5 0 0 0 4.5 4.5A4.5 4.5 0 0 0 12.5 8a4.5 4.5 0 0 0-1.719-3.556.75.75 0 0 1 .982-1.139zM8 1a.75.75 0 0 1 .75.75v4.5a.75.75 0 0 1-1.5 0v-4.5A.75.75 0 0 1 8 1z"/>
                </svg>
                Wake
              </>
            )}
          </button>
          {wakeError && (
            <span className="text-[10px] text-red-400">wake failed</span>
          )}
        </div>
      )}

      {/* NEW badge */}
      {isNew && (
        <span className="inline-block px-1.5 py-0.5 rounded text-[10px] font-bold bg-green-800 text-green-300 uppercase tracking-wider">
          new
        </span>
      )}
    </div>
  )
}
