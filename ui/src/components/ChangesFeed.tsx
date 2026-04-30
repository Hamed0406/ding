// Shows what changed between the last two scans.
// Each change is colour-coded:
//   🟢 NEW   — a device joined the network for the first time ever
//   🔵 BACK  — a known device returned after being absent
//   🔴 GONE  — a device left the network
//   🟡 PORTS — a device's open ports changed

import type { Change } from '../types'

// Tailwind classes for the left border + background of each change type
const kindStyle: Record<string, string> = {
  NEW:        'border-green-500  bg-green-950/30',
  BACK:       'border-blue-500   bg-blue-950/30',
  GONE:       'border-red-500    bg-red-950/30',
  PORTS:      'border-amber-500  bg-amber-950/30',
  MAC_CHANGE: 'border-orange-500 bg-orange-950/40',
}

// Tailwind classes for the small badge label
const kindBadge: Record<string, string> = {
  NEW:        'bg-green-800  text-green-300',
  BACK:       'bg-blue-800   text-blue-300',
  GONE:       'bg-red-800    text-red-300',
  PORTS:      'bg-amber-800  text-amber-300',
  MAC_CHANGE: 'bg-orange-800 text-orange-200',
}

interface Props {
  changes: Change[]
}

export function ChangesFeed({ changes }: Props) {
  // Don't render anything if there are no changes
  if (changes.length === 0) return null

  return (
    <div className="space-y-1.5">
      {/* Section label */}
      <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-500">
        Changes
      </h2>

      {/* Scrollable list — max height 12rem to avoid taking over the whole screen */}
      <div className="space-y-1.5 max-h-48 overflow-y-auto pr-1">
        {changes.map((c, i) => (
          <div
            key={`${c.kind}-${c.ip}-${i}`}
            className={`flex items-start gap-3 rounded-lg px-3 py-2 border-l-4 ${kindStyle[c.kind] ?? 'border-slate-600 bg-slate-800'}`}
          >
            {/* Coloured badge: NEW / GONE / PORTS */}
            <span
              className={`mt-0.5 px-1.5 py-0.5 rounded text-[10px] font-bold uppercase tracking-wider shrink-0 ${kindBadge[c.kind] ?? 'bg-slate-700 text-slate-300'}`}
            >
              {c.kind}
            </span>

            {/* IP address and description */}
            <div className="min-w-0">
              <p className="font-mono text-sm text-slate-200">{c.ip}</p>
              <p className="text-xs text-slate-400 truncate">{c.desc}</p>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
