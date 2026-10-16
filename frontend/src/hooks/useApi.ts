import { useCallback, useEffect, useRef, useState } from 'react'

/**
 * Loads data with an async function and tracks loading / error state.
 * `reload()` re-runs it; stale responses from earlier calls are ignored.
 */
export function useApi<T>(fn: () => Promise<T>, deps: unknown[]) {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<unknown>(null)
  const [loading, setLoading] = useState(true)
  const callId = useRef(0)

  // eslint-disable-next-line react-hooks/exhaustive-deps
  const run = useCallback(fn, deps)

  const reload = useCallback(async (opts: { silent?: boolean } = {}) => {
    const id = ++callId.current
    if (!opts.silent) setLoading(true)
    try {
      const result = await run()
      if (id === callId.current) {
        setData(result)
        setError(null)
      }
    } catch (err) {
      if (id === callId.current) setError(err)
    } finally {
      if (id === callId.current) setLoading(false)
    }
  }, [run])

  useEffect(() => {
    reload()
  }, [reload])

  return { data, error, loading, reload, setData }
}
