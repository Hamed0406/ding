// Email alert settings panel.
// Mode toggle pre-fills sensible defaults for External vs Self-hosted SMTP.

import { useEffect, useState } from 'react'
import { fetchEmailSettings, saveEmailSettings, testEmailSettings } from '../api/client'

type Mode = 'external' | 'selfhosted'

const PRESETS: Record<string, { host: string; port: number; hint: string }> = {
  'Gmail':              { host: 'smtp.gmail.com',       port: 587, hint: 'Use an App Password — your normal password will not work. Enable 2FA first, then visit myaccount.google.com/apppasswords.' },
  'Outlook / Hotmail':  { host: 'smtp.office365.com',   port: 587, hint: 'Use an App Password from your Microsoft account security settings.' },
  'Yahoo':              { host: 'smtp.mail.yahoo.com',  port: 587, hint: 'Enable "Allow apps that use less secure sign-in" and use an App Password.' },
  'iCloud':             { host: 'smtp.mail.me.com',     port: 587, hint: 'Use an App-Specific Password from appleid.apple.com.' },
  'Fastmail':           { host: 'smtp.fastmail.com',    port: 587, hint: 'Use your Fastmail password or an app password.' },
  'Brevo (Sendinblue)': { host: 'smtp-relay.brevo.com', port: 587, hint: 'Use your Brevo SMTP key as the password.' },
  'Custom':             { host: '',                     port: 587, hint: '' },
}

const SELFHOSTED_DEFAULT = { host: 'localhost', port: 1025 }

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1.5">
      <label className="block text-xs font-medium text-slate-400 uppercase tracking-wide">{label}</label>
      {hint && <p className="text-xs text-slate-500">{hint}</p>}
      {children}
    </div>
  )
}

const inputCls = 'w-full bg-slate-700 border border-slate-600 rounded-lg px-3 py-2 text-sm text-slate-100 placeholder-slate-500 outline-none focus:ring-1 focus:ring-cyan-500 focus:border-cyan-500'

