// Auth page — handles both sign-in and registration in one view.
// Automatically shows "Create your account" on first run (when no users exist yet).
// Social login buttons appear only when the server has those providers configured.

import { useEffect, useState } from 'react'
import { fetchAuthProviders, login, register } from '../api/client'

interface Props {
  onLogin: () => void
  initialError?: string
}

interface Providers {
  google: boolean
  github: boolean
  has_users: boolean
}

export function LoginPage({ onLogin, initialError }: Props) {
  const [providers, setProviders] = useState<Providers | null>(null)
  // 'login' | 'register' — auto-set to 'register' on first run
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [loading, setLoading] = useState(false)

  // Pick up ?auth_error= set by the OAuth callback on failure, or initialError from exchange
  const oauthError = new URLSearchParams(window.location.search).get('auth_error')
  const [error, setError] = useState(initialError ?? oauthError ?? '')

  useEffect(() => {
    fetchAuthProviders()
      .then((p) => {
        setProviders(p)
        if (!p.has_users) setMode('register') // first-run: go straight to registration
      })
      .catch(() => setProviders({ google: false, github: false, has_users: false }))
  }, [])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (loading) return
    setError('')

    if (mode === 'register') {
      if (password !== confirm) { setError('Passwords do not match'); return }
      if (password.length < 8) { setError('Password must be at least 8 characters'); return }
    }

    setLoading(true)
    try {
      if (mode === 'register') {
        await register(email.trim().toLowerCase(), password)
      } else {
        await login(email.trim().toLowerCase(), password)
      }
      onLogin()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong')
    } finally {
      setLoading(false)
    }
  }

  const isFirstRun = providers && !providers.has_users
  const hasOAuth = providers && (providers.google || providers.github)

  return (
    <div className="min-h-screen bg-slate-900 flex items-center justify-center px-4">
      <div className="w-full max-w-sm">

        {/* Logo */}
        <div className="flex items-center gap-2.5 justify-center mb-8">
          <span className="w-3 h-3 rounded-full bg-cyan-500 animate-pulse" />
          <h1 className="text-2xl font-bold tracking-tight text-slate-100">Ding</h1>
        </div>

        <div className="bg-slate-800 border border-slate-700 rounded-2xl p-6 space-y-5">

          {/* Tab toggle — hidden on first run (only register makes sense) */}
          {providers && providers.has_users && (
            <div className="flex rounded-lg bg-slate-700/50 p-0.5">
              {(['login', 'register'] as const).map((m) => (
                <button
                  key={m}
                  type="button"
                  onClick={() => { setMode(m); setError('') }}
                  className={[
                    'flex-1 py-1.5 text-xs font-medium rounded-md capitalize transition-colors',
                    mode === m ? 'bg-slate-600 text-slate-100' : 'text-slate-400 hover:text-slate-200',
                  ].join(' ')}
                >
                  {m === 'login' ? 'Sign in' : 'Create account'}
                </button>
              ))}
            </div>
          )}

          {isFirstRun && (
            <p className="text-xs text-slate-400 text-center">
              Welcome! Create your admin account to get started.
            </p>
          )}

          {/* OAuth buttons */}
          {hasOAuth && (
            <div className="space-y-2">
              {providers.google && (
                <a
                  href="/api/auth/google"
                  className="flex items-center justify-center gap-2.5 w-full py-2 rounded-lg border border-slate-600 bg-slate-700/50 hover:bg-slate-700 text-sm text-slate-200 transition-colors"
                >
                  <GoogleIcon />
                  Continue with Google
                </a>
              )}
              {providers.github && (
                <a
                  href="/api/auth/github"
                  className="flex items-center justify-center gap-2.5 w-full py-2 rounded-lg border border-slate-600 bg-slate-700/50 hover:bg-slate-700 text-sm text-slate-200 transition-colors"
                >
                  <GitHubIcon />
                  Continue with GitHub
                </a>
              )}
              <div className="flex items-center gap-3">
                <div className="flex-1 h-px bg-slate-700" />
                <span className="text-xs text-slate-500">or</span>
                <div className="flex-1 h-px bg-slate-700" />
              </div>
            </div>
          )}

          {/* Email + password form */}
          <form onSubmit={handleSubmit} className="space-y-3">
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="Email address"
              autoComplete="email"
              autoFocus
              required
              className="w-full bg-slate-700 border border-slate-600 rounded-lg px-3 py-2 text-sm text-slate-100 placeholder-slate-500 outline-none focus:ring-1 focus:ring-cyan-500 focus:border-cyan-500"
            />
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Password"
              autoComplete={mode === 'register' ? 'new-password' : 'current-password'}
              required
              className="w-full bg-slate-700 border border-slate-600 rounded-lg px-3 py-2 text-sm text-slate-100 placeholder-slate-500 outline-none focus:ring-1 focus:ring-cyan-500 focus:border-cyan-500"
            />
            {mode === 'register' && (
              <input
                type="password"
                value={confirm}
                onChange={(e) => setConfirm(e.target.value)}
                placeholder="Confirm password"
                autoComplete="new-password"
                required
                className="w-full bg-slate-700 border border-slate-600 rounded-lg px-3 py-2 text-sm text-slate-100 placeholder-slate-500 outline-none focus:ring-1 focus:ring-cyan-500 focus:border-cyan-500"
              />
            )}

            {error && <p className="text-xs text-red-400">{error}</p>}

            <button
              type="submit"
              disabled={loading || !email || !password}
              className="w-full py-2 rounded-lg bg-cyan-600 hover:bg-cyan-500 disabled:bg-slate-700 disabled:text-slate-500 text-sm font-semibold text-white transition-colors"
            >
              {loading
                ? (mode === 'register' ? 'Creating account…' : 'Signing in…')
                : (mode === 'register' ? 'Create account' : 'Sign in')}
            </button>
          </form>

        </div>
      </div>
    </div>
  )
}

function GoogleIcon() {
  return (
    <svg viewBox="0 0 24 24" className="w-4 h-4" aria-hidden="true">
      <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"/>
      <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/>
      <path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/>
      <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/>
    </svg>
  )
}

function GitHubIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" className="w-4 h-4 text-slate-300" aria-hidden="true">
      <path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0 1 12 6.844a9.59 9.59 0 0 1 2.504.337c1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.02 10.02 0 0 0 22 12.017C22 6.484 17.522 2 12 2z"/>
    </svg>
  )
}
