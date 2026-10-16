import { useState } from 'react'
import { Link } from 'react-router-dom'
import { errorMessage } from '../api/client'
import { bookingsApi } from '../api/endpoints'
import { EmptyState, ErrorState, Spinner } from '../components/Feedback'
import { useApi } from '../hooks/useApi'
import { useToast } from '../hooks/useToast'
import { bookingCode, formatDate, formatMoney, formatTime } from '../lib/format'
import type { Booking } from '../types'

export function MyBookingsPage() {
  const { data, error, loading, reload } = useApi(() => bookingsApi.list(), [])
  const toast = useToast()
  const [cancelling, setCancelling] = useState<number | null>(null)

  if (loading) return <Spinner label="Loading your bookings…" />
  if (error) return <ErrorState error={error} onRetry={reload} />
  const bookings = data?.bookings ?? []
  // The API already sorts upcoming first, then past.
  const now = Date.now()
  const upcoming = bookings.filter((b) => new Date(b.startTime).getTime() > now)
  const past = bookings.filter((b) => new Date(b.startTime).getTime() <= now)

  async function cancel(b: Booking) {
    if (!window.confirm(`Cancel booking ${bookingCode(b.id)} for ${b.movieTitle}? Seats ${b.seats.join(', ')} will be released.`)) return
    setCancelling(b.id)
    try {
      await bookingsApi.cancel(b.id)
      toast.success(`Booking ${bookingCode(b.id)} cancelled.`)
      reload({ silent: true })
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setCancelling(null)
    }
  }

  return (
    <div>
      <h1 className="page-title mb-6">My bookings</h1>
      {bookings.length === 0 ? (
        <EmptyState title="No bookings yet" action={<Link to="/" className="btn-primary">Browse movies</Link>}>
          When you book tickets, they’ll appear here.
        </EmptyState>
      ) : (
        <div className="space-y-8">
          <Section title="Upcoming" bookings={upcoming} empty="No upcoming bookings." onCancel={cancel} cancelling={cancelling} />
          {past.length > 0 && <Section title="Past" bookings={past} onCancel={cancel} cancelling={cancelling} />}
        </div>
      )}
    </div>
  )
}

function Section({ title, bookings, empty, onCancel, cancelling }: {
  title: string; bookings: Booking[]; empty?: string; onCancel: (b: Booking) => void; cancelling: number | null
}) {
  return (
    <section>
      <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-stone-500">{title}</h2>
      {bookings.length === 0 ? <p className="text-sm text-stone-500">{empty}</p> : (
        <div className="card overflow-hidden">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-stone-200 bg-stone-50 text-xs uppercase tracking-wide text-stone-500">
              <tr>
                <th className="px-4 py-3 font-semibold">Booking ID</th>
                <th className="px-4 py-3 font-semibold">Movie</th>
                <th className="px-4 py-3 font-semibold">Show</th>
                <th className="px-4 py-3 font-semibold">Seats</th>
                <th className="px-4 py-3 text-right font-semibold">Amount</th>
                <th className="px-4 py-3 font-semibold">Status</th>
                <th className="px-4 py-3" />
              </tr>
            </thead>
            <tbody className="divide-y divide-stone-100">
              {bookings.map((b) => (
                <tr key={b.id} className={b.status === 'CANCELLED' ? 'text-stone-400' : ''}>
                  <td className="px-4 py-3 font-mono text-xs"><Link to={`/bookings/${b.id}`} className="hover:underline">{bookingCode(b.id)}</Link></td>
                  <td className="px-4 py-3 font-medium">{b.movieTitle}</td>
                  <td className="px-4 py-3">{formatDate(b.startTime)}<span className="block text-xs text-stone-500">{formatTime(b.startTime)}</span></td>
                  <td className="px-4 py-3">{b.seats.join(', ')}</td>
                  <td className="px-4 py-3 text-right">{formatMoney(b.totalPaise)}</td>
                  <td className="px-4 py-3">
                    <span className={`rounded-full px-2 py-0.5 text-xs font-semibold ${b.status === 'CONFIRMED' ? 'bg-emerald-50 text-emerald-700' : 'bg-stone-100 text-stone-500'}`}>{b.status}</span>
                  </td>
                  <td className="px-4 py-3 text-right">
                    {b.cancellable && (
                      <button className="text-xs font-semibold text-red-700 hover:underline disabled:opacity-50" disabled={cancelling === b.id} onClick={() => onCancel(b)}>
                        {cancelling === b.id ? 'Cancelling…' : 'Cancel'}
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
