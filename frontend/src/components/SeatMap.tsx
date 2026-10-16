import type { Seat, SeatStatus } from '../types'

interface Props {
  seats: Seat[]
  selected?: Set<number>
  onToggle?: (seat: Seat) => void
  readOnly?: boolean
}

// Visual style per state. Each state also differs by shape/marking, not just colour, and a
// text legend is always shown.
const seatStyle: Record<SeatStatus | 'selected', string> = {
  available: 'border-stone-300 bg-white text-stone-600 hover:border-brand-500 hover:bg-brand-50',
  selected: 'border-brand-700 bg-brand-600 text-white shadow-sm',
  held: 'border-amber-300 bg-amber-100 text-amber-700 bg-[repeating-linear-gradient(45deg,transparent,transparent_3px,rgba(217,119,6,.18)_3px,rgba(217,119,6,.18)_6px)]',
  held_by_you: 'border-brand-300 bg-brand-100 text-brand-800',
  booked: 'border-stone-400 bg-stone-400 text-white',
}

const statusText: Record<SeatStatus, string> = {
  available: 'available',
  held: 'held by another customer',
  held_by_you: 'on hold for you',
  booked: 'booked',
}

export function SeatMap({ seats, selected = new Set(), onToggle, readOnly = false }: Props) {
  const rows = [...new Set(seats.map((s) => s.row))]
  return (
    <div className="select-none">
      <div className="mx-auto mb-8 max-w-xl">
        <div className="h-2 rounded-t-full bg-gradient-to-b from-stone-400 to-stone-200" />
        <p className="mt-2 text-center text-xs font-semibold uppercase tracking-[0.3em] text-stone-500">Screen this way</p>
      </div>
      <div className="flex flex-col items-center gap-2">
        {rows.map((row) => (
          <div key={row} className="flex items-center gap-2">
            <span className="w-5 text-right text-xs font-semibold text-stone-400">{row}</span>
            {seats.filter((s) => s.row === row).map((seat) => {
              const isSelected = selected.has(seat.id)
              const selectable = !readOnly && (seat.status === 'available' || seat.status === 'held_by_you')
              const style = isSelected ? seatStyle.selected : seatStyle[seat.status]
              return (
                <button
                  key={seat.id}
                  type="button"
                  disabled={!selectable}
                  onClick={() => onToggle?.(seat)}
                  aria-pressed={isSelected}
                  aria-label={`Seat ${seat.label}, ${isSelected ? 'selected' : statusText[seat.status]}`}
                  title={`${seat.label} — ${isSelected ? 'selected' : statusText[seat.status]}`}
                  className={`relative h-8 w-8 rounded-t-lg rounded-b-sm border text-[10px] font-semibold transition
                    ${style} ${seat.number === 5 ? 'mr-5' : ''} ${selectable ? 'cursor-pointer' : 'cursor-not-allowed'}`}
                >
                  {seat.status === 'booked' && !isSelected ? '✕' : seat.number}
                </button>
              )
            })}
            <span className="w-5 text-xs font-semibold text-stone-400">{row}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

export function SeatLegend({ showSelected = true, showYours = true }: { showSelected?: boolean; showYours?: boolean }) {
  const items: { style: string; label: string; mark?: string }[] = [
    { style: seatStyle.available, label: 'Available' },
    ...(showSelected ? [{ style: seatStyle.selected, label: 'Selected' }] : []),
    ...(showYours ? [{ style: seatStyle.held_by_you, label: 'Your hold' }] : []),
    { style: seatStyle.held, label: 'Held by another customer' },
    { style: seatStyle.booked, label: 'Booked', mark: '✕' },
  ]
  return (
    <ul className="flex flex-wrap items-center justify-center gap-x-5 gap-y-2 text-xs text-stone-600">
      {items.map((i) => (
        <li key={i.label} className="flex items-center gap-2">
          <span className={`inline-flex h-4 w-4 items-center justify-center rounded-t-md rounded-b-sm border text-[8px] ${i.style}`}>{i.mark}</span>
          {i.label}
        </li>
      ))}
    </ul>
  )
}
