import { useParams } from 'react-router-dom'
import { adminApi } from '../../api/endpoints'
import { BackLink, EmptyState, ErrorState, Spinner } from '../../components/Feedback'
import { SeatLegend, SeatMap } from '../../components/SeatMap'
import { useApi } from '../../hooks/useApi'
import { bookingCode, formatDateLong, formatDateTime, formatMoney, formatTime } from '../../lib/format'

export function AdminShowDetailPage() {
  const id = Number(useParams().id)
  const { data, error, loading, reload } = useApi(() => adminApi.getShow(id), [id])

  if (loading) return <Spinner />
  if (error) return <ErrorState error={error} onRetry={reload} />
  if (!data) return null
  const { show, bookings, seats } = data

  const stats = [
    { label: 'Seats booked', value: `${show.bookedSeats} / ${show.totalSeats}` },
    { label: 'Occupancy', value: `${show.occupancyPercent}%` },
    { label: 'Confirmed revenue', value: formatMoney(show.revenuePaise) },
    { label: 'Currently held', value: show.heldSeats.toString() },
  ]

  return (
    <div>
      <BackLink to="/admin">Admin dashboard</BackLink>
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="page-title">{show.movieTitle}</h1>
          <p className="mt-1 text-sm text-stone-600">{formatDateLong(show.startTime)} · {formatTime(show.startTime)} – {formatTime(show.endTime)} · {formatMoney(show.pricePaise)} per seat</p>
        </div>
        <button className="btn-secondary" onClick={() => reload()}>Refresh</button>
      </div>

      <div className="mt-6 grid gap-4 sm:grid-cols-4">
        {stats.map((s) => (
          <div key={s.label} className="card p-5">
            <p className="text-xs font-medium uppercase tracking-wide text-stone-500">{s.label}</p>
            <p className="mt-1 text-2xl font-bold">{s.value}</p>
          </div>
        ))}
      </div>

      <div className="mt-6 grid gap-6 xl:grid-cols-[auto_1fr]">
        <section className="card overflow-x-auto p-6">
          <h2 className="mb-6 font-semibold">Seat map</h2>
          <SeatMap seats={seats} readOnly />
          <div className="mt-6 border-t border-stone-100 pt-4"><SeatLegend showSelected={false} showYours={false} /></div>
        </section>
        <section>
          <h2 className="mb-3 font-semibold">Bookings ({bookings.length})</h2>
          {bookings.length === 0 ? <EmptyState title="No bookings yet" /> : (
            <div className="card overflow-x-auto">
              <table className="w-full text-left text-sm">
                <thead className="border-b border-stone-200 bg-stone-50 text-xs uppercase tracking-wide text-stone-500">
                  <tr>
                    <th className="px-4 py-3 font-semibold">Booking ID</th>
                    <th className="px-4 py-3 font-semibold">Customer</th>
                    <th className="px-4 py-3 font-semibold">Seats</th>
                    <th className="px-4 py-3 font-semibold">Booked at</th>
                    <th className="px-4 py-3 text-right font-semibold">Amount</th>
                    <th className="px-4 py-3 font-semibold">Status</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-stone-100">
                  {bookings.map((b) => (
                    <tr key={b.id} className={b.status === 'CANCELLED' ? 'text-stone-400' : ''}>
                      <td className="px-4 py-3 font-mono text-xs">{bookingCode(b.id)}</td>
                      <td className="px-4 py-3">{b.customerName}<span className="block text-xs text-stone-500">{b.customerEmail}</span></td>
                      <td className="px-4 py-3">{b.seats.join(', ')}</td>
                      <td className="px-4 py-3 whitespace-nowrap">{formatDateTime(b.createdAt)}</td>
                      <td className="px-4 py-3 text-right">{formatMoney(b.totalPaise)}</td>
                      <td className="px-4 py-3 text-xs font-semibold">{b.status}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
      </div>
    </div>
  )
}
