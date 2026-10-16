import { Link } from 'react-router-dom'
import { moviesApi } from '../api/endpoints'
import { EmptyState, ErrorState, Spinner } from '../components/Feedback'
import { Poster, RatingBadge } from '../components/Poster'
import { useApi } from '../hooks/useApi'
import { formatDuration } from '../lib/format'

export function HomePage() {
  const { data, error, loading, reload } = useApi(() => moviesApi.list(), [])

  if (loading) return <Spinner label="Loading movies…" />
  if (error) return <ErrorState error={error} onRetry={reload} />
  const movies = data?.movies ?? []

  return (
    <div>
      <div className="mb-6">
        <h1 className="page-title">Now showing</h1>
        <p className="mt-1 text-sm text-stone-600">Pick a movie to see show times and book seats.</p>
      </div>
      {movies.length === 0 ? (
        <EmptyState title="No movies yet">Check back soon — new movies are added regularly.</EmptyState>
      ) : (
        <div className="grid grid-cols-2 gap-6 md:grid-cols-3 lg:grid-cols-4">
          {movies.map((m) => (
            <Link key={m.id} to={`/movies/${m.id}`} className="group card overflow-hidden transition hover:-translate-y-0.5 hover:shadow-md">
              <Poster title={m.title} url={m.posterUrl} className="aspect-[2/3] w-full" />
              <div className="p-4">
                <h2 className="font-semibold text-stone-900 group-hover:text-brand-700">{m.title}</h2>
                <div className="mt-1 flex items-center gap-2 text-xs text-stone-500">
                  <RatingBadge rating={m.rating} />
                  <span>{m.language}</span>
                  <span aria-hidden>·</span>
                  <span>{formatDuration(m.durationMinutes)}</span>
                </div>
                <p className="mt-2 line-clamp-2 text-sm text-stone-600">{m.description}</p>
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}
