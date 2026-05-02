// Full-page settings view. Reached via the gear icon in the main header.
// Left sidebar lists settings sections; right panel shows the active section.
// Add new sections by appending to SECTIONS and adding a matching panel below.

import { useEffect, useState } from 'react'
import {
  fetchTelegramSettings, saveTelegramSettings, testTelegramSettings,
  fetchWebhookSettings, saveWebhookSettings, testWebhookSettings,
} from '../api/client'
import { EmailSettings } from './EmailSettings'
import { SpeedTest } from './SpeedTest'

interface Props {
  onBack: () => void
}

type Section = 'notifications' | 'email' | 'webhooks' | 'speedtest'

const SECTIONS: { id: Section; label: string; icon: React.ReactNode }[] = [
  {
    id: 'notifications',
    label: 'Telegram',
    icon: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"
        strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4">
        <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/>
        <path d="M13.73 21a2 2 0 0 1-3.46 0"/>
      </svg>
    ),
  },
  {
    id: 'email',
    label: 'Email',
    icon: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"
        strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4">
        <path d="M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z"/>
        <polyline points="22,6 12,13 2,6"/>
      </svg>
    ),
  },
  {
    id: 'webhooks',
    label: 'Webhooks',
    icon: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"
        strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4">
        <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/>
        <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>
      </svg>
    ),
  },
  {
    id: 'speedtest',
    label: 'Speed Test',
    icon: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"
        strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4">
        <path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z"/>
      </svg>
    ),
  },
]

export function SettingsPage({ onBack }: Props) {
  const [active, setActive] = useState<Section>('notifications')

  return (
    <div className="min-h-screen bg-slate-900 text-slate-100">

      {/* Top bar */}
      <header className="sticky top-0 z-10 bg-slate-900/90 backdrop-blur border-b border-slate-800 px-4 py-3">
        <div className="max-w-5xl mx-auto flex items-center gap-3">
          <button
            onClick={onBack}
            className="flex items-center gap-1.5 text-sm text-slate-400 hover:text-slate-100 transition-colors"
          >
            <svg viewBox="0 0 16 16" fill="currentColor" className="w-4 h-4">
              <path fillRule="evenodd" d="M7.78 12.53a.75.75 0 0 1-1.06 0L2.47 8.28a.75.75 0 0 1 0-1.06l4.25-4.25a.75.75 0 0 1 1.06 1.06L4.81 7h7.44a.75.75 0 0 1 0 1.5H4.81l2.97 2.97a.75.75 0 0 1 0 1.06z"/>
            </svg>
            Back
          </button>
          <span className="text-slate-600">|</span>
          <h1 className="text-sm font-semibold text-slate-200">Settings</h1>
        </div>
      </header>

      <div className="max-w-5xl mx-auto px-4 py-8 flex gap-8">

        {/* Sidebar */}
        <nav className="w-48 flex-shrink-0">
          <ul className="space-y-0.5">
            {SECTIONS.map((s) => (
              <li key={s.id}>
                <button
                  onClick={() => setActive(s.id)}
                  className={[
                    'w-full flex items-center gap-2.5 px-3 py-2 rounded-lg text-sm transition-colors text-left',
                    active === s.id
                      ? 'bg-slate-700 text-slate-100'
                      : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800',
                  ].join(' ')}
                >
                  {s.icon}
                  {s.label}
                </button>
              </li>
            ))}
          </ul>
        </nav>

        {/* Content */}
        <div className="flex-1 min-w-0">
          {active === 'notifications' && <TelegramSection />}
          {active === 'email' && <EmailSettings />}
          {active === 'webhooks' && <WebhookSection />}
          {active === 'speedtest' && <SpeedTest />}
        </div>

      </div>
    </div>
  )
}

// ─── Telegram section ────────────────────────────────────────────────────────

