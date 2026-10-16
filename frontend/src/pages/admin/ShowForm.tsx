import { useState, type FormEvent } from 'react'
import { ApiError, errorMessage } from '../../api/client'
import { adminApi } from '../../api/endpoints'
import { Alert } from '../../components/Feedback'
import { useToast } from '../../hooks/useToast'
import { cinemaDateKey, formatDuration, formatTime, getCinemaTimeZone } from '../../lib/format'
import type { Movie } from '../../types'

export function ShowForm({ movies, onCreated }: { movies: Movie[]; onCreated: () => void }) {
  const toast = useToast()
  const tomorrow = cinemaDateKey(new Date(Date.now() + 24 * 3600 * 1000))
  const [movieId, setMovieId] = useState('')
  const [date, setDate] = useState(tomorrow)
  const [time, setTime] = useState('18:00')
  const [price, setPrice] = useState('250')
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [formError, setFormError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const movie = movies.find((m) => m.id === Number(movieId))

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setErrors({})
    setFormError(null)
    const rupees = Number(price)
    const local: Record<string, string> = {}
    if (!movieId) local.movieId = 'Choose a movie.'
    if (!date || !time) local.startTime = 'Choose a date and start time.'
    if (!Number.isFinite(rupees) || rupees < 1) local.pricePaise = 'Enter a ticket price of at least ₹1.'
    if (Object.keys(local).length) {
      setErrors(local)
      return
    }
    setBusy(true)
    try {
      // Price converted to integer paise; date/time are sent as the cinema's local wall-clock
      // time and converted to an absolute instant by the server.
      await adminApi.createShow({ movieId: Number(movieId), date, startTime: time, pricePaise: Math.round(rupees * 100) })
      toast.success('Show scheduled.')
      onCreated()
    } catch (err) {
      if (err instanceof ApiError && err.code === 'SHOW_OVERLAP') {
        const c = err.body.conflictingShow as { movieTitle: string; startTime: string; endTime: string } | undefined
        setFormError(c ? `Overlaps with “${c.movieTitle}” (${formatTime(c.startTime)} – ${formatTime(c.endTime)}) on the same screen.` : err.message)
      } else if (err instanceof ApiError && Object.keys(err.fields).length) {
        setErrors(err.fields)
      } else {
        setFormError(errorMessage(err))
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={onSubmit} className="card h-fit space-y-4 p-6" noValidate>
      <div>
        <h2 className="font-semibold">Schedule a show</h2>
        <p className="text-xs text-stone-500">Times are in cinema time ({getCinemaTimeZone()}).</p>
      </div>
      {formError && <Alert>{formError}</Alert>}
      <div>
        <label className="label" htmlFor="s-movie">Movie</label>
        <select id="s-movie" className="input" value={movieId} onChange={(e) => setMovieId(e.target.value)}>
          <option value="">Select…</option>
          {movies.map((m) => <option key={m.id} value={m.id}>{m.title}</option>)}
        </select>
        {movie && <p className="mt-1 text-xs text-stone-500">Runs {formatDuration(movie.durationMinutes)}</p>}
        {errors.movieId && <p className="field-error">{errors.movieId}</p>}
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="label" htmlFor="s-date">Date</label>
          <input id="s-date" type="date" className="input" value={date} onChange={(e) => setDate(e.target.value)} />
        </div>
        <div>
          <label className="label" htmlFor="s-time">Start time</label>
          <input id="s-time" type="time" className="input" value={time} onChange={(e) => setTime(e.target.value)} />
        </div>
      </div>
      {errors.startTime && <p className="field-error -mt-2">{errors.startTime}</p>}
      <div>
        <label className="label" htmlFor="s-price">Ticket price (₹)</label>
        <input id="s-price" type="number" min={1} step="0.01" className="input" value={price} onChange={(e) => setPrice(e.target.value)} />
        {errors.pricePaise && <p className="field-error">{errors.pricePaise}</p>}
      </div>
      <button type="submit" className="btn-primary w-full" disabled={busy}>{busy ? 'Scheduling…' : 'Add show'}</button>
    </form>
  )
}
