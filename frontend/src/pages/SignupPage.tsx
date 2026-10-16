import { useState, type FormEvent } from 'react'
import { Link, Navigate, useNavigate, useSearchParams } from 'react-router-dom'
import { ApiError, errorMessage } from '../api/client'
import { Alert } from '../components/Feedback'
import { useAuth } from '../hooks/useAuth'
import { useToast } from '../hooks/useToast'

export function SignupPage() {
  const { user, signup } = useAuth()
  const toast = useToast()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const nextParam = params.get('next')
  const next = nextParam && nextParam.startsWith('/') && !nextParam.startsWith('//') ? nextParam : '/'

  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [formError, setFormError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  // While a login request is in flight (busy), the handler does the redirect itself.
  if (user && !busy) return <Navigate to={next} replace />

  function validate() {
    const e: Record<string, string> = {}
    if (!name.trim()) e.name = 'Name is required.'
    if (!/^\S+@\S+\.\S+$/.test(email.trim())) e.email = 'Enter a valid email address.'
    if (password.length < 8) e.password = 'Password must be at least 8 characters.'
    return e
  }

  async function onSubmit(ev: FormEvent) {
    ev.preventDefault()
    setFormError(null)
    const e = validate()
    setErrors(e)
    if (Object.keys(e).length) return
    setBusy(true)
    try {
      const u = await signup(name.trim(), email.trim(), password)
      toast.success(`Welcome to Yozigo, ${u.name}!`)
      navigate(next, { replace: true })
    } catch (err) {
      setBusy(false)
      if (err instanceof ApiError && err.code === 'EMAIL_TAKEN') setErrors({ email: err.message })
      else if (err instanceof ApiError && err.code === 'WEAK_PASSWORD') setErrors({ password: err.message })
      else setFormError(errorMessage(err))
    }
  }

  return (
    <section className="card mx-auto max-w-md p-8">
      <h1 className="page-title">Create your account</h1>
      <p className="mt-1 text-sm text-stone-600">Book seats in under a minute.</p>
      <form onSubmit={onSubmit} className="mt-6 space-y-4" noValidate>
        {formError && <Alert>{formError}</Alert>}
        <div>
          <label htmlFor="name" className="label">Name</label>
          <input id="name" className="input" autoComplete="name" value={name} onChange={(e) => setName(e.target.value)} />
          {errors.name && <p className="field-error">{errors.name}</p>}
        </div>
        <div>
          <label htmlFor="email" className="label">Email</label>
          <input id="email" type="email" className="input" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} />
          {errors.email && <p className="field-error">{errors.email}</p>}
        </div>
        <div>
          <label htmlFor="password" className="label">Password</label>
          <input id="password" type="password" className="input" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} />
          {errors.password ? <p className="field-error">{errors.password}</p> : <p className="mt-1 text-xs text-stone-500">At least 8 characters.</p>}
        </div>
        <button type="submit" className="btn-primary w-full" disabled={busy}>{busy ? 'Creating account…' : 'Sign up'}</button>
      </form>
      <p className="mt-6 text-center text-sm text-stone-600">
        Already have an account? <Link to="/login" className="font-semibold text-brand-700 hover:underline">Log in</Link>
      </p>
    </section>
  )
}