function TelegramSection() {
  const [token, setToken] = useState('')
  const [chatId, setChatId] = useState('')
  const [tokenSet, setTokenSet] = useState(false)
  const [tokenPreview, setTokenPreview] = useState('')
  const [editingToken, setEditingToken] = useState(false)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [status, setStatus] = useState<{ ok: boolean; msg: string } | null>(null)

  useEffect(() => {
    fetchTelegramSettings()
      .then((s) => {
        setTokenSet(s.token_set)
        setTokenPreview(s.token_preview)
        setChatId(s.chat_id)
      })
      .catch(() => {})
  }, [])

  const handleSave = async () => {
    if (saving) return
    setSaving(true)
    setStatus(null)
    try {
      // Send the typed token when creating or changing; empty string tells the
      // backend to keep the existing token (so chat ID can be updated alone).
      const tokenToSend = (tokenSet && !editingToken) ? '' : token
      await saveTelegramSettings(tokenToSend, chatId)
      setStatus({ ok: true, msg: 'Settings saved.' })
      if (!tokenSet || editingToken) {
        if (token) {
          setTokenPreview(token.slice(0, 10) + '•'.repeat(Math.max(0, token.length - 10)))
          setTokenSet(true)
        } else {
          setTokenSet(false)
          setTokenPreview('')
        }
        setEditingToken(false)
        setToken('')
      }
    } catch (err) {
      setStatus({ ok: false, msg: err instanceof Error ? err.message : 'Save failed.' })
    } finally {
      setSaving(false)
    }
  }

  const handleTest = async () => {
    if (testing) return
    setTesting(true)
    setStatus(null)
    try {
      await testTelegramSettings()
      setStatus({ ok: true, msg: 'Test message sent — check your Telegram.' })
    } catch (err) {
      setStatus({ ok: false, msg: err instanceof Error ? err.message : 'Test failed.' })
    } finally {
      setTesting(false)
    }
  }

  const handleClear = async () => {
    setSaving(true)
    setStatus(null)
    try {
      await saveTelegramSettings('', '')
      setTokenSet(false)
      setTokenPreview('')
      setChatId('')
      setToken('')
      setEditingToken(false)
      setStatus({ ok: true, msg: 'Telegram config cleared.' })
    } catch {
      setStatus({ ok: false, msg: 'Failed to clear.' })
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="max-w-lg space-y-6">

      <div>
        <h2 className="text-base font-semibold text-slate-100 mb-1">Telegram Notifications</h2>
        <p className="text-sm text-slate-400">
          Receive a Telegram message when a device you're watching joins, leaves, or changes ports.
          Notifications are opt-in per device — enable them from the bell icon on each device card.
        </p>
      </div>

      <div className="bg-slate-800 border border-slate-700 rounded-xl p-5 space-y-4">

        {/* Bot Token */}
        <div className="space-y-1.5">
          <label className="block text-xs font-medium text-slate-400 uppercase tracking-wide">
            Bot Token
          </label>
          <p className="text-xs text-slate-500">
            Create a bot via <span className="text-slate-300">@BotFather</span> on Telegram and paste the token here.
          </p>
          {tokenSet && !editingToken ? (
            <div className="flex items-center gap-2">
              <span className="flex-1 font-mono text-sm text-slate-400 bg-slate-700/50 border border-slate-600 rounded-lg px-3 py-2 truncate">
                {tokenPreview}
              </span>
              <button
                onClick={() => { setEditingToken(true); setToken('') }}
                className="text-xs text-cyan-500 hover:text-cyan-400 transition-colors flex-shrink-0"
              >
                Change
              </button>
            </div>
          ) : (
            <input
              type="password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder="123456789:ABCdef…"
              autoComplete="off"
              className="w-full bg-slate-700 border border-slate-600 rounded-lg px-3 py-2 text-sm text-slate-100 placeholder-slate-500 outline-none focus:ring-1 focus:ring-cyan-500 focus:border-cyan-500 font-mono"
            />
          )}
        </div>

        {/* Chat ID */}
        <div className="space-y-1.5">
          <label className="block text-xs font-medium text-slate-400 uppercase tracking-wide">
            Chat ID
          </label>
          <p className="text-xs text-slate-500">
            Your personal chat ID or a group/channel ID. Get it from <span className="text-slate-300">@userinfobot</span>.
          </p>
          <input
            type="text"
            value={chatId}
            onChange={(e) => setChatId(e.target.value)}
            placeholder="987654321"
            autoComplete="off"
            className="w-full bg-slate-700 border border-slate-600 rounded-lg px-3 py-2 text-sm text-slate-100 placeholder-slate-500 outline-none focus:ring-1 focus:ring-cyan-500 focus:border-cyan-500 font-mono"
          />
        </div>

        {/* Status */}
        {status && (
          <p className={`text-sm ${status.ok ? 'text-green-400' : 'text-red-400'}`}>
            {status.msg}
          </p>
        )}

        {/* Actions */}
        <div className="flex items-center gap-3 pt-1">
          <button
            onClick={handleSave}
            disabled={saving}
            className="px-5 py-2 rounded-lg bg-cyan-600 hover:bg-cyan-500 disabled:bg-slate-700 disabled:text-slate-500 text-sm font-semibold text-white transition-colors"
          >
            {saving ? 'Saving…' : 'Save'}
          </button>
          <button
            onClick={handleTest}
            disabled={testing || !tokenSet}
            title={!tokenSet ? 'Save your token first' : 'Send a test message to Telegram'}
            className="px-5 py-2 rounded-lg bg-slate-700 hover:bg-slate-600 disabled:opacity-40 disabled:cursor-not-allowed text-sm font-medium text-slate-200 transition-colors"
          >
            {testing ? 'Sending…' : 'Send test message'}
          </button>
          {tokenSet && (
            <button
              onClick={handleClear}
              disabled={saving}
              className="ml-auto text-xs text-slate-500 hover:text-red-400 transition-colors"
            >
              Clear
            </button>
          )}
        </div>

      </div>

      {/* How-to hint */}
      <div className="text-xs text-slate-500 space-y-1">
        <p><span className="text-slate-400 font-medium">1.</span> Message @BotFather → /newbot → copy the token above.</p>
        <p><span className="text-slate-400 font-medium">2.</span> Message your new bot once, then paste your chat ID from @userinfobot.</p>
        <p><span className="text-slate-400 font-medium">3.</span> Click <span className="text-slate-300">Send test message</span> to confirm it works.</p>
        <p><span className="text-slate-400 font-medium">4.</span> Enable the bell icon on the devices you want to track.</p>
      </div>

    </div>
  )
}

// ─── Webhook section ─────────────────────────────────────────────────────────

function WebhookSection() {
  const [url, setUrl] = useState('')
  const [urlSet, setUrlSet] = useState(false)
  const [editing, setEditing] = useState(false)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [status, setStatus] = useState<{ ok: boolean; msg: string } | null>(null)

  useEffect(() => {
    fetchWebhookSettings()
      .then((s) => {
        setUrlSet(s.url_set)
        if (s.url_set) setUrl(s.url)
      })
      .catch(() => {})
  }, [])

  const handleSave = async () => {
    if (saving) return
    setSaving(true)
    setStatus(null)
    try {
      await saveWebhookSettings(url.trim())
      setUrlSet(url.trim() !== '')
      setEditing(false)
      setStatus({ ok: true, msg: 'Webhook URL saved.' })
    } catch (err) {
      setStatus({ ok: false, msg: err instanceof Error ? err.message : 'Save failed.' })
    } finally {
      setSaving(false)
    }
  }

  const handleTest = async () => {
    if (testing) return
    setTesting(true)
    setStatus(null)
    try {
      await testWebhookSettings()
      setStatus({ ok: true, msg: 'Test payload sent — check your endpoint.' })
    } catch (err) {
      setStatus({ ok: false, msg: err instanceof Error ? err.message : 'Test failed.' })
    } finally {
      setTesting(false)
    }
  }

  const handleClear = async () => {
    setSaving(true)
    setStatus(null)
    try {
      await saveWebhookSettings('')
      setUrl('')
      setUrlSet(false)
      setEditing(false)
      setStatus({ ok: true, msg: 'Webhook cleared.' })
    } catch {
      setStatus({ ok: false, msg: 'Failed to clear.' })
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="max-w-lg space-y-6">

      <div>
        <h2 className="text-base font-semibold text-slate-100 mb-1">Webhook Notifications</h2>
        <p className="text-sm text-slate-400">
          POST a JSON payload to any HTTP endpoint when a tracked device joins, leaves, or changes ports.
          Works natively with Slack, Discord, ntfy.sh, Home Assistant, and any custom endpoint.
        </p>
      </div>

      <div className="bg-slate-800 border border-slate-700 rounded-xl p-5 space-y-4">

        <div className="space-y-1.5">
          <label className="block text-xs font-medium text-slate-400 uppercase tracking-wide">
            Webhook URL
          </label>
          <p className="text-xs text-slate-500">
            Any URL that accepts an HTTP POST with a JSON body.
          </p>
          {urlSet && !editing ? (
            <div className="flex items-center gap-2">
              <span className="flex-1 font-mono text-sm text-slate-400 bg-slate-700/50 border border-slate-600 rounded-lg px-3 py-2 truncate">
                {url}
              </span>
              <button
                onClick={() => setEditing(true)}
                className="text-xs text-cyan-500 hover:text-cyan-400 transition-colors flex-shrink-0"
              >
                Change
              </button>
            </div>
          ) : (
            <input
              type="url"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="https://hooks.slack.com/services/…"
              autoComplete="off"
              className="w-full bg-slate-700 border border-slate-600 rounded-lg px-3 py-2 text-sm text-slate-100 placeholder-slate-500 outline-none focus:ring-1 focus:ring-cyan-500 focus:border-cyan-500 font-mono"
            />
          )}
        </div>

        {status && (
          <p className={`text-sm ${status.ok ? 'text-green-400' : 'text-red-400'}`}>
            {status.msg}
          </p>
        )}

        <div className="flex items-center gap-3 pt-1">
          <button
            onClick={handleSave}
            disabled={saving}
            className="px-5 py-2 rounded-lg bg-cyan-600 hover:bg-cyan-500 disabled:bg-slate-700 disabled:text-slate-500 text-sm font-semibold text-white transition-colors"
          >
            {saving ? 'Saving…' : 'Save'}
          </button>
          <button
            onClick={handleTest}
            disabled={testing || !urlSet}
            title={!urlSet ? 'Save a URL first' : 'Send a test payload'}
            className="px-5 py-2 rounded-lg bg-slate-700 hover:bg-slate-600 disabled:opacity-40 disabled:cursor-not-allowed text-sm font-medium text-slate-200 transition-colors"
          >
            {testing ? 'Sending…' : 'Send test payload'}
          </button>
          {urlSet && (
            <button
              onClick={handleClear}
              disabled={saving}
              className="ml-auto text-xs text-slate-500 hover:text-red-400 transition-colors"
            >
              Clear
            </button>
          )}
        </div>

      </div>

      {/* Payload reference */}
      <div className="space-y-3">
        <h3 className="text-xs font-medium text-slate-400 uppercase tracking-wide">Payload format</h3>
        <pre className="bg-slate-800 border border-slate-700 rounded-xl p-4 text-xs text-slate-300 overflow-x-auto leading-relaxed">{`{
  "event": "network_change",
  "text": "Ding network changes:\\n• NEW 192.168.1.42 …",
  "content": "…",   // same as text (Discord)
  "message": "…",   // same as text (ntfy.sh)
  "changes": [
    { "kind": "NEW", "ip": "192.168.1.42", "desc": "…" }
  ],
  "timestamp": "2026-04-30T12:00:00Z"
}`}</pre>
        <div className="text-xs text-slate-500 space-y-1">
          <p><span className="text-slate-400 font-medium">Slack:</span> paste your Incoming Webhook URL — the <code className="text-slate-300">text</code> field is picked up automatically.</p>
          <p><span className="text-slate-400 font-medium">Discord:</span> append <code className="text-slate-300">/slack</code> to your Discord webhook URL for Slack-compatible mode.</p>
          <p><span className="text-slate-400 font-medium">ntfy.sh:</span> use <code className="text-slate-300">https://ntfy.sh/your-topic</code> — the <code className="text-slate-300">message</code> field is used.</p>
          <p><span className="text-slate-400 font-medium">Home Assistant / n8n:</span> any URL — parse the full JSON payload.</p>
        </div>
      </div>

    </div>
  )
}
