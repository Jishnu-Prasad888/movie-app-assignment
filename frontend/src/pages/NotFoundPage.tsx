import { Link } from 'react-router-dom'

export function NotFoundPage() {
  return (
    <div className="card mx-auto max-w-md p-10 text-center">
      <p className="text-4xl font-bold text-stone-300">404</p>
      <p className="mt-2 font-semibold">Page not found</p>
      <Link to="/" className="btn-primary mt-6">Back to movies</Link>
    </div>
  )
}
