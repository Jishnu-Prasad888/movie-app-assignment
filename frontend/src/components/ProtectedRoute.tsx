import type { ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router-dom'
import { useAuth } from '../hooks/useAuth'
import { Spinner } from './Feedback'

/**
 * Client-side route guard. This is a UX convenience only (redirects to login, hides admin
 * pages); every protected API call is independently authorized by the backend.
 */
export function ProtectedRoute({ children, adminOnly = false }: { children: ReactNode; adminOnly?: boolean }) {
  const { user, loading } = useAuth()
  const location = useLocation()
  if (loading) return <Spinner />
  if (!user) {
    const next = encodeURIComponent(location.pathname + location.search)
    return <Navigate to={`/login?next=${next}`} replace />
  }
  if (adminOnly && user.role !== 'admin') {
    return (
      <div className="card mx-auto max-w-md p-8 text-center">
        <p className="text-lg font-semibold">Admins only</p>
        <p className="mt-1 text-sm text-stone-600">Your account doesn’t have access to the admin dashboard.</p>
      </div>
    )
  }
  return <>{children}</>
}
