import { Link, useParams } from 'react-router-dom'
import { moviesApi } from '../api/endpoints'
import { BackLink, EmptyState, ErrorState, Spinner } from '../components/Feedback'
import { Poster, RatingBadge } from '../components/Poster'
import { useApi } from '../hooks/useApi'
import { cinemaDateKey, formatDateLong, formatDuration, formatMoney, formatTime, ratingLabel } from '../lib/format'
import type { Show } from '../types'

export function MovieDetailPage() {
  const id = Number(useParams().id)
  const { data, error, loading, reload } = useApi(() => moviesApi.get(id), [id])

  if (loading) return <Spinner />
  if (error) return <ErrorState error={error} onRetry={reload} />
  if (!data) return null
  const { movie, shows } = data

  // Group upcoming shows by calendar date in the cinema's time zone.
  const groups = new Map<string, Show[]>()
  for (const s of shows) {
    const key = cinemaDateKey(s.startTime)
    groups.set(key, [...(groups.get(key) ?? []), s])
  }

  return (
    <div>
      <BackLink to="/">All movies</BackLink>
      <div className="grid gap-8 md:grid-cols-[240px_1fr]">
        <Poster title={movie.title} url={movie.posterUrl} className="aspect-[2/3] w-full rounded-xl shadow-sm" />
        <div>
          <h1 className="text-3xl font-bold tracking-tight">{movie.title}</h1>
          <div className="mt-2 flex flex-wrap items-center gap-3 text-sm text-stone-600">
            <RatingBadge rating={movie.rating} />
            <span>{ratingLabel[movie.rating]}</span>
            <span aria-hidden>·</span>
            <span>{movie.language}</span>
            <span aria-hidden>·</span>
            <span>{formatDuration(movie.durationMinutes)}</span>
          </div>
          <p className="mt-4 max-w-2xl leading-relaxed text-stone-700">{movie.description || 'No description available.'}</p>

          <h2 className="mt-8 mb-3 text-lg font-semibold">Show times</h2>
          {shows.length === 0 ? (
            <EmptyState title="No upcoming shows">This movie has no upcoming shows right now.</EmptyState>
          ) : (
            <div className="space-y-5">
              {[...groups.entries()].map(([key, list]) => (
                <section key={key}>
                  <h3 className="mb-2 text-sm font-semibold text-stone-500">{formatDateLong(list[0].startTime)}</h3>
                  <div className="flex flex-wrap gap-3">
                    {list.map((s) => <ShowButton key={s.id} show={s} />)}
                  </div>
                </section>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function ShowButton({ show }: { show: Show }) {
  const soldOut = show.remainingSeats <= 0
  const low = !soldOut && show.remainingSeats <= 10
  const inner = (
    <>
      <span className="text-base font-semibold">{formatTime(show.startTime)}</span>
      <span className="text-xs text-stone-500">{formatMoney(show.pricePaise)}</span>
      <span className={`text-xs font-medium ${soldOut ? 'text-stone-400' : low ? 'text-amber-700' : 'text-emerald-700'}`}>
        {soldOut ? 'Sold out' : `${show.remainingSeats} seats left`}
      </span>
    </>
  )
  const cls = 'flex w-36 flex-col items-start gap-0.5 rounded-lg border px-4 py-3 text-left transition'
  if (soldOut) return <div className={`${cls} cursor-not-allowed border-stone-200 bg-stone-50 opacity-70`}>{inner}</div>
  return <Link to={`/shows/${show.id}/seats`} className={`${cls} border-stone-300 bg-white hover:border-brand-500 hover:shadow-sm`}>{inner}</Link>
}
