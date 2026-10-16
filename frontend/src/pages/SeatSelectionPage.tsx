import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError, errorMessage } from '../api/client'
import { seatsApi } from '../api/endpoints'
import { Alert, BackLink, ErrorState, Spinner } from '../components/Feedback'
import { SeatLegend, SeatMap } from '../components/SeatMap'
import { useApi } from '../hooks/useApi'
import { useAuth } from '../hooks/useAuth'
import { useToast } from '../hooks/useToast'
import { formatDateLong, formatMoney, formatTime } from '../lib/format'
import type { Seat } from '../types'

const POLL_MS = 4000

export function SeatSelectionPage() {
  const showId = Number(useParams().id)
  const navigate = useNavigate()
  const toast = useToast()
  const { clearUser } = useAuth()
  const { data, error, loading, reload } = useApi(() => seatsApi.seatMap(showId), [showId])
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [submitting, setSubmitting] = useState(false)
  const [conflict, setConflict] = useState<string | null>(null)
  const initialised = useRef(false)

  // Poll availability so seats taken by others show up without a manual refresh.
  useEffect(() => {
    const timer = setInterval(() => {
      if (!submitting && document.visibilityState === 'visible') reload({ silent: true })
    }, POLL_MS)
    return () => clearInterval(timer)
  }, [reload, submitting])

  // Keep the selection consistent with the latest server state.
  useEffect(() => {
    if (!data) return
    if (!initialised.current) {
      // Returning to the page with an active hold: pre-select the held seats.
      initialised.current = true
      setSelected(new Set(data.seats.filter((s) => s.status === 'held_by_you').map((s) => s.id)))
      return
    }
    const unavailable = data.seats.filter((s) => selected.has(s.id) && (s.status === 'held' || s.status === 'booked'))
    if (unavailable.length > 0) {
      setSelected((prev) => {
        const next = new Set(prev)
        unavailable.forEach((s) => next.delete(s.id))
        return next
      })
      toast.error(`Seat ${unavailable.map((s) => s.label).join(', ')} was just taken by someone else.`)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data])

  if (loading && !data) return <Spinner label="Loading seat map…" />
  if (error && !data) return <ErrorState error={error} onRetry={reload} />
  if (!data) return null

  const { show, seats, maxSeats, activeHoldId } = data
  const started = new Date(show.startTime).getTime() <= Date.now()
  const selectedSeats = seats.filter((s) => selected.has(s.id))
  const total = selectedSeats.length * show.pricePaise

  function toggle(seat: Seat) {
    setConflict(null)
    if (!selected.has(seat.id) && selected.size >= maxSeats) {
      toast.error(`You can select at most ${maxSeats} seats per booking.`)
      return
    }
    const next = new Set(selected)
    if (next.has(seat.id)) next.delete(seat.id)
    else next.add(seat.id)
    setSelected(next)
  }

  async function onContinue() {
    setSubmitting(true)
    setConflict(null)
    try {
      const { hold } = await seatsApi.hold(showId, [...selected])
      navigate(`/checkout/${hold.id}`)
    } catch (err) {
      if (err instanceof ApiError && err.code === 'SEATS_UNAVAILABLE') {
        const taken = (err.body.conflictingSeatIds as number[]) ?? []
        const labels = (err.body.conflictingSeats as string[]) ?? []
        setSelected((prev) => new Set([...prev].filter((id) => !taken.includes(id))))
        setConflict(`Seat${labels.length > 1 ? 's' : ''} ${labels.join(', ')} ${labels.length > 1 ? 'are' : 'is'} no longer available. Please pick different seats.`)
        reload({ silent: true })
      } else if (err instanceof ApiError && err.status === 401) {
        clearUser()
        navigate(`/login?next=/shows/${showId}/seats`)
      } else {
        setConflict(errorMessage(err))
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div>
      <BackLink to={`/movies/${show.movieId}`}>{show.movieTitle}</BackLink>
      <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="page-title">Choose your seats</h1>
          <p className="mt-1 text-sm text-stone-600">
            {show.movieTitle} · {formatDateLong(show.startTime)} · {formatTime(show.startTime)} · {formatMoney(show.pricePaise)} per seat
          </p>
        </div>
        <p className="text-sm text-stone-500">{show.remainingSeats} of {show.totalSeats} seats available</p>
      </div>

      {activeHoldId && (
        <div className="mb-4">
          <Alert kind="info">
            You already have seats on hold for this show.{' '}
            <Link to={`/checkout/${activeHoldId}`} className="font-semibold underline">Resume checkout</Link> or pick new seats below (your previous hold will be replaced).
          </Alert>
        </div>
      )}
      {started && <div className="mb-4"><Alert kind="warning">This show has already started. Booking is closed.</Alert></div>}

      <div className="grid gap-6 lg:grid-cols-[1fr_300px]">
        <div className="card overflow-x-auto p-8">
          <SeatMap seats={seats} selected={selected} onToggle={toggle} readOnly={started} />
          <div className="mt-8 border-t border-stone-100 pt-5">
            <SeatLegend />
          </div>
        </div>

        <aside className="card h-fit p-6 lg:sticky lg:top-24">
          <h2 className="font-semibold">Your selection</h2>
          {selectedSeats.length === 0 ? (
            <p className="mt-2 text-sm text-stone-500">Select up to {maxSeats} seats on the map.</p>
          ) : (
            <div className="mt-3 flex flex-wrap gap-1.5">
              {selectedSeats.map((s) => (
                <span key={s.id} className="rounded-md bg-brand-50 px-2 py-1 text-xs font-semibold text-brand-800">{s.label}</span>
              ))}
            </div>
          )}
          <dl className="mt-5 space-y-1 border-t border-stone-100 pt-4 text-sm">
            <div className="flex justify-between text-stone-600"><dt>Seats</dt><dd>{selectedSeats.length} × {formatMoney(show.pricePaise)}</dd></div>
            <div className="flex justify-between text-base font-semibold"><dt>Total</dt><dd>{formatMoney(total)}</dd></div>
          </dl>
          {conflict && <div className="mt-4"><Alert>{conflict}</Alert></div>}
          <button className="btn-primary mt-5 w-full" disabled={selected.size === 0 || submitting || started} onClick={onContinue}>
            {submitting ? 'Holding seats…' : 'Continue'}
          </button>
          <p className="mt-2 text-center text-xs text-stone-500">Seats are held for 5 minutes while you pay.</p>
        </aside>
      </div>
    </div>
  )
}
