// Speed test component — runs a full ping/download/upload test on demand.
// Results are persisted on the server and shown in the history table below.

import { useEffect, useState } from 'react'
import { fetchSpeedtestHistory, runSpeedtest } from '../api/client'
import type { SpeedtestResult } from '../types'

// Mbps value that maps to 100% of the bar — anything higher is capped visually.
const BAR_MAX = 200

function SpeedBar({ mbps, color }: { mbps: number; color: string }) {
  const pct = Math.min((mbps / BAR_MAX) * 100, 100)
  return (
    <div className="h-2 rounded-full bg-slate-700 overflow-hidden">
      <div
        className={`h-full rounded-full transition-all duration-700 ${color}`}
        style={{ width: `${pct}%` }}
      />
    </div>
  )
}

function ResultCard({ result }: { result: SpeedtestResult }) {
  return (
    <div className="bg-slate-800 border border-slate-700 rounded-xl p-5 space-y-5">
      <div className="grid grid-cols-3 gap-4 text-center">
        {/* Download */}
        <div>
          <p className="text-xs font-medium text-slate-400 uppercase tracking-wide mb-1">Download</p>
          <p className="text-3xl font-bold text-cyan-400 tabular-nums leading-none">
            {result.download_mbps.toFixed(1)}
          </p>
          <p className="text-xs text-slate-500 mt-0.5">Mbps</p>
        </div>
        {/* Upload */}
        <div>
          <p className="text-xs font-medium text-slate-400 uppercase tracking-wide mb-1">Upload</p>
          <p className="text-3xl font-bold text-violet-400 tabular-nums leading-none">
            {result.upload_mbps.toFixed(1)}
          </p>
          <p className="text-xs text-slate-500 mt-0.5">Mbps</p>
        </div>
        {/* Ping */}
        <div>
          <p className="text-xs font-medium text-slate-400 uppercase tracking-wide mb-1">Ping</p>
          <p className="text-3xl font-bold text-emerald-400 tabular-nums leading-none">
            {result.ping_ms.toFixed(1)}
          </p>
          <p className="text-xs text-slate-500 mt-0.5">ms</p>
        </div>
      </div>

      {/* Bars */}
      <div className="space-y-2">
        <div className="flex items-center gap-3">
          <span className="text-xs text-slate-500 w-16 shrink-0">Download</span>
          <SpeedBar mbps={result.download_mbps} color="bg-cyan-500" />
          <span className="text-xs text-slate-400 w-16 text-right tabular-nums">{result.download_mbps.toFixed(1)} Mbps</span>
        </div>
        <div className="flex items-center gap-3">
          <span className="text-xs text-slate-500 w-16 shrink-0">Upload</span>
          <SpeedBar mbps={result.upload_mbps} color="bg-violet-500" />
          <span className="text-xs text-slate-400 w-16 text-right tabular-nums">{result.upload_mbps.toFixed(1)} Mbps</span>
        </div>
      </div>

      <p className="text-xs text-slate-600 text-right">via {result.server}</p>
    </div>
  )
}

function RunningCard() {
  const [dots, setDots] = useState('.')
  useEffect(() => {
    const t = setInterval(() => setDots((d) => (d.length >= 3 ? '.' : d + '.')), 500)
    return () => clearInterval(t)
  }, [])

  return (
    <div className="bg-slate-800 border border-slate-700 rounded-xl p-8 flex flex-col items-center gap-4">
      <div className="relative w-16 h-16">
        <div className="absolute inset-0 rounded-full border-4 border-slate-700" />
        <div className="absolute inset-0 rounded-full border-4 border-cyan-500 border-t-transparent animate-spin" />
      </div>
      <div className="text-center">
        <p className="text-sm font-medium text-slate-200">Running speed test{dots}</p>
        <p className="text-xs text-slate-500 mt-1">Ping → Download → Upload · takes 10–30 s</p>
      </div>
    </div>
  )
}

export function SpeedTest() {
  const [running, setRunning] = useState(false)
  const [latest, setLatest] = useState<SpeedtestResult | null>(null)
  const [history, setHistory] = useState<SpeedtestResult[]>([])
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    fetchSpeedtestHistory()
      .then((h) => {
        setHistory(h)
        if (h.length > 0) setLatest(h[0])
      })
      .catch(() => {})
  }, [])

  const handleRun = async () => {
    if (running) return
    setRunning(true)
    setError(null)
    try {
      const result = await runSpeedtest()
      setLatest(result)
      setHistory((prev) => [result, ...prev].slice(0, 20))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Speed test failed.')
    } finally {
      setRunning(false)
    }
  }

  return (
    <div className="max-w-lg space-y-6">
      <div>
        <h2 className="text-base font-semibold text-slate-100 mb-1">Internet Speed Test</h2>
        <p className="text-sm text-slate-400">
          Measures your internet connection speed using Cloudflare's speed test servers.
          Tests are run on demand and results are saved for comparison.
        </p>
      </div>

      {/* Run button */}
      <button
        onClick={handleRun}
        disabled={running}
        className="flex items-center gap-2 px-6 py-2.5 rounded-lg bg-cyan-600 hover:bg-cyan-500 disabled:bg-slate-700 disabled:text-slate-500 text-sm font-semibold text-white transition-colors"
      >
        {running ? (
          <>
            <svg className="w-4 h-4 animate-spin" viewBox="0 0 24 24" fill="none">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"/>
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
            </svg>
            Testing…
          </>
        ) : (
          <>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"
              strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4">
              <path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z"/>
            </svg>
            Run Speed Test
          </>
        )}
      </button>

      {/* Error */}
      {error && (
        <p className="text-sm text-red-400">{error}</p>
      )}

      {/* Result or running animation */}
      {running && <RunningCard />}
      {!running && latest && <ResultCard result={latest} />}

      {/* History table */}
      {history.length > 1 && (
        <div>
          <h3 className="text-xs font-medium text-slate-400 uppercase tracking-wide mb-3">
            History
          </h3>
          <div className="rounded-xl overflow-hidden border border-slate-700">
            <table className="w-full text-xs text-slate-300">
              <thead>
                <tr className="bg-slate-800 text-slate-500 text-left">
                  <th className="px-3 py-2 font-medium">When</th>
                  <th className="px-3 py-2 font-medium text-right">Down</th>
                  <th className="px-3 py-2 font-medium text-right">Up</th>
                  <th className="px-3 py-2 font-medium text-right">Ping</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-700/50">
                {history.map((r, i) => (
                  <tr key={i} className="bg-slate-800/40 hover:bg-slate-800 transition-colors">
                    <td className="px-3 py-2 text-slate-400">
                      {new Date(r.tested_at).toLocaleString([], {
                        month: 'short', day: 'numeric',
                        hour: '2-digit', minute: '2-digit',
                      })}
                    </td>
                    <td className="px-3 py-2 text-right tabular-nums text-cyan-400">
                      {r.download_mbps.toFixed(1)}
                    </td>
                    <td className="px-3 py-2 text-right tabular-nums text-violet-400">
                      {r.upload_mbps.toFixed(1)}
                    </td>
                    <td className="px-3 py-2 text-right tabular-nums text-emerald-400">
                      {r.ping_ms.toFixed(0)} ms
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <p className="text-xs text-slate-600 mt-2 text-right">Down/Up in Mbps</p>
        </div>
      )}
    </div>
  )
}
