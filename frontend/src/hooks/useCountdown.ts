import { useEffect, useState } from 'react'

/**
 * Seconds remaining until `expiresAt`. `serverTime` (from the API response) corrects for a
 * browser clock that is ahead of or behind the server; the server still enforces expiry.
 */
export function useCountdown(expiresAt: string | null | undefined, serverTime?: string | null) {
  const [offsetMs] = useState(() => (serverTime ? new Date(serverTime).getTime() - Date.now() : 0))
  const compute = () => (expiresAt ? Math.max(0, Math.floor((new Date(expiresAt).getTime() - (Date.now() + offsetMs)) / 1000)) : 0)
  const [remaining, setRemaining] = useState(compute)

  useEffect(() => {
    setRemaining(compute())
    if (!expiresAt) return
    const timer = setInterval(() => setRemaining(compute()), 250)
    return () => clearInterval(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [expiresAt, offsetMs])

  return remaining
}

export function formatCountdown(seconds: number): string {
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return `${m}:${s.toString().padStart(2, '0')}`
}
