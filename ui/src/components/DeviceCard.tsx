// One card in the device grid — shows everything we know about a single device.

import type { Device } from '../types'

interface Props {
  device: Device
  isNew?: boolean // true if this device appeared for the first time in the last scan
}

export function DeviceCard({ device, isNew }: Props) {
  return (
    <div
      className={[
        'rounded-xl p-4 space-y-2 border transition-colors',
        // New devices get a green tint; known devices get the default dark card
        isNew
          ? 'bg-green-950/40 border-green-800/50'
          : 'bg-slate-800 border-slate-700',
      ].join(' ')}
    >
      {/* Top row: IP address on the left, alive indicator dot on the right */}
      <div className="flex items-center justify-between">
        <span className="font-mono text-base font-semibold text-slate-100 tracking-tight">
          {device.ip}
        </span>
        <span
          className={[
            'w-2.5 h-2.5 rounded-full',
            // Green pulsing dot = alive, grey dot = not responding
            device.alive ? 'bg-green-400 animate-pulse' : 'bg-slate-600',
          ].join(' ')}
          title={device.alive ? 'alive' : 'unreachable'}
        />
      </div>

      {/* MAC address (hardware address) — shows "—" if we couldn't get it */}
      <p className="font-mono text-xs text-slate-400 truncate">
        {device.mac ?? '—'}
      </p>

      {/* Vendor (manufacturer) — looked up from MAC, only shown if known */}
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
            >
              {p}
            </span>
          ))}
        </div>
      ) : (
        <p className="text-[10px] text-slate-600 pt-1">no open ports</p>
      )}

      {/* "NEW" badge — only shown when the device appeared for the first time */}
      {isNew && (
        <span className="inline-block px-1.5 py-0.5 rounded text-[10px] font-bold bg-green-800 text-green-300 uppercase tracking-wider">
          new
        </span>
      )}
    </div>
  )
}
