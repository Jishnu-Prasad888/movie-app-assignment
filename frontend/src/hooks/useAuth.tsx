import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { authApi, configApi, type DemoAccountKey } from '../api/endpoints'
import type { AppConfig, User } from '../types'
import { setCinemaTimeZone } from '../lib/format'

interface AuthState {
  user: User | null
  config: AppConfig | null
  loading: boolean
  login: (email: string, password: string) => Promise<User>
  signup: (name: string, email: string, password: string) => Promise<User>
  demoLogin: (account: DemoAccountKey) => Promise<User>
  logout: () => Promise<void>
  /** Called when the API says the session is gone (401). */
  clearUser: () => void
}

const AuthContext = createContext<AuthState | null>(null)

/**
 * Holds the current user for the UI. The user object is only used for display and routing;
 * the real authorization decision is made by the API on every request using the HttpOnly
 * session cookie, so tampering with this state grants nothing.
 */
export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [config, setConfig] = useState<AppConfig | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    Promise.allSettled([authApi.me(), configApi.get()]).then(([me, cfg]) => {
      if (me.status === 'fulfilled') setUser(me.value.user)
      if (cfg.status === 'fulfilled') {
        setConfig(cfg.value)
        setCinemaTimeZone(cfg.value.timezone)
      }
      setLoading(false)
    })
  }, [])

  const login = useCallback(async (email: string, password: string) => {
    const { user } = await authApi.login(email, password)
    setUser(user)
    return user
  }, [])
  const signup = useCallback(async (name: string, email: string, password: string) => {
    const { user } = await authApi.signup(name, email, password)
    setUser(user)
    return user
  }, [])
  const demoLogin = useCallback(async (account: DemoAccountKey) => {
    const { user } = await authApi.demoLogin(account)
    setUser(user)
    return user
  }, [])
  const logout = useCallback(async () => {
    try {
      await authApi.logout()
    } finally {
      setUser(null)
    }
  }, [])
  const clearUser = useCallback(() => setUser(null), [])

  const value = useMemo(
    () => ({ user, config, loading, login, signup, demoLogin, logout, clearUser }),
    [user, config, loading, login, signup, demoLogin, logout, clearUser],
  )
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used inside AuthProvider')
  return ctx
}
