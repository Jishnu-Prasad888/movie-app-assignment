// Formatting helpers. All show times are displayed in the cinema's time zone (from
// /api/config), not the browser's, so every visitor sees the same local show time.

let cinemaTimeZone = 'Asia/Kolkata'

export function setCinemaTimeZone(tz: string) {
  cinemaTimeZone = tz
}

export function getCinemaTimeZone() {
  return cinemaTimeZone
}

const inr = new Intl.NumberFormat('en-IN', { style: 'currency', currency: 'INR', minimumFractionDigits: 0, maximumFractionDigits: 2 })

/** Money is handled as integer paise everywhere; converted only for display. */
export function formatMoney(paise: number): string {
  return inr.format(paise / 100)
}

export function formatTime(iso: string): string {
  return new Intl.DateTimeFormat('en-IN', { timeZone: cinemaTimeZone, hour: 'numeric', minute: '2-digit', hour12: true }).format(new Date(iso))
}

export function formatDate(iso: string): string {
  return new Intl.DateTimeFormat('en-IN', { timeZone: cinemaTimeZone, weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' }).format(new Date(iso))
}

export function formatDateLong(iso: string): string {
  return new Intl.DateTimeFormat('en-IN', { timeZone: cinemaTimeZone, weekday: 'long', day: 'numeric', month: 'long' }).format(new Date(iso))
}

export function formatDateTime(iso: string): string {
  return `${formatDate(iso)}, ${formatTime(iso)}`
}

/** Calendar date (YYYY-MM-DD) of an instant in the cinema's time zone, used for grouping. */
export function cinemaDateKey(iso: string | Date): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: cinemaTimeZone, year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date(iso))
}

export function formatDuration(minutes: number): string {
  const h = Math.floor(minutes / 60)
  const m = minutes % 60
  return h > 0 ? `${h}h ${m.toString().padStart(2, '0')}m` : `${m}m`
}

export function bookingCode(id: number): string {
  return `YZ-${id.toString().padStart(6, '0')}`
}

export const ratingLabel: Record<string, string> = {
  U: 'U · All ages',
  UA: 'UA · Parental guidance',
  A: 'A · Adults only',
}
