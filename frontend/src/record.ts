// Recording of frames: kept in memory, saved to and read from a JSON file,
// and shown again one by one. Also what tells a new single-shot capture
// from the same capture read again.
import type { Frame } from './types'

export interface RecFrame {
  t: number // ms since 1970, when the frame came
  frame: Frame
}

export const MAX_FRAMES = 2000
const FORMAT = 'owon-hds272s-frames'

export function recordingJSON(frames: RecFrame[]): string {
  return JSON.stringify({ format: FORMAT, version: 1, frames })
}

// parseRecording returns the frames of a file, or what is wrong with it.
export function parseRecording(text: string): RecFrame[] | string {
  let d: any
  try {
    d = JSON.parse(text)
  } catch {
    return 'Это не JSON'
  }
  if (d?.format !== FORMAT || !Array.isArray(d.frames)) return 'Это не запись кадров этой программы'
  const frames = (d.frames as any[]).filter(
    (r) => typeof r?.t === 'number' && Array.isArray(r.frame?.channels) && typeof r.frame.timeDiv === 'number',
  )
  return frames.length ? frames.slice(0, MAX_FRAMES) : 'В файле нет кадров'
}

// sameData tells a frame that repeats the samples of another: a stopped
// device keeps sending its last capture.
export function sameData(a: Frame, b: Frame): boolean {
  if (a.channels.length !== b.channels.length) return false
  return a.channels.every((c, i) => {
    const x = c.raw ?? [], y = b.channels[i].raw ?? []
    return x.length === y.length && x.every((v, j) => v === y[j])
  })
}

// scopeOf gives the settings a recorded frame was taken with, over the
// current ones, for what the screen draws: which channels are on, their
// offsets, the trigger point and level.
export function scopeOf(f: Frame, base: Record<string, string>): Record<string, string> {
  const s = { ...base }
  for (const c of f.channels) {
    const k = c.name.toLowerCase()
    s[`${k}.display`] = c.on ? 'ON' : 'OFF'
    s[`${k}.offset`] = (c.offset / 25).toFixed(2)
  }
  s['hor.offset'] = String(f.hOffset)
  if (f.trig?.source) s['trig.source'] = f.trig.source.toUpperCase()
  if (f.trig?.levelStr) s['trig.level'] = String(f.trig.level)
  return s
}
