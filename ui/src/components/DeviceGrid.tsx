// Renders all discovered devices as a responsive grid of DeviceCards.
// On mobile: 1 column. On tablet: 2 columns. On desktop: 3–4 columns.

import type { Device } from '../types'
import { DeviceCard } from './DeviceCard'

interface Props {
  devices: Device[]
  newIPs: Set<string> // set of IPs that are new this scan (used to show the green badge)
}

// Sort devices by IP address numerically (192.168.1.2 before 192.168.1.10)
// Plain alphabetical sorting would put 192.168.1.10 before 192.168.1.2
function sortByIP(devices: Device[]): Device[] {
  return [...devices].sort((a, b) => {
    // Convert "192.168.1.42" → a single number for easy comparison
    const toNum = (ip: string) =>
      ip.split('.').reduce((acc, octet) => acc * 256 + parseInt(octet, 10), 0)
    return toNum(a.ip) - toNum(b.ip)
  })
}

export function DeviceGrid({ devices, newIPs }: Props) {
  // Show a friendly message when no scan has run yet
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
          isNew={newIPs.has(d.ip)} // pass true if this IP is in the new-devices set
        />
      ))}
    </div>
  )
}
