// Reading of a distortion sweep: the distortion of the output against the
// amplitude, where it passes a limit, and where the gain compresses.
import type { SweepPoint } from './types'

export interface ThdPoint {
  amp: number // generator Vpp
  in: number // CH1 AC RMS, volts
  out: number // CH2 AC RMS
  db: number // gain
  thd: number // CH2, a fraction
  thdIn: number // CH1: the generator and the ADC
  harm: number[] // CH2 from the 2nd harmonic on, against the fundamental
}

export function thdPoints(ps: SweepPoint[]): ThdPoint[] {
  const out: ThdPoint[] = []
  for (const p of ps) {
    if (p.gain == null || !(p.gain > 0) || p.thd == null || p.thdIn == null || p.in == null || p.out == null) continue
    out.push({ amp: p.amp, in: p.in, out: p.out, db: 20 * Math.log10(p.gain), thd: p.thd, thdIn: p.thdIn, harm: p.harm ?? [] })
  }
  return out
}

// logLerp interpolates between a and b on a log scale.
const logLerp = (a: number, b: number, t: number) => Math.exp(Math.log(a) + t * (Math.log(b) - Math.log(a)))

export interface Level {
  amp: number // generator Vpp
  out: number // CH2 RMS
}

// onset finds where the distortion first rises through limit, interpolated
// on log scales. It returns null when the first point is already above.
export function onset(b: ThdPoint[], limit: number): Level | null {
  for (let i = 1; i < b.length; i++) {
    const p = b[i - 1], q = b[i]
    if (p.thd >= limit) return null
    if (q.thd < limit) continue
    const t = Math.log(limit / p.thd) / Math.log(q.thd / p.thd)
    return { amp: logLerp(p.amp, q.amp, t), out: logLerp(p.out, q.out, t) }
  }
  return null
}

// compression finds where the gain falls drop dB below its level at small
// amplitudes, the median of the first three points.
export function compression(b: ThdPoint[], drop = 1): (Level & { ref: number }) | null {
  if (b.length < 4) return null
  const ref = median(b.slice(0, 3).map((p) => p.db))
  const level = ref - drop
  for (let i = 1; i < b.length; i++) {
    const p = b[i - 1], q = b[i]
    if (p.db < level || q.db >= level) continue
    const t = (p.db - level) / (p.db - q.db)
    return { amp: logLerp(p.amp, q.amp, t), out: logLerp(p.out, q.out, t), ref }
  }
  return null
}

export function median(v: number[]): number {
  const s = [...v].sort((a, b) => a - b)
  const m = s.length >> 1
  return s.length % 2 ? s[m] : (s[m - 1] + s[m]) / 2
}

// pct prints a fraction as per cent with two significant digits at most.
export function pct(v: number): string {
  const x = v * 100
  return `${x >= 10 ? x.toFixed(0) : +x.toPrecision(2)} %`
}
