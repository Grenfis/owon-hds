// The multimeter log: readings with their times, the limits they are held
// to, the chart columns and the CSV file.

export const MAX_READINGS = 200_000

export type Side = 'lo' | 'hi' | null

// side tells whether a reading is below the low limit or above the high
// one; a limit left empty is not checked.
export function side(v: number, lo: number | null, hi: number | null): Side {
  if (lo != null && v < lo) return 'lo'
  if (hi != null && v > hi) return 'hi'
  return null
}

// firstAt is the index of the first time not before t, in times that grow.
export function firstAt(ts: number[], t: number): number {
  let a = 0, b = ts.length
  while (a < b) {
    const m = (a + b) >> 1
    if (ts[m] < t) a = m + 1
    else b = m
  }
  return a
}

// envelope reduces the readings between t0 and t1 to columns of the chart:
// the least and the greatest value of each, NaN where there is none. A long
// log then draws as fast as a short one and keeps its spikes.
export function envelope(ts: number[], vs: number[], t0: number, t1: number, cols: number) {
  const lo = new Float64Array(cols).fill(NaN), hi = new Float64Array(cols).fill(NaN)
  const k = cols / (t1 - t0 || 1)
  for (let i = firstAt(ts, t0); i < ts.length && ts[i] <= t1; i++) {
    const c = Math.min(cols - 1, Math.floor((ts[i] - t0) * k))
    const v = vs[i]
    if (!(lo[c] <= v)) lo[c] = v
    if (!(hi[c] >= v)) hi[c] = v
  }
  return { lo, hi }
}

const TIME_STEPS = [1, 2, 5, 10, 15, 30, 60, 120, 300, 600, 900, 1800, 3600, 7200, 10800, 21600, 43200, 86400]

// timeStep is a step of the time axis, in seconds, that cuts span into about
// n parts and falls on whole minutes and hours.
export function timeStep(span: number, n: number): number {
  return TIME_STEPS.find((s) => s >= span / n) ?? 86400
}

// elapsed writes seconds since the start of the log: 0:05, 12:30, 1:02:00.
export function elapsed(s: number): string {
  const t = Math.round(s)
  const h = Math.floor(t / 3600), m = Math.floor((t % 3600) / 60), sec = t % 60
  const ss = String(sec).padStart(2, '0')
  return h ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`
}

// niceStep returns a 1-2-5 step that cuts span into about n parts.
export function niceStep(span: number, n: number): number {
  const raw = span / n, e = 10 ** Math.floor(Math.log10(raw))
  return [1, 2, 5, 10].map((m) => m * e).find((s) => s >= raw) ?? 10 * e
}

const ASCII_UNITS: Record<string, string> = { В: 'V', А: 'A', Ом: 'Ohm', Ф: 'F' }

// logCSV writes the local time of each reading, the seconds from the first
// one and the value as the device sent it.
export function logCSV(ts: number[], vs: number[], unit: string, mode: string): string {
  const pad = (n: number, w = 2) => String(n).padStart(w, '0')
  const clock = (t: number) => {
    const d = new Date(t)
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${pad(d.getMilliseconds(), 3)}`
  }
  const lines = [`# OWON HDS272S, мультиметр: ${mode}; local time`, `time,t_s,value_${ASCII_UNITS[unit] ?? unit}`]
  for (let i = 0; i < ts.length; i++) lines.push(`${clock(ts[i])},${((ts[i] - ts[0]) / 1000).toFixed(3)},${vs[i]}`)
  return lines.join('\n') + '\n'
}
