// Reading of a frequency response sweep: the gain in dB, the phase unwrapped
// along the frequency, the -3 dB points and the maximum.
import type { SweepPoint } from './types'

export interface BodePoint {
  freq: number
  db: number
  phase: number // degrees, unwrapped
}

// bodePoints keeps the measured points. The phase is unwrapped, so that a
// filter of a higher order goes on below −180° instead of jumping to +180°.
export function bodePoints(ps: SweepPoint[]): BodePoint[] {
  const out: BodePoint[] = []
  for (const p of ps) {
    if (p.gain == null || p.phase == null || !(p.gain > 0)) continue
    let phase = p.phase
    const prev = out[out.length - 1]
    if (prev) phase += 360 * Math.round((prev.phase - phase) / 360)
    out.push({ freq: p.freq, db: 20 * Math.log10(p.gain), phase })
  }
  return out
}

export interface Cutoff {
  freq: number
  phase: number
  rising: boolean // the gain rises through the level: a lower cutoff
}

// cutoffs finds where the gain crosses `drop` dB below its maximum,
// interpolated on the log frequency axis.
export function cutoffs(b: BodePoint[], drop = 3): { ref: number; list: Cutoff[] } | null {
  if (b.length < 2) return null
  const ref = Math.max(...b.map((p) => p.db))
  const level = ref - drop
  const list: Cutoff[] = []
  for (let i = 1; i < b.length; i++) {
    const p = b[i - 1], q = b[i]
    if (p.db >= level === q.db >= level) continue
    const t = (level - p.db) / (q.db - p.db)
    list.push({
      freq: Math.exp(Math.log(p.freq) + t * (Math.log(q.freq) - Math.log(p.freq))),
      phase: p.phase + t * (q.phase - p.phase),
      rising: q.db > p.db,
    })
  }
  return { ref, list }
}

// maximum is the highest point when it lies inside the sweep and stands at
// least 1 dB above both ends, as at a resonance or the centre of a band
// pass. A parabola through it and its neighbours on the log frequency axis
// places it between the points.
export function maximum(b: BodePoint[]): { freq: number; db: number } | null {
  if (b.length < 3) return null
  let k = 0
  for (let i = 1; i < b.length; i++) if (b[i].db > b[k].db) k = i
  if (k === 0 || k === b.length - 1 || b[k].db - b[0].db < 1 || b[k].db - b[b.length - 1].db < 1) return null
  const [x0, x1, x2] = [k - 1, k, k + 1].map((i) => Math.log(b[i].freq))
  const [y0, y1, y2] = [k - 1, k, k + 1].map((i) => b[i].db)
  const d = (x0 - x1) * (x0 - x2) * (x1 - x2)
  const A = (x2 * (y1 - y0) + x1 * (y0 - y2) + x0 * (y2 - y1)) / d
  const B = (x2 * x2 * (y0 - y1) + x1 * x1 * (y2 - y0) + x0 * x0 * (y1 - y2)) / d
  const C = (x1 * x2 * (x1 - x2) * y0 + x2 * x0 * (x2 - x0) * y1 + x0 * x1 * (x0 - x1) * y2) / d
  if (!(A < 0)) return { freq: b[k].freq, db: b[k].db }
  const x = Math.min(x2, Math.max(x0, -B / (2 * A)))
  return { freq: Math.exp(x), db: A * x * x + B * x + C }
}
