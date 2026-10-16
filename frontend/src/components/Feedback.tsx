import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { errorMessage } from '../api/client'

export function Spinner({ label = 'Loading…' }: { label?: string }) {
  return (
    <div className="flex items-center justify-center gap-3 py-16 text-sm text-stone-500" role="status">
      <span className="h-5 w-5 animate-spin rounded-full border-2 border-stone-300 border-t-brand-600" />
      {label}
    </div>
  )
}

export function ErrorState({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  return (
    <div className="card mx-auto max-w-lg p-6 text-center">
      <p className="font-semibold text-stone-900">Couldn’t load this page</p>
      <p className="mt-1 text-sm text-stone-600">{errorMessage(error)}</p>
      {onRetry && <button className="btn-secondary mt-4" onClick={onRetry}>Try again</button>}
    </div>
  )
}

export function EmptyState({ title, children, action }: { title: string; children?: ReactNode; action?: ReactNode }) {
  return (
    <div className="card px-6 py-14 text-center">
      <p className="text-base font-semibold text-stone-900">{title}</p>
      {children && <div className="mx-auto mt-1 max-w-md text-sm text-stone-600">{children}</div>}
      {action && <div className="mt-5">{action}</div>}
    </div>
  )
}

export function Alert({ kind = 'error', children }: { kind?: 'error' | 'warning' | 'success' | 'info'; children: ReactNode }) {
  const styles = {
    error: 'border-red-200 bg-red-50 text-red-800',
    warning: 'border-amber-200 bg-amber-50 text-amber-900',
    success: 'border-emerald-200 bg-emerald-50 text-emerald-900',
    info: 'border-sky-200 bg-sky-50 text-sky-900',
  }
  return <div role="alert" className={`rounded-lg border px-4 py-3 text-sm ${styles[kind]}`}>{children}</div>
}

export function BackLink({ to, children }: { to: string; children: ReactNode }) {
  return (
    <Link to={to} className="mb-4 inline-flex items-center gap-1 text-sm font-medium text-stone-500 hover:text-stone-900">
      <span aria-hidden>←</span> {children}
    </Link>
  )
}