export function EmailSettings() {
  const [mode, setMode] = useState<Mode>('external')
  const [preset, setPreset] = useState('Gmail')

  const [host, setHost]         = useState('')
  const [port, setPort]         = useState(587)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [from, setFrom]         = useState('')
  const [to, setTo]             = useState('')

  const [passwordSet, setPasswordSet]         = useState(false)
  const [editingPassword, setEditingPassword] = useState(false)
  const [savedToServer, setSavedToServer]     = useState(false)

  const [saving, setSaving]   = useState(false)
  const [testing, setTesting] = useState(false)
  const [status, setStatus]   = useState<{ ok: boolean; msg: string } | null>(null)

  // Load saved config on mount
  useEffect(() => {
    fetchEmailSettings()
      .then((cfg) => {
        if (cfg.host) {
          // Detect mode from saved port/host
          const isSelfHosted = cfg.port === 1025 || cfg.host === 'localhost' || cfg.host === '127.0.0.1'
          setMode(isSelfHosted ? 'selfhosted' : 'external')
          setHost(cfg.host)
          setPort(cfg.port)
          setUsername(cfg.username)
          setFrom(cfg.from)
          setTo(cfg.to)
          setPasswordSet(cfg.password_set)
          setSavedToServer(true)
        }
      })
      .catch(() => {})
  }, [])

  // When the user picks a preset, update host + port
  const handlePreset = (name: string) => {
    setPreset(name)
    const p = PRESETS[name]
    if (p) { setHost(p.host); setPort(p.port) }
  }

  // When mode changes, reset to sensible defaults
  const handleModeChange = (m: Mode) => {
    setMode(m)
    setStatus(null)
    if (m === 'selfhosted') {
      setHost(SELFHOSTED_DEFAULT.host)
      setPort(SELFHOSTED_DEFAULT.port)
      setUsername('')
      setPassword('')
      setEditingPassword(false)
    } else {
      handlePreset('Gmail')
    }
  }

  const handleSave = async () => {
    if (saving) return
    setSaving(true)
    setStatus(null)
    try {
      const passwordToSend = (passwordSet && !editingPassword) ? '' : password
      await saveEmailSettings({ host, port, username, password: passwordToSend, from, to })
      setStatus({ ok: true, msg: 'Settings saved.' })
      setSavedToServer(true)
      if (password) { setPasswordSet(true); setEditingPassword(false); setPassword('') }
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
      await testEmailSettings()
      setStatus({ ok: true, msg: `Test email sent to ${to} — check your inbox.` })
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
      await saveEmailSettings({ host: '', port: 587, username: '', password: '', from: '', to: '' })
      setHost(''); setPort(587); setUsername(''); setPassword('')
      setFrom(''); setTo(''); setPasswordSet(false); setEditingPassword(false)
      setSavedToServer(false)
      setStatus({ ok: true, msg: 'Email config cleared.' })
    } catch {
      setStatus({ ok: false, msg: 'Failed to clear.' })
    } finally {
      setSaving(false)
    }
  }

  const presetHint = mode === 'external' ? PRESETS[preset]?.hint : ''
  const canTest = savedToServer && host !== '' && to !== ''

  return (
    <div className="max-w-lg space-y-6">

      <div>
        <h2 className="text-base font-semibold text-slate-100 mb-1">Email Alerts</h2>
        <p className="text-sm text-slate-400">
          Receive an email when a new device joins your network or a tracked device changes.
        </p>
      </div>

      {/* Mode toggle */}
      <div className="flex rounded-lg bg-slate-800 border border-slate-700 p-0.5 w-fit">
        {(['external', 'selfhosted'] as Mode[]).map((m) => (
          <button
            key={m}
            onClick={() => handleModeChange(m)}
            className={[
              'px-4 py-1.5 text-sm font-medium rounded-md transition-colors',
              mode === m ? 'bg-slate-700 text-slate-100' : 'text-slate-400 hover:text-slate-200',
            ].join(' ')}
          >
            {m === 'external' ? 'External (Gmail, Outlook…)' : 'Self-hosted (Mailpit, Postfix…)'}
          </button>
        ))}
      </div>

      <div className="bg-slate-800 border border-slate-700 rounded-xl p-5 space-y-4">

        {/* External: provider preset picker */}
        {mode === 'external' && (
          <Field label="Provider">
            <select
              value={preset}
              onChange={(e) => handlePreset(e.target.value)}
              className={inputCls}
            >
              {Object.keys(PRESETS).map((name) => (
                <option key={name} value={name}>{name}</option>
              ))}
            </select>
            {presetHint && <p className="text-xs text-amber-400 mt-1">{presetHint}</p>}
          </Field>
        )}

        {/* SMTP Host */}
        <Field label="SMTP Host">
          <input
            type="text"
            value={host}
            onChange={(e) => setHost(e.target.value)}
            placeholder={mode === 'selfhosted' ? 'localhost' : 'smtp.gmail.com'}
            className={inputCls}
          />
        </Field>

        {/* SMTP Port */}
        <Field
          label="Port"
          hint={mode === 'external' ? '587 = STARTTLS (recommended) · 465 = Implicit TLS' : '1025 = Mailpit default · 25 = standard SMTP'}
        >
          <input
            type="number"
            value={port}
            onChange={(e) => setPort(Number(e.target.value))}
            className={inputCls}
          />
        </Field>

        {/* Auth — shown for external, hidden for self-hosted unless host is non-local */}
        {(mode === 'external' || (mode === 'selfhosted' && (host !== 'localhost' && host !== '127.0.0.1'))) && (
          <>
            <Field label="Username" hint={mode === 'external' ? 'Usually your email address' : ''}>
              <input
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="you@example.com"
                autoComplete="off"
                className={inputCls}
              />
            </Field>

            <Field label="Password / App Password">
              {passwordSet && !editingPassword ? (
                <div className="flex items-center gap-2">
                  <span className="flex-1 font-mono text-sm text-slate-400 bg-slate-700/50 border border-slate-600 rounded-lg px-3 py-2">
                    ••••••••••••
                  </span>
                  <button
                    onClick={() => { setEditingPassword(true); setPassword('') }}
                    className="text-xs text-cyan-500 hover:text-cyan-400 transition-colors flex-shrink-0"
                  >
                    Change
                  </button>
                </div>
              ) : (
                <input
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="App password or SMTP password"
                  autoComplete="new-password"
                  className={inputCls}
                />
              )}
            </Field>
          </>
        )}

        {/* From */}
        <Field label="From address" hint='Display name is optional, e.g. "Ding <ding@home.local>"'>
          <input
            type="text"
            value={from}
            onChange={(e) => setFrom(e.target.value)}
            placeholder={mode === 'selfhosted' ? 'ding@home.local' : 'you@gmail.com'}
            className={inputCls}
          />
        </Field>

        {/* To */}
        <Field label="Send alerts to">
          <input
            type="email"
            value={to}
            onChange={(e) => setTo(e.target.value)}
            placeholder="you@example.com"
            className={inputCls}
          />
        </Field>

        {/* Status message */}
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
            disabled={testing || !canTest}
            title={!savedToServer ? 'Click Save first' : (!canTest ? 'Fill in host and To address first' : 'Send a test email')}
            className="px-5 py-2 rounded-lg bg-slate-700 hover:bg-slate-600 disabled:opacity-40 disabled:cursor-not-allowed text-sm font-medium text-slate-200 transition-colors"
          >
            {testing ? 'Sending…' : 'Send test email'}
          </button>
          {savedToServer && (
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

      {/* Self-hosted setup hint */}
      {mode === 'selfhosted' && (
        <div className="text-xs text-slate-500 space-y-1">
          <p><span className="text-slate-400 font-medium">Mailpit:</span> add it to your docker-compose.yml alongside Ding, then set host=mailpit, port=1025.</p>
          <p><span className="text-slate-400 font-medium">Postfix:</span> set host=localhost, port=25 — ensure Postfix is installed and running.</p>
          <p><span className="text-slate-400 font-medium">Remote relay:</span> set the remote host/port and provide credentials if required.</p>
        </div>
      )}

      {/* External provider hint */}
      {mode === 'external' && (
        <div className="text-xs text-slate-500 space-y-1">
          <p><span className="text-slate-400 font-medium">1.</span> Pick your provider above and follow the App Password link in the hint.</p>
          <p><span className="text-slate-400 font-medium">2.</span> Enter your email as both username and From address.</p>
          <p><span className="text-slate-400 font-medium">3.</span> Click <span className="text-slate-300">Send test email</span> to confirm delivery.</p>
        </div>
      )}

    </div>
  )
}
