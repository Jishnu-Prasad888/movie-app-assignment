import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { errorMessage } from '../../api/client'
import { adminApi, moviesApi } from '../../api/endpoints'
import { EmptyState, ErrorState, Spinner } from '../../components/Feedback'
import { RatingBadge } from '../../components/Poster'
import { useApi } from '../../hooks/useApi'
import { useToast } from '../../hooks/useToast'
import { formatDate, formatDuration, formatMoney, formatTime } from '../../lib/format'
import type { AdminShow, Movie } from '../../types'
import { MovieForm } from './MovieForm'
import { ShowForm } from './ShowForm'

type Tab = 'shows' | 'movies'

export function AdminDashboardPage() {
  const [params, setParams] = useSearchParams()
  const tab: Tab = params.get('tab') === 'movies' ? 'movies' : 'shows'
  const shows = useApi(() => adminApi.listShows(), [])
  const movies = useApi(() => moviesApi.list(), [])

  // Keep occupancy/revenue fresh while the dashboard is open.
  useEffect(() => {
    const t = setInterval(() => {
      if (document.visibilityState === 'visible') shows.reload({ silent: true })
    }, 10000)
    return () => clearInterval(t)
  }, [shows.reload]) // eslint-disable-line react-hooks/exhaustive-deps

  const tabClass = (t: Tab) =>
    `border-b-2 px-1 pb-3 text-sm font-semibold transition ${tab === t ? 'border-brand-600 text-stone-900' : 'border-transparent text-stone-500 hover:text-stone-800'}`

  return (
    <div>
      <h1 className="page-title">Admin dashboard</h1>
      <Summary shows={shows.data?.shows ?? []} />
      <div className="mt-8 mb-6 flex gap-6 border-b border-stone-200">
        <button className={tabClass('shows')} onClick={() => setParams({})}>Shows</button>
        <button className={tabClass('movies')} onClick={() => setParams({ tab: 'movies' })}>Movies</button>
      </div>
      {tab === 'shows' ? (
        <ShowsTab
          shows={shows.data?.shows ?? null} loading={shows.loading} error={shows.error} reload={() => shows.reload({ silent: true })}
          movies={movies.data?.movies ?? []}
        />
      ) : (
        <MoviesTab
          movies={movies.data?.movies ?? null} loading={movies.loading} error={movies.error}
          reload={() => { movies.reload({ silent: true }); shows.reload({ silent: true }) }}
        />
      )}
    </div>
  )
}

function Summary({ shows }: { shows: AdminShow[] }) {
  const now = Date.now()
  const upcoming = shows.filter((s) => new Date(s.startTime).getTime() > now)
  const revenue = shows.reduce((a, s) => a + s.revenuePaise, 0)
  const booked = shows.reduce((a, s) => a + s.bookedSeats, 0)
  const cards = [
    { label: 'Upcoming shows', value: upcoming.length.toString() },
    { label: 'Seats booked (all shows)', value: booked.toString() },
    { label: 'Confirmed revenue', value: formatMoney(revenue) },
  ]
  return (
    <div className="mt-6 grid gap-4 sm:grid-cols-3">
      {cards.map((c) => (
        <div key={c.label} className="card p-5">
          <p className="text-xs font-medium uppercase tracking-wide text-stone-500">{c.label}</p>
          <p className="mt-1 text-2xl font-bold">{c.value}</p>
        </div>
      ))}
    </div>
  )
}

function OccupancyBar({ percent }: { percent: number }) {
  return (
    <div className="flex items-center gap-2">
      <div className="h-1.5 w-20 overflow-hidden rounded-full bg-stone-100">
        <div className="h-full rounded-full bg-brand-600" style={{ width: `${Math.min(100, percent)}%` }} />
      </div>
      <span className="text-xs tabular-nums text-stone-600">{percent}%</span>
    </div>
  )
}

