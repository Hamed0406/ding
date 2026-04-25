// ============================================================
// ui/src/api/client.ts — Functions that talk to the Go server
//
// These are thin wrappers around the browser's `fetch` API.
// Each function maps to one REST endpoint on the Go server.
// ============================================================

import type { Device, HistoryEntry, Status } from '../types'

// Generic helper: fetch a URL and parse the response as JSON.
// Throws an error if the HTTP status is not OK (e.g. 404, 500).
async function get<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`)
  return res.json() as Promise<T>
}

// GET /api/status — returns interface name, subnet, and last scan time
export const fetchStatus = () => get<Status>('/api/status')

// GET /api/devices — returns the device list from the most recent scan
export const fetchDevices = () => get<Device[]>('/api/devices')

// GET /api/history — returns the last 20 scan records
export const fetchHistory = () => get<HistoryEntry[]>('/api/history')

// POST /api/scan — asks the server to start a new scan right now.
// The server responds with 202 (Accepted) immediately.
// The actual results arrive later via the SSE stream (useEvents hook).
export async function triggerScan(): Promise<void> {
  const res = await fetch('/api/scan', { method: 'POST' })
  // 202 = scan started, 409 = already scanning (both are fine)
  if (res.status !== 202 && res.status !== 409) {
    throw new Error(`scan trigger failed: HTTP ${res.status}`)
  }
}
