// Full-page persistent change log — all network events stored in SQLite,
// newest first. Filterable by event kind. Loads on mount; refreshes after
// each scan via the onRefresh prop.

import { useEffect, useState } from 'react'
import { fetchChangeLog } from '../api/client'
import type { ChangeLogEntry } from '../types'

type KindFilter = 'ALL' | 'NEW' | 'BACK' | 'GONE' | 'PORTS'

const kindStyle: Record<string, string> = {
  NEW:   'border-green-500 bg-green-950/30',
  BACK:  'border-blue-500  bg-blue-950/30',
  GONE:  'border-red-500   bg-red-950/30',
  PORTS: 'border-amber-500 bg-amber-950/30',
}

const kindBadge: Record<string, string> = {
  NEW:   'bg-green-800 text-green-300',
  BACK:  'bg-blue-800  text-blue-300',
  GONE:  'bg-red-800   text-red-300',
  PORTS: 'bg-amber-800 text-amber-300',
}

const kindLabel: Record<string, string> = {
  NEW:   'New device',
  BACK:  'Device returned',
  GONE:  'Device left',
  PORTS: 'Ports changed',
}

function formatTime(iso: string): string {
  const d = new Date(iso)
  return d.toLocaleString(undefined, {
    month: 'short', day: 'numeric',
    hour: '2-digit', minute: '2-digit', second: '2-digit',
  })
}

interface Props {
  scanCount: number  // bumped after each scan so the log auto-refreshes
}

export function ChangeLog({ scanCount }: Props) {
  const [entries, setEntries] = useState<ChangeLogEntry[]>([])
  const [filter, setFilter] = useState<KindFilter>('ALL')
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    setLoading(true)
    fetchChangeLog(200)
      .then(setEntries)
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [scanCount])

  const visible = filter === 'ALL' ? entries : entries.filter((e) => e.kind === filter)

  const counts = entries.reduce<Record<string, number>>((acc, e) => {
    acc[e.kind] = (acc[e.kind] ?? 0) + 1
    return acc
  }, {})

  return (
    <div className="space-y-4">
      {/* Header + filter pills */}
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="text-sm font-semibold text-slate-300 mr-2">Event log</h2>
        {(['ALL', 'NEW', 'BACK', 'GONE', 'PORTS'] as KindFilter[]).map((k) => {
          const count = k === 'ALL' ? entries.length : (counts[k] ?? 0)
          return (
            <button
              key={k}
              onClick={() => setFilter(k)}
              className={[
                'px-2.5 py-1 rounded-full text-xs font-medium transition-colors',
                filter === k
                  ? 'bg-cyan-700 text-cyan-100'
                  : 'bg-slate-800 text-slate-400 hover:bg-slate-700 hover:text-slate-200',
              ].join(' ')}
            >
              {k === 'ALL' ? 'All' : k}
              {count > 0 && (
                <span className="ml-1.5 opacity-70">{count}</span>
              )}
            </button>
          )
        })}
        <span className="ml-auto text-xs text-slate-600">last 200 events</span>
      </div>

      {/* Entry list */}
      {loading ? (
        <p className="text-slate-500 text-sm text-center py-8">Loading…</p>
      ) : visible.length === 0 ? (
        <p className="text-slate-500 text-sm text-center py-8">
          {entries.length === 0
            ? 'No events yet — run a scan to start recording changes.'
            : `No ${filter} events recorded.`}
        </p>
      ) : (
        <div className="space-y-1.5">
          {visible.map((e, i) => (
            <div
              key={i}
              className={`flex items-start gap-3 rounded-lg px-3 py-2 border-l-4 ${kindStyle[e.kind] ?? 'border-slate-600 bg-slate-800'}`}
            >
              {/* Kind badge */}
              <span
                className={`mt-0.5 px-1.5 py-0.5 rounded text-[10px] font-bold uppercase tracking-wider shrink-0 ${kindBadge[e.kind] ?? 'bg-slate-700 text-slate-300'}`}
              >
                {e.kind}
              </span>

              {/* IP + description */}
              <div className="min-w-0 flex-1">
                <div className="flex items-baseline gap-2 flex-wrap">
                  <span className="font-mono text-sm text-slate-200">{e.ip}</span>
                  <span className="text-xs text-slate-500">{kindLabel[e.kind] ?? e.kind}</span>
                </div>
                {e.desc && (
                  <p className="text-xs text-slate-400 truncate mt-0.5">{e.desc}</p>
                )}
              </div>

              {/* Timestamp */}
              <time
                className="shrink-0 text-xs text-slate-500 tabular-nums mt-0.5"
                dateTime={e.occurred_at}
              >
                {formatTime(e.occurred_at)}
              </time>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
