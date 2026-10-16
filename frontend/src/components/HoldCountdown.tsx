import { formatCountdown } from '../hooks/useCountdown'

export function HoldCountdown({ seconds }: { seconds: number }) {
  const urgent = seconds <= 60
  return (
    <div className={`flex items-center justify-between rounded-lg border px-4 py-3 ${urgent ? 'border-red-200 bg-red-50' : 'border-amber-200 bg-amber-50'}`}>
      <div>
        <p className={`text-sm font-semibold ${urgent ? 'text-red-800' : 'text-amber-900'}`}>Seats held for you</p>
        <p className={`text-xs ${urgent ? 'text-red-700' : 'text-amber-800'}`}>Complete payment before the timer runs out.</p>
      </div>
      <span className={`font-mono text-2xl font-bold tabular-nums ${urgent ? 'text-red-700' : 'text-amber-900'}`} aria-live="off">
        {formatCountdown(seconds)}
      </span>
    </div>
  )
}
