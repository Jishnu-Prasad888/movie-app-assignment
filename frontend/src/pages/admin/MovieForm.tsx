import { useState, type FormEvent } from 'react'
import { ApiError, errorMessage } from '../../api/client'
import { adminApi } from '../../api/endpoints'
import { Alert } from '../../components/Feedback'
import { useToast } from '../../hooks/useToast'
import type { Movie, MovieInput, Rating } from '../../types'

export function MovieForm({ movie, onSaved, onCancel }: { movie: Movie | null; onSaved: () => void; onCancel: () => void }) {
  const toast = useToast()
  const [form, setForm] = useState({
    title: movie?.title ?? '',
    description: movie?.description ?? '',
    language: movie?.language ?? '',
    durationMinutes: movie?.durationMinutes.toString() ?? '',
    rating: (movie?.rating ?? 'UA') as Rating,
    posterUrl: movie?.posterUrl ?? '',
  })
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [formError, setFormError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const set = (k: keyof typeof form) => (e: { target: { value: string } }) => setForm((f) => ({ ...f, [k]: e.target.value }))

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setErrors({})
    setFormError(null)
    const input: MovieInput = {
      title: form.title,
      description: form.description,
      language: form.language,
      durationMinutes: Number(form.durationMinutes) || 0,
      rating: form.rating,
      posterUrl: form.posterUrl.trim() || null,
    }
    setBusy(true)
    try {
      if (movie) await adminApi.updateMovie(movie.id, input)
      else await adminApi.createMovie(input)
      toast.success(movie ? 'Movie updated.' : 'Movie added.')
      onSaved()
    } catch (err) {
      if (err instanceof ApiError && Object.keys(err.fields).length) setErrors(err.fields)
      else setFormError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={onSubmit} className="card h-fit space-y-4 p-6" noValidate>
      <h2 className="font-semibold">{movie ? `Edit “${movie.title}”` : 'Add a movie'}</h2>
      {formError && <Alert>{formError}</Alert>}
      <div>
        <label className="label" htmlFor="m-title">Title</label>
        <input id="m-title" className="input" value={form.title} onChange={set('title')} />
        {errors.title && <p className="field-error">{errors.title}</p>}
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="label" htmlFor="m-lang">Language</label>
          <input id="m-lang" className="input" value={form.language} onChange={set('language')} />
          {errors.language && <p className="field-error">{errors.language}</p>}
        </div>
        <div>
          <label className="label" htmlFor="m-dur">Duration (min)</label>
          <input id="m-dur" type="number" min={1} max={600} className="input" value={form.durationMinutes} onChange={set('durationMinutes')} />
          {errors.durationMinutes && <p className="field-error">{errors.durationMinutes}</p>}
        </div>
      </div>
      <div>
        <label className="label" htmlFor="m-rating">Rating</label>
        <select id="m-rating" className="input" value={form.rating} onChange={set('rating')}>
          <option value="U">U — All ages</option>
          <option value="UA">UA — Parental guidance</option>
          <option value="A">A — Adults only</option>
        </select>
        {errors.rating && <p className="field-error">{errors.rating}</p>}
      </div>
      <div>
        <label className="label" htmlFor="m-poster">Poster URL <span className="font-normal text-stone-400">(optional)</span></label>
        <input id="m-poster" className="input" placeholder="https://…" value={form.posterUrl} onChange={set('posterUrl')} />
        {errors.posterUrl && <p className="field-error">{errors.posterUrl}</p>}
      </div>
      <div>
        <label className="label" htmlFor="m-desc">Short description</label>
        <textarea id="m-desc" rows={3} className="input" value={form.description} onChange={set('description')} />
        {errors.description && <p className="field-error">{errors.description}</p>}
      </div>
      {movie && <p className="text-xs text-stone-500">Changing the duration updates the end time of upcoming shows (rejected if they would overlap).</p>}
      <div className="flex gap-2">
        <button type="submit" className="btn-primary flex-1" disabled={busy}>{busy ? 'Saving…' : movie ? 'Save changes' : 'Add movie'}</button>
        {movie && <button type="button" className="btn-secondary" onClick={onCancel}>Cancel</button>}
      </div>
    </form>
  )
}
