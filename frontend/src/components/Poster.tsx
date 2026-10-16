import { useState } from 'react'

// Deterministic gradient so each movie without a poster still looks distinct.
const gradients = [
  'from-rose-700 to-orange-500',
  'from-indigo-700 to-sky-500',
  'from-emerald-700 to-teal-400',
  'from-fuchsia-700 to-pink-400',
  'from-amber-700 to-yellow-400',
  'from-slate-800 to-slate-500',
]

export function Poster({ title, url, className = '' }: { title: string; url: string | null; className?: string }) {
  const [failed, setFailed] = useState(false)
  if (url && !failed) {
    return <img src={url} alt={`${title} poster`} onError={() => setFailed(true)} className={`object-cover ${className}`} loading="lazy" />
  }
  const g = gradients[[...title].reduce((a, c) => a + c.charCodeAt(0), 0) % gradients.length]
  const initials = title.split(/\s+/).filter(Boolean).slice(0, 2).map((w) => w[0]?.toUpperCase()).join('')
  return (
    <div className={`flex flex-col items-center justify-center bg-gradient-to-br ${g} p-4 text-center text-white ${className}`} aria-label={`${title} (no poster)`}>
      <span className="text-4xl font-bold tracking-tight opacity-90">{initials}</span>
      <span className="mt-2 line-clamp-2 text-xs font-medium uppercase tracking-wider opacity-80">{title}</span>
    </div>
  )
}

export function RatingBadge({ rating }: { rating: string }) {
  const color = rating === 'A' ? 'border-red-300 text-red-700' : rating === 'UA' ? 'border-amber-300 text-amber-700' : 'border-emerald-300 text-emerald-700'
  return <span className={`inline-flex rounded border px-1.5 py-0.5 text-xs font-bold ${color}`} title={`Rated ${rating}`}>{rating}</span>
}
