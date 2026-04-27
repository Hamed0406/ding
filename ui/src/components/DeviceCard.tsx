// One card in the device grid — shows everything we know about a single device.
// Hover the card to reveal the edit button; click it to set a custom name.

import { useEffect, useRef, useState } from 'react'
import type { Device } from '../types'
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

      {/* Vendor */}
      {device.vendor && (
        <p className="text-xs text-slate-500 truncate">{device.vendor}</p>
      )}

      {/* Hostname */}
      {device.hostname && (
        <p className="text-xs text-slate-400 truncate">{device.hostname}</p>
      )}

      {/* Open ports */}
      {device.open_ports.length > 0 ? (
        <div className="flex flex-wrap gap-1 pt-1">
          {device.open_ports.map((p) => (
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
        <p className="text-[10px] text-slate-600 pt-1">no open ports</p>
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
