import { Link, useParams, useSearchParams } from 'react-router-dom'
import { bookingsApi } from '../api/endpoints'
import { ErrorState, Spinner } from '../components/Feedback'
import { useApi } from '../hooks/useApi'
import { bookingCode, formatDateLong, formatDateTime, formatMoney, formatTime } from '../lib/format'

export function BookingConfirmationPage() {
  const id = Number(useParams().id)
  const [params] = useSearchParams()
  const isNew = params.get('new') === '1'
  const { data, error, loading, reload } = useApi(() => bookingsApi.get(id), [id])

  if (loading) return <Spinner />
  if (error) return <ErrorState error={error} onRetry={reload} />
  if (!data) return null
  const b = data.booking
  const confirmed = b.status === 'CONFIRMED'

  return (
    <div className="mx-auto max-w-lg">
      {isNew && confirmed && (
        <div className="mb-6 text-center">
          <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-emerald-100 text-2xl text-emerald-700">✓</div>
          <h1 className="page-title">Booking confirmed</h1>
          <p className="mt-1 text-sm text-stone-600">Enjoy the show! Your tickets are below.</p>
        </div>
      )}
      <div className="card overflow-hidden">
        <div className="flex items-center justify-between border-b border-dashed border-stone-300 bg-stone-900 px-6 py-4 text-white">
          <div>
            <p className="text-xs uppercase tracking-wider text-stone-400">Booking ID</p>
            <p className="font-mono text-lg font-bold">{bookingCode(b.id)}</p>
          </div>
          <span className={`rounded-full px-3 py-1 text-xs font-bold uppercase ${confirmed ? 'bg-emerald-500 text-white' : 'bg-stone-600 text-stone-200'}`}>{b.status}</span>
        </div>
        <dl className="grid grid-cols-2 gap-5 p-6 text-sm">
          <div className="col-span-2"><dt className="text-stone-500">Movie</dt><dd className="text-lg font-semibold">{b.movieTitle}</dd></div>
          <div><dt className="text-stone-500">Date</dt><dd className="font-medium">{formatDateLong(b.startTime)}</dd></div>
          <div><dt className="text-stone-500">Time</dt><dd className="font-medium">{formatTime(b.startTime)}</dd></div>
          <div><dt className="text-stone-500">Seats</dt><dd className="font-medium">{b.seats.join(', ')}</dd></div>
          <div><dt className="text-stone-500">Amount</dt><dd className="font-medium">{formatMoney(b.totalPaise)}</dd></div>
          <div className="col-span-2 text-xs text-stone-500">Booked {formatDateTime(b.createdAt)}{b.cancelledAt && ` · Cancelled ${formatDateTime(b.cancelledAt)}`}</div>
        </dl>
      </div>
      <div className="mt-6 flex justify-center gap-3">
        <Link to="/bookings" className="btn-secondary">My bookings</Link>
        <Link to="/" className="btn-primary">Browse movies</Link>
      </div>
    </div>
  )
}
