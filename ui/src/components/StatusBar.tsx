// Shows the current network interface, subnet, and how long ago the last scan ran.
// Displayed in the top-right corner of the header.

import type { Status } from '../types'

// Convert an ISO timestamp (e.g. "2026-04-25T18:00:00Z") into a human-readable
// relative time string (e.g. "2m ago", "1h ago").
function relativeTime(iso: string): string {
  const diffSeconds = Math.floor((Date.now() - new Date(iso).getTime()) / 1000)
  if (diffSeconds < 60)   return `${diffSeconds}s ago`
  if (diffSeconds < 3600) return `${Math.floor(diffSeconds / 60)}m ago`
  return `${Math.floor(diffSeconds / 3600)}h ago`
}

interface Props {
  status: Status | null // null while the initial data is still loading
}

export function StatusBar({ status }: Props) {
  // Show a placeholder while waiting for the first API response
  if (!status) return <div className="text-slate-500 text-sm">connecting…</div>

  return (
    <div className="flex flex-col items-end text-xs text-slate-400 leading-tight">
      {/* Interface name and subnet */}
      <span>
        <span className="text-slate-500">iface </span>
        <span className="text-slate-300 font-mono">{status.iface}</span>
        <span className="text-slate-500 ml-2">subnet </span>
        <span className="text-slate-300 font-mono">{status.subnet}</span>
      </span>
      {/* Last scan time — only shown after the first scan has run */}
      {status.last_scan && (
        <span className="text-slate-500">last scan {relativeTime(status.last_scan)}</span>
      )}
    </div>
  )
}
