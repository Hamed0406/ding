// Renders all discovered devices as a responsive grid of DeviceCards.

import type { Device } from '../types'
import { DeviceCard } from './DeviceCard'

interface Props {
  devices: Device[]
  newIPs: Set<string>
  onLabelChange: (ip: string, label: string | null) => void
  onSelect: (ip: string) => void
}

function sortByIP(devices: Device[]): Device[] {
  return [...devices].sort((a, b) => {
    const toNum = (ip: string) =>
      ip.split('.').reduce((acc, octet) => acc * 256 + parseInt(octet, 10), 0)
    return toNum(a.ip) - toNum(b.ip)
  })
}

export function DeviceGrid({ devices, newIPs, onLabelChange, onSelect }: Props) {
  if (devices.length === 0) {
    return (
      <p className="text-slate-500 text-sm text-center py-12">
        No devices found yet — run a scan to discover your network.
      </p>
    )
  }

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
      {sortByIP(devices).map((d) => (
        <DeviceCard
          key={d.ip}
          device={d}
          isNew={newIPs.has(d.ip)}
          onLabelChange={onLabelChange}
          onSelect={() => onSelect(d.ip)}
        />
      ))}
    </div>
  )
}
