// ============================================================
// ui/src/api/client.ts — Functions that talk to the Go server
//
// Each function maps to one REST endpoint:
//
//   fetchStatus()              GET /api/status
//   fetchDevices()             GET /api/devices
//   fetchDeviceHistory(ip)     GET /api/devices/{ip}/history
//   fetchHistory()             GET /api/history
//   fetchTopology()            GET /api/topology
//   triggerScan()              POST /api/scan
//   scanDevice(ip)             POST /api/devices/{ip}/scan
//   setDeviceLabel(ip, name)   PUT /api/devices/{ip}/label
//   deleteDeviceLabel(ip)      DELETE /api/devices/{ip}/label
//
// Auth strategy:
//   The server sets an HttpOnly session cookie AND returns the token in the
//   response body. The token is stored in localStorage so it can be sent as
//   Authorization: Bearer on every request. This dual approach works when
//   Cloudflare Tunnel strips Set-Cookie headers from certain responses.
// ============================================================

import type { ARPWatchEntry, ChangeLogEntry, Device, DeviceHistoryEntry, EmailConfig, HistoryEntry, SpeedtestResult, Status, TopologyGraph } from '../types'

// Thrown when any API call gets a 401 — lets App.tsx redirect to the login page.
export class AuthError extends Error {
  constructor() { super('unauthorized') }
}

const TOKEN_KEY = 'ding_session'

export function saveToken(token: string) { localStorage.setItem(TOKEN_KEY, token) }
export function getToken(): string | null { return localStorage.getItem(TOKEN_KEY) }
export function clearToken() { localStorage.removeItem(TOKEN_KEY) }

// Returns Authorization header if we have a stored token.
function authHeaders(): Record<string, string> {
  const token = getToken()
  return token ? { Authorization: `Bearer ${token}` } : {}
}

