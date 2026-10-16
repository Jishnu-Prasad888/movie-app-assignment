// Thin fetch wrapper.
//
// Authentication uses an HttpOnly session cookie set by the API, so this code never sees or
// stores a token: the browser attaches the cookie automatically (same origin). Every request
// sends X-Requested-With, which the API requires on state-changing requests as a CSRF defence.

export class ApiError extends Error {
  status: number
  code: string
  body: Record<string, unknown>

  constructor(status: number, body: Record<string, unknown>) {
    super(typeof body.error === 'string' ? body.error : `Request failed (${status})`)
    this.status = status
    this.code = typeof body.code === 'string' ? body.code : 'UNKNOWN'
    this.body = body
  }

  /** Per-field validation messages returned by the API, if any. */
  get fields(): Record<string, string> {
    return (this.body.fields as Record<string, string>) ?? {}
  }
}

export async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  let res: Response
  try {
    res = await fetch(`/api${path}`, {
      method,
      credentials: 'same-origin',
      headers: {
        'X-Requested-With': 'XMLHttpRequest',
        ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}),
      },
      body: body !== undefined ? JSON.stringify(body) : undefined,
    })
  } catch {
    throw new ApiError(0, { error: 'Cannot reach the server. Check your connection and try again.', code: 'NETWORK' })
  }
  if (res.status === 204) return undefined as T
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new ApiError(res.status, data)
  return data as T
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body ?? {}),
  put: <T>(path: string, body: unknown) => request<T>('PUT', path, body),
  del: <T = void>(path: string) => request<T>('DELETE', path),
}

export function errorMessage(err: unknown): string {
  if (err instanceof Error) return err.message
  return 'Something went wrong.'
}
