import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError, errorMessage } from '../api/client'
import { seatsApi } from '../api/endpoints'
import { Alert, ErrorState, Spinner } from '../components/Feedback'
import { HoldCountdown } from '../components/HoldCountdown'
import { useApi } from '../hooks/useApi'
import { useCountdown } from '../hooks/useCountdown'
import { useToast } from '../hooks/useToast'
import { formatDateLong, formatMoney, formatTime } from '../lib/format'

type Step = 'review' | 'gateway' | 'failed'

export function CheckoutPage() {
  const holdId = useParams().holdId!
  const navigate = useNavigate()
  const toast = useToast()
  const { data, error, loading, reload } = useApi(() => seatsApi.getHold(holdId), [holdId])
  const [step, setStep] = useState<Step>('review')
  const [busy, setBusy] = useState<'success' | 'failure' | 'release' | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const [serverExpired, setServerExpired] = useState(false)
  const remaining = useCountdown(data?.hold.expiresAt, data?.serverTime)

  const hold = data?.hold

  // An already-confirmed hold (e.g. after a page refresh) goes straight to its booking.
  useEffect(() => {
    if (hold?.status === 'CONFIRMED' && hold.bookingId) navigate(`/bookings/${hold.bookingId}`, { replace: true })
  }, [hold, navigate])

  if (loading) return <Spinner label="Loading checkout…" />
  if (error) return <ErrorState error={error} onRetry={reload} />
  if (!hold) return null

  const expired = serverExpired || hold.status === 'EXPIRED' || hold.status === 'RELEASED' || remaining <= 0
  const { show } = hold

  async function pay(outcome: 'success' | 'failure') {
    setBusy(outcome)
    setMessage(null)
    try {
      const res = await seatsApi.pay(holdId, outcome)
      toast.success('Payment successful — your booking is confirmed!')
      navigate(`/bookings/${res.booking.id}?new=1`, { replace: true })
    } catch (err) {
      if (err instanceof ApiError && err.code === 'PAYMENT_FAILED') {
        setStep('failed')
        setMessage(err.message)
      } else if (err instanceof ApiError && (err.code === 'HOLD_EXPIRED' || err.code === 'HOLD_RELEASED')) {
        setServerExpired(true)
      } else if (err instanceof ApiError && err.code === 'HOLD_ALREADY_CONFIRMED') {
        reload()
      } else {
        setMessage(errorMessage(err))
      }
    } finally {
      setBusy(null)
    }
  }

  async function changeSeats() {
    setBusy('release')
    try {
      await seatsApi.releaseHold(holdId)
    } catch {
      /* the hold may already be gone; either way go back to the seat map */
    }
    navigate(`/shows/${show.id}/seats`)
  }

  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="page-title mb-6">Checkout</h1>
      <div className="grid gap-6 md:grid-cols-[1fr_280px]">
        <section className="card p-6">
          <h2 className="text-lg font-semibold">{show.movieTitle}</h2>
          <p className="mt-1 text-sm text-stone-600">{show.movieLanguage} · {show.movieRating}</p>
          <dl className="mt-5 grid grid-cols-2 gap-4 text-sm">
            <div><dt className="text-stone-500">Date</dt><dd className="font-medium">{formatDateLong(show.startTime)}</dd></div>
            <div><dt className="text-stone-500">Time</dt><dd className="font-medium">{formatTime(show.startTime)}</dd></div>
            <div className="col-span-2">
              <dt className="text-stone-500">Seats</dt>
              <dd className="mt-1 flex flex-wrap gap-1.5">
                {hold.seats.map((s) => <span key={s.id} className="rounded-md bg-brand-50 px-2 py-1 text-xs font-semibold text-brand-800">{s.label}</span>)}
              </dd>
            </div>
          </dl>
          <dl className="mt-6 space-y-1 border-t border-stone-100 pt-4 text-sm">
            <div className="flex justify-between text-stone-600"><dt>{hold.seats.length} × ticket</dt><dd>{formatMoney(hold.pricePaise)}</dd></div>
            <div className="flex justify-between text-lg font-bold"><dt>Total</dt><dd>{formatMoney(hold.totalPaise)}</dd></div>
          </dl>
        </section>

        <aside className="space-y-4">
          {expired ? (
            <div className="card p-6 text-center">
              <p className="font-semibold text-stone-900">Your hold has expired</p>
              <p className="mt-1 text-sm text-stone-600">The seats have been released. No payment was taken.</p>
              <Link to={`/shows/${show.id}/seats`} className="btn-primary mt-4 w-full">Choose seats again</Link>
            </div>
          ) : (
            <>
              <HoldCountdown seconds={remaining} />
              {message && <Alert kind={step === 'failed' ? 'error' : 'warning'}>{message}</Alert>}

              {step === 'review' && (
                <div className="card space-y-3 p-6">
                  <button className="btn-primary w-full" onClick={() => setStep('gateway')}>Pay {formatMoney(hold.totalPaise)}</button>
                  <button className="btn-ghost w-full" onClick={changeSeats} disabled={busy !== null}>Change seats</button>
                </div>
              )}

              {step === 'gateway' && (
                <div className="card overflow-hidden">
                  <div className="border-b border-stone-200 bg-stone-900 px-5 py-3 text-white">
                    <p className="text-xs uppercase tracking-wider text-stone-400">Fake payment gateway</p>
                    <p className="text-lg font-semibold">{formatMoney(hold.totalPaise)}</p>
                  </div>
                  <div className="space-y-3 p-5">
                    <p className="text-xs text-stone-500">This is a simulation. Choose the outcome:</p>
                    <button className="btn w-full bg-emerald-600 text-white hover:bg-emerald-700" disabled={busy !== null} onClick={() => pay('success')}>
                      {busy === 'success' ? 'Processing…' : 'Payment success'}
                    </button>
                    <button className="btn w-full bg-red-600 text-white hover:bg-red-700" disabled={busy !== null} onClick={() => pay('failure')}>
                      {busy === 'failure' ? 'Processing…' : 'Payment failure'}
                    </button>
                    <button className="btn-ghost w-full" disabled={busy !== null} onClick={() => setStep('review')}>Cancel</button>
                  </div>
                </div>
              )}

              {step === 'failed' && (
                <div className="card space-y-3 p-6">
                  <p className="text-sm text-stone-600">Your seats are still held until the timer runs out.</p>
                  <button className="btn-primary w-full" onClick={() => { setMessage(null); setStep('gateway') }}>Try again</button>
                  <button className="btn-ghost w-full" onClick={changeSeats} disabled={busy !== null}>Change seats</button>
                </div>
              )}
            </>
          )}
        </aside>
      </div>
    </div>
  )
}