// Generic GET: sends auth header, throws AuthError on 401.
async function get<T>(path: string): Promise<T> {
  const res = await fetch(path, { headers: authHeaders() })
  if (res.status === 401) { clearToken(); throw new AuthError() }
  if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`)
  return res.json() as Promise<T>
}

// Generic POST/PUT/DELETE helper with auth header.
async function send(method: string, path: string, body?: unknown): Promise<Response> {
  const headers: Record<string, string> = { ...authHeaders() }
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  return fetch(path, { method, headers, body: body !== undefined ? JSON.stringify(body) : undefined })
}

// GET /api/status
export const fetchStatus = () => get<Status>('/api/status')

// GET /api/devices
export const fetchDevices = () => get<Device[]>('/api/devices')

// GET /api/devices/{ip}/history
export const fetchDeviceHistory = (ip: string) =>
  get<DeviceHistoryEntry[]>(`/api/devices/${encodeURIComponent(ip)}/history`)

// GET /api/history
export const fetchHistory = () => get<HistoryEntry[]>('/api/history')

// GET /api/topology
export const fetchTopology = () => get<TopologyGraph>('/api/topology')

// POST /api/scan
export async function triggerScan(): Promise<void> {
  const res = await send('POST', '/api/scan')
  if (res.status !== 202 && res.status !== 409) {
    throw new Error(`scan trigger failed: HTTP ${res.status}`)
  }
}

// POST /api/devices/{ip}/scan
export async function scanDevice(ip: string): Promise<{ ip: string; open_ports: number[] }> {
  const res = await send('POST', `/api/devices/${encodeURIComponent(ip)}/scan`)
  if (!res.ok) throw new Error(`scanDevice: HTTP ${res.status}`)
  return res.json()
}

// GET /api/auth/providers — public endpoint, no auth needed
export const fetchAuthProviders = () =>
  fetch('/api/auth/providers').then((r) => r.json()) as Promise<{ google: boolean; github: boolean; has_users: boolean }>

// POST /api/auth/register
export async function register(email: string, password: string): Promise<void> {
  const res = await fetch('/api/auth/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(body.error ?? `register: HTTP ${res.status}`)
  }
  const data = await res.json().catch(() => ({})) as { token?: string }
  if (data.token) saveToken(data.token)
}

// POST /api/auth/login
export async function login(email: string, password: string): Promise<void> {
  const res = await fetch('/api/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(body.error ?? `login: HTTP ${res.status}`)
  }
  const data = await res.json().catch(() => ({})) as { token?: string }
  if (data.token) saveToken(data.token)
}

// POST /api/auth/logout
export async function logout(): Promise<void> {
  await send('POST', '/api/auth/logout')
  clearToken()
}

// POST /api/auth/exchange — consume a one-time OAuth exchange token and start a session.
export async function exchangeToken(token: string): Promise<void> {
  const res = await fetch('/api/auth/exchange', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token }),
  })
  if (!res.ok) {
    const body = await res.text().catch(() => '')
    throw new Error(`exchange failed (${res.status}): ${body}`)
  }
  const data = await res.json() as { token?: string }
  if (data.token) {
    saveToken(data.token)
  } else {
    throw new Error('exchange response missing token')
  }
}

// POST /api/devices/{ip}/wake
export async function wakeDevice(ip: string): Promise<void> {
  const res = await send('POST', `/api/devices/${encodeURIComponent(ip)}/wake`)
  if (!res.ok) throw new Error(`wakeDevice: HTTP ${res.status}`)
}

// GET /api/settings/telegram
export const fetchTelegramSettings = () =>
  get<{ token_set: boolean; token_preview: string; chat_id: string }>('/api/settings/telegram')

// PUT /api/settings/telegram
export async function saveTelegramSettings(token: string, chatId: string): Promise<void> {
  const res = await send('PUT', '/api/settings/telegram', { token, chat_id: chatId })
  if (!res.ok) {
    const body = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(body.error ?? `save failed: HTTP ${res.status}`)
  }
}

// POST /api/settings/telegram/test
export async function testTelegramSettings(): Promise<void> {
  const res = await send('POST', '/api/settings/telegram/test')
  if (!res.ok) {
    const body = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(body.error ?? `test failed: HTTP ${res.status}`)
  }
}

// GET /api/settings/webhook
export const fetchWebhookSettings = () =>
  get<{ url: string; url_set: boolean }>('/api/settings/webhook')

// PUT /api/settings/webhook
export async function saveWebhookSettings(url: string): Promise<void> {
  const res = await send('PUT', '/api/settings/webhook', { url })
  if (!res.ok) {
    const body = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(body.error ?? `save failed: HTTP ${res.status}`)
  }
}

// POST /api/settings/webhook/test
export async function testWebhookSettings(): Promise<void> {
  const res = await send('POST', '/api/settings/webhook/test')
  if (!res.ok) {
    const body = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(body.error ?? `test failed: HTTP ${res.status}`)
  }
}

// PUT /api/devices/{ip}/notify
export async function setDeviceNotify(ip: string, enabled: boolean): Promise<void> {
  const res = await send('PUT', `/api/devices/${encodeURIComponent(ip)}/notify`, { enabled })
  if (!res.ok) throw new Error(`setDeviceNotify: HTTP ${res.status}`)
}

// PUT /api/devices/{ip}/label
export async function setDeviceLabel(ip: string, name: string): Promise<void> {
  const res = await send('PUT', `/api/devices/${encodeURIComponent(ip)}/label`, { name })
  if (!res.ok) throw new Error(`setDeviceLabel: HTTP ${res.status}`)
}

// DELETE /api/devices/{ip}/label
export async function deleteDeviceLabel(ip: string): Promise<void> {
  const res = await send('DELETE', `/api/devices/${encodeURIComponent(ip)}/label`)
  if (!res.ok) throw new Error(`deleteDeviceLabel: HTTP ${res.status}`)
}

// POST /api/speedtest — runs a full speed test (takes 5–30 s)
export async function runSpeedtest(): Promise<SpeedtestResult> {
  const res = await send('POST', '/api/speedtest')
  if (res.status === 409) throw new Error('A speed test is already running.')
  if (!res.ok) {
    const body = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(body.error ?? `speedtest: HTTP ${res.status}`)
  }
  return res.json() as Promise<SpeedtestResult>
}

// GET /api/speedtest/history — last 20 results, newest first
export const fetchSpeedtestHistory = () => get<SpeedtestResult[]>('/api/speedtest/history')

// GET /api/changes — last N change-log entries, newest first (default 200)
export const fetchChangeLog = (limit = 200) =>
  get<ChangeLogEntry[]>(`/api/changes?limit=${limit}`)

// GET /api/arpwatch — IPs that have been seen with more than one distinct MAC
export const fetchARPWatch = () => get<ARPWatchEntry[]>('/api/arpwatch')

// GET /api/settings/email
export const fetchEmailSettings = () => get<EmailConfig>('/api/settings/email')

// PUT /api/settings/email
export async function saveEmailSettings(cfg: {
  host: string; port: number; username: string; password: string; from: string; to: string
}): Promise<void> {
  const res = await send('PUT', '/api/settings/email', cfg)
  if (!res.ok) {
    const body = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(body.error ?? `save failed: HTTP ${res.status}`)
  }
}

// POST /api/settings/email/test
export async function testEmailSettings(): Promise<void> {
  const res = await send('POST', '/api/settings/email/test')
  if (!res.ok) {
    const body = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(body.error ?? `test failed: HTTP ${res.status}`)
  }
}

// GET /api/devices/export — trigger a browser download of all devices
// format: 'csv' (default) or 'json'
export function exportDevices(format: 'csv' | 'json' = 'csv'): void {
  const token = getToken()
  const tokenParam = token ? `&token=${encodeURIComponent(token)}` : ''
  // Use window.location to trigger a real download (not fetch) so the browser
  // shows the Save-As dialog / downloads to the Downloads folder automatically.
  window.location.href = `/api/devices/export?format=${format}${tokenParam}`
}
