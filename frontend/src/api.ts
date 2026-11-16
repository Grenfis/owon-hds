// Bridge to the Go side. In the desktop window Wails injects window.go and
// window.runtime; in browser mode (cmd/owon-web) the same calls go over HTTP
// and events arrive as server-sent events.

type Handler = (data: any) => void

const w = window as any
export const isDesktop = typeof w.go?.app?.App === 'object'

export async function call<T = unknown>(method: string, ...args: unknown[]): Promise<T> {
  if (isDesktop) return w.go.app.App[method](...args)
  const r = await fetch('/api/call', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ method, args }),
  })
  if (!r.ok) throw new Error(`${method}: HTTP ${r.status}`)
  const j = await r.json()
  if (j.error) throw new Error(j.error)
  return j.result as T
}

const handlers = new Map<string, Set<Handler>>()
let source: EventSource | null = null

export function on(event: string, fn: Handler): () => void {
  if (isDesktop) return w.runtime.EventsOn(event, fn)
  if (!source) {
    source = new EventSource('/api/events')
    source.onmessage = (e) => {
      const { event, data } = JSON.parse(e.data)
      handlers.get(event)?.forEach((h) => h(data))
    }
  }
  if (!handlers.has(event)) handlers.set(event, new Set())
  handlers.get(event)!.add(fn)
  return () => handlers.get(event)?.delete(fn)
}

// saveFile stores a file through the native dialog in the desktop window,
// or as a browser download otherwise. Returns the path or file name, or ''
// when the user cancelled.
export async function saveFile(name: string, data: Blob): Promise<string> {
  if (!isDesktop) {
    const a = document.createElement('a')
    a.href = URL.createObjectURL(data)
    a.download = name
    a.click()
    setTimeout(() => URL.revokeObjectURL(a.href), 1000)
    return name
  }
  const buf = new Uint8Array(await data.arrayBuffer())
  let bin = ''
  for (let i = 0; i < buf.length; i += 0x8000) bin += String.fromCharCode(...buf.subarray(i, i + 0x8000))
  return call<string>('SaveFile', name, btoa(bin))
}
