import { Link, NavLink, Outlet, useNavigate } from 'react-router-dom'
import { useAuth } from '../hooks/useAuth'
import { useToast } from '../hooks/useToast'

const navClass = ({ isActive }: { isActive: boolean }) =>
  `rounded-md px-3 py-2 text-sm font-medium transition ${isActive ? 'bg-stone-100 text-stone-900' : 'text-stone-600 hover:text-stone-900'}`

export function MainLayout() {
  const { user, logout } = useAuth()
  const toast = useToast()
  const navigate = useNavigate()

  async function handleLogout() {
    await logout()
    toast.info('You have been logged out.')
    navigate('/')
  }

  return (
    <div className="flex min-h-screen flex-col">
      <header className="sticky top-0 z-40 border-b border-stone-200 bg-white/95 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-6xl items-center gap-6 px-6">
          <Link to="/" className="flex items-center gap-2">
            <img src="/favicon.svg" alt="" className="h-8 w-8" />
            <span className="text-lg font-bold tracking-tight">Yozigo</span>
          </Link>
          <nav className="flex items-center gap-1">
            <NavLink to="/" end className={navClass}>Movies</NavLink>
            {user && <NavLink to="/bookings" className={navClass}>My Bookings</NavLink>}
            {user?.role === 'admin' && <NavLink to="/admin" className={navClass}>Admin</NavLink>}
          </nav>
          <div className="ml-auto flex items-center gap-3">
            {user ? (
              <>
                <div className="text-right leading-tight">
                  <p className="text-sm font-semibold text-stone-900">{user.name}</p>
                  <p className="text-xs text-stone-500">
                    <span className={`inline-block rounded px-1.5 py-px font-semibold uppercase tracking-wide ${user.role === 'admin' ? 'bg-brand-100 text-brand-800' : 'bg-stone-100 text-stone-600'}`}>
                      {user.role}
                    </span>
                  </p>
                </div>
                <button onClick={handleLogout} className="btn-secondary">Logout</button>
              </>
            ) : (
              <>
                <Link to="/login" className="btn-ghost">Log in</Link>
                <Link to="/signup" className="btn-primary">Sign up</Link>
              </>
            )}
          </div>
        </div>
      </header>
      <main className="mx-auto w-full max-w-6xl flex-1 px-6 py-8">
        <Outlet />
      </main>
      <footer className="border-t border-stone-200 py-6 text-center text-xs text-stone-500">
        Yozigo · Single-screen cinema · Payments are simulated
      </footer>
    </div>
  )
}
