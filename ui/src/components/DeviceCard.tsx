// One card in the device grid — shows everything we know about a single device.

import type { Device } from '../types'
import { portLabel } from '../utils/ports'

interface Props {
  device: Device
  isNew?: boolean // true if this device appeared for the first time in the last scan
}

// Maps device category to a Tailwind colour pair [bg, text].
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

export function DeviceCard({ device, isNew }: Props) {
  return (
    <div
      className={[
        'rounded-xl p-4 space-y-2 border transition-colors',
        isNew
          ? 'bg-green-950/40 border-green-800/50'
          : 'bg-slate-800 border-slate-700',
      ].join(' ')}
    >
      {/* Top row: device type badge (if known) + alive indicator dot */}
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

      {/* IP address */}
      <span className="font-mono text-base font-semibold text-slate-100 tracking-tight block">
        {device.ip}
      </span>

      {/* MAC address — shows "—" if we couldn't get it */}
      <p className="font-mono text-xs text-slate-400 truncate">
        {device.mac ?? '—'}
      </p>

      {/* Vendor (manufacturer) — only shown if known */}
      {device.vendor && (
        <p className="text-xs text-slate-500 truncate">{device.vendor}</p>
      )}

      {/* Hostname — only shown if we have one */}
      {device.hostname && (
        <p className="text-xs text-slate-400 truncate">{device.hostname}</p>
      )}

      {/* Open ports as small coloured pills — or a grey message if none found */}
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

      {/* "NEW" badge */}
      {isNew && (
        <span className="inline-block px-1.5 py-0.5 rounded text-[10px] font-bold bg-green-800 text-green-300 uppercase tracking-wider">
          new
        </span>
      )}
    </div>
  )
}