function ShowsTab({ shows, loading, error, reload, movies }: {
  shows: AdminShow[] | null; loading: boolean; error: unknown; reload: () => void; movies: Movie[]
}) {
  const toast = useToast()
  const [showPast, setShowPast] = useState(false)
  const [deleting, setDeleting] = useState<number | null>(null)

  if (loading && !shows) return <Spinner />
  if (error && !shows) return <ErrorState error={error} onRetry={reload} />
  const now = Date.now()
  const list = (shows ?? []).filter((s) => showPast || new Date(s.startTime).getTime() > now)

  async function remove(s: AdminShow) {
    if (!window.confirm(`Delete the ${formatTime(s.startTime)} show of ${s.movieTitle} on ${formatDate(s.startTime)}?`)) return
    setDeleting(s.id)
    try {
      await adminApi.deleteShow(s.id)
      toast.success('Show deleted.')
      reload()
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setDeleting(null)
    }
  }

  return (
    <div className="grid gap-6 lg:grid-cols-[1fr_320px]">
      <div>
        <div className="mb-3 flex items-center justify-between">
          <h2 className="font-semibold">{showPast ? 'All shows' : 'Upcoming shows'}</h2>
          <label className="flex items-center gap-2 text-sm text-stone-600">
            <input type="checkbox" checked={showPast} onChange={(e) => setShowPast(e.target.checked)} className="accent-brand-600" />
            Include past shows
          </label>
        </div>
        {list.length === 0 ? (
          <EmptyState title="No shows">Use the form to schedule a show.</EmptyState>
        ) : (
          <div className="card overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="border-b border-stone-200 bg-stone-50 text-xs uppercase tracking-wide text-stone-500">
                <tr>
                  <th className="px-4 py-3 font-semibold">Show</th>
                  <th className="px-4 py-3 font-semibold">Movie</th>
                  <th className="px-4 py-3 font-semibold">Booked</th>
                  <th className="px-4 py-3 font-semibold">Occupancy</th>
                  <th className="px-4 py-3 text-right font-semibold">Revenue</th>
                  <th className="px-4 py-3" />
                </tr>
              </thead>
              <tbody className="divide-y divide-stone-100">
                {list.map((s) => {
                  const past = new Date(s.startTime).getTime() <= now
                  return (
                    <tr key={s.id} className={past ? 'text-stone-400' : ''}>
                      <td className="px-4 py-3 whitespace-nowrap">{formatDate(s.startTime)}<span className="block text-xs text-stone-500">{formatTime(s.startTime)} – {formatTime(s.endTime)} · {formatMoney(s.pricePaise)}</span></td>
                      <td className="px-4 py-3 font-medium">{s.movieTitle}</td>
                      <td className="px-4 py-3 tabular-nums">{s.bookedSeats}/{s.totalSeats}</td>
                      <td className="px-4 py-3"><OccupancyBar percent={s.occupancyPercent} /></td>
                      <td className="px-4 py-3 text-right tabular-nums">{formatMoney(s.revenuePaise)}</td>
                      <td className="px-4 py-3 text-right whitespace-nowrap">
                        <Link to={`/admin/shows/${s.id}`} className="text-xs font-semibold text-brand-700 hover:underline">View</Link>
                        <button
                          className="ml-3 text-xs font-semibold text-red-700 hover:underline disabled:cursor-not-allowed disabled:text-stone-300 disabled:no-underline"
                          disabled={deleting === s.id || s.bookingCount > 0}
                          title={s.bookingCount > 0 ? 'Shows with bookings cannot be deleted' : 'Delete show'}
                          onClick={() => remove(s)}
                        >
                          Delete
                        </button>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
      <ShowForm movies={movies} onCreated={reload} />
    </div>
  )
}

function MoviesTab({ movies, loading, error, reload }: {
  movies: Movie[] | null; loading: boolean; error: unknown; reload: () => void
}) {
  const toast = useToast()
  const [editing, setEditing] = useState<Movie | null>(null)
  const [formKey, setFormKey] = useState(0)

  if (loading && !movies) return <Spinner />
  if (error && !movies) return <ErrorState error={error} onRetry={reload} />

  async function remove(m: Movie) {
    if (!window.confirm(`Delete "${m.title}" and all of its shows? This only works if none of its shows have bookings.`)) return
    try {
      await adminApi.deleteMovie(m.id)
      toast.success(`Deleted "${m.title}".`)
      if (editing?.id === m.id) setEditing(null)
      reload()
    } catch (err) {
      toast.error(errorMessage(err))
    }
  }

  return (
    <div className="grid gap-6 lg:grid-cols-[1fr_360px]">
      <div className="card overflow-x-auto">
        {(movies ?? []).length === 0 ? <p className="p-6 text-sm text-stone-500">No movies yet.</p> : (
          <table className="w-full text-left text-sm">
            <thead className="border-b border-stone-200 bg-stone-50 text-xs uppercase tracking-wide text-stone-500">
              <tr>
                <th className="px-4 py-3 font-semibold">Title</th>
                <th className="px-4 py-3 font-semibold">Language</th>
                <th className="px-4 py-3 font-semibold">Duration</th>
                <th className="px-4 py-3 font-semibold">Rating</th>
                <th className="px-4 py-3" />
              </tr>
            </thead>
            <tbody className="divide-y divide-stone-100">
              {(movies ?? []).map((m) => (
                <tr key={m.id} className={editing?.id === m.id ? 'bg-brand-50/50' : ''}>
                  <td className="px-4 py-3 font-medium">{m.title}</td>
                  <td className="px-4 py-3">{m.language}</td>
                  <td className="px-4 py-3">{formatDuration(m.durationMinutes)}</td>
                  <td className="px-4 py-3"><RatingBadge rating={m.rating} /></td>
                  <td className="px-4 py-3 text-right whitespace-nowrap">
                    <button className="text-xs font-semibold text-brand-700 hover:underline" onClick={() => setEditing(m)}>Edit</button>
                    <button className="ml-3 text-xs font-semibold text-red-700 hover:underline" onClick={() => remove(m)}>Delete</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
      <MovieForm
        key={editing ? `edit-${editing.id}` : `new-${formKey}`}
        movie={editing}
        onCancel={() => setEditing(null)}
        onSaved={() => { setEditing(null); setFormKey((k) => k + 1); reload() }}
      />
    </div>
  )
}
