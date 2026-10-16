import { useState, type FormEvent } from 'react'
import { Link, Navigate, useNavigate, useSearchParams } from 'react-router-dom'
import { errorMessage } from '../api/client'
import type { DemoAccountKey } from '../api/endpoints'
import { Alert } from '../components/Feedback'
import { useAuth } from '../hooks/useAuth'
import { useToast } from '../hooks/useToast'

// Only the account key is sent to the server. There are no demo passwords in the frontend.
const demoAccounts: { key: DemoAccountKey; label: string; hint: string }[] = [
  { key: 'admin', label: 'Login as Admin', hint: 'Manage movies, shows and view bookings' },
  { key: 'customer1', label: 'Login as Customer 1', hint: 'Ravi Kumar' },
  { key: 'customer2', label: 'Login as Customer 2', hint: 'Meera Iyer' },
  { key: 'customer3', label: 'Login as Customer 3', hint: 'Arjun Singh' },
]

/** Only allow redirects to paths inside this app (prevents open redirects via ?next=). */
function safeNext(next: string | null): string {
  return next && next.startsWith('/') && !next.startsWith('//') ? next : '/'
}

export function LoginPage() {
  const { user, login, demoLogin, config } = useAuth()
  const toast = useToast()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const next = safeNext(params.get('next'))

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState<string | null>(null)

  // While a login request is in flight (busy), the handler does the redirect itself.
  if (user && !busy) return <Navigate to={next} replace />

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    if (!email.trim() || !password) {
      setError('Enter your email and password.')
      return
    }
    setBusy('form')
    try {
      const u = await login(email, password)
      toast.success(`Welcome back, ${u.name}!`)
      navigate(next, { replace: true })
    } catch (err) {
      setBusy(null)
      setError(errorMessage(err))
    }
  }

  async function onDemo(key: DemoAccountKey) {
    setError(null)
    setBusy(key)
    try {
      const u = await demoLogin(key)
      toast.success(`Logged in as ${u.name} (${u.role}).`)
      navigate(u.role === 'admin' && next === '/' ? '/admin' : next, { replace: true })
    } catch (err) {
      setBusy(null)
      setError(errorMessage(err))
    }
  }

  return (
    <div className="mx-auto grid max-w-4xl gap-8 md:grid-cols-2">
      <section className="card p-8">
        <h1 className="page-title">Log in</h1>
        <p className="mt-1 text-sm text-stone-600">Use your email and password.</p>
        <form onSubmit={onSubmit} className="mt-6 space-y-4" noValidate>
          {error && <Alert>{error}</Alert>}
          <div>
            <label htmlFor="email" className="label">Email</label>
            <input id="email" type="email" autoComplete="email" className="input" value={email} onChange={(e) => setEmail(e.target.value)} />
          </div>
          <div>
            <label htmlFor="password" className="label">Password</label>
            <input id="password" type="password" autoComplete="current-password" className="input" value={password} onChange={(e) => setPassword(e.target.value)} />
          </div>
          <button type="submit" className="btn-primary w-full" disabled={busy !== null}>
            {busy === 'form' ? 'Logging in…' : 'Log in'}
          </button>
        </form>
        <p className="mt-6 text-center text-sm text-stone-600">
          New to Yozigo? <Link to={`/signup${next !== '/' ? `?next=${encodeURIComponent(next)}` : ''}`} className="font-semibold text-brand-700 hover:underline">Create an account</Link>
        </p>
      </section>

      {config?.demoLoginEnabled && (
        <section className="rounded-xl border-2 border-dashed border-amber-300 bg-amber-50/60 p-8">
          <div className="flex items-center gap-2">
            <h2 className="text-lg font-bold text-stone-900">Demo Accounts</h2>
            <span className="rounded bg-amber-200 px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wider text-amber-900">Demo only</span>
          </div>
          <p className="mt-1 text-sm text-stone-700">One-click login to seeded accounts. Enabled for this demo environment only.</p>
          <div className="mt-5 space-y-3">
            {demoAccounts.map((a) => (
              <button
                key={a.key}
                type="button"
                onClick={() => onDemo(a.key)}
                disabled={busy !== null}
                className="flex w-full items-center justify-between rounded-lg border border-amber-300 bg-white px-4 py-3 text-left transition hover:border-amber-500 hover:shadow-sm disabled:opacity-50"
              >
                <span>
                  <span className="block text-sm font-semibold text-stone-900">{a.label}</span>
                  <span className="block text-xs text-stone-500">{a.hint}</span>
                </span>
                <span className="text-sm text-amber-700">{busy === a.key ? '…' : '→'}</span>
              </button>
            ))}
          </div>
        </section>
      )}
    </div>
  )
}
