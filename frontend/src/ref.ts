// Reference waveform: a saved frame or a CSV file, drawn over the live
// signal. Its samples are kept in volts and seconds from the trigger point,
// so it is drawn at the scales the live traces have now and stays comparable
// with them when the scales change.
import { MATH_OPS, channelVolts, mathCsvName, type MathOp } from './math.ts'
import type { Frame } from './types'

export type RefName = 'CH1' | 'CH2' | 'M'

export interface RefTrace {
  name: RefName
  v: number[]
}

export interface Reference {
  t: number[] // seconds from the trigger point, rising
  traces: RefTrace[]
  math: MathOp // the function of the M trace, for its unit
  source: string // the time of the frame or the file name
  timeDiv: number // of the frame; 0 for a file
}

// A longer file is thinned to this many points: enough for any screen, and
// small enough to keep between runs.
export const MAX_POINTS = 20000

// refFromFrame takes the channels that are on and the math trace.
export function refFromFrame(f: Frame, math: number[] | null, op: MathOp, source: string): Reference | null {
  const traces: RefTrace[] = []
  for (const c of f.channels) {
    if (c.on && c.raw && c.raw.length > 1 && (c.name === 'CH1' || c.name === 'CH2')) traces.push({ name: c.name, v: channelVolts(c) })
  }
  if (traces.length && math && op !== 'off' && math.length === traces[0].v.length) traces.push({ name: 'M', v: math.slice() })
  if (!traces.length) return null
  const n = traces[0].v.length
  // As in the CSV export: a positive offset puts the trigger point hOffset
  // divisions left of the centre.
  const t = Array.from({ length: n }, (_, i) => ((i / (n - 1)) * 12 - 6 + f.hOffset) * f.timeDiv)
  return { t, traces, math: op, source, timeDiv: f.timeDiv }
}

// refFromCSV reads a table with the time in seconds in the first column and
// volts in the others: the CSV of this program, or one from a spreadsheet
// with ";" and a decimal comma. A column named CH1…, CH2… or M… goes to that
// channel, the others take CH1 and CH2 in turn. It returns what is wrong
// with the file instead of a reference.
export function refFromCSV(text: string, source: string): Reference | string {
  const lines = text.split(/\r?\n/).map((l) => l.trim()).filter((l) => l && !l.startsWith('#'))
  if (!lines.length) return 'Файл пуст'
  const sep = lines[0].includes(';') ? ';' : lines[0].includes('\t') ? '\t' : ','
  const split = (l: string) => l.split(sep).map((s) => s.trim().replace(/^"(.*)"$/, '$1'))
  const num = (s: string) => (s === '' ? NaN : Number(sep === ',' ? s : s.replace(',', '.')))

  let head: string[] = []
  let body = lines
  if (!Number.isFinite(num(split(lines[0])[0]))) [head, body] = [split(lines[0]), lines.slice(1)]
  const width = Math.max(head.length, split(body[0] ?? '').length)
  if (width < 2) return 'Нужно хотя бы два столбца: время и напряжение'

  // Which trace each column after the time goes to.
  const named = (h: string): RefName | null => {
    const u = h.toUpperCase()
    return u.startsWith('CH1') ? 'CH1' : u.startsWith('CH2') ? 'CH2' : u.startsWith('M') ? 'M' : null
  }
  const cols: (RefName | null)[] = Array.from({ length: width }, (_, j) => (j ? named(head[j] ?? '') : null))
  const free = (['CH1', 'CH2'] as RefName[]).filter((n) => !cols.includes(n))
  const used = new Set<RefName>()
  for (let j = 1; j < width; j++) {
    const n = cols[j] ?? free.shift() ?? null
    cols[j] = n && !used.has(n) ? n : null
    if (n) used.add(n)
  }
  const keep = cols.flatMap((n, j) => (n ? [j] : []))

  const t: number[] = []
  const vs: number[][] = keep.map(() => [])
  for (const l of body) {
    const cells = split(l).map(num)
    if (!Number.isFinite(cells[0]) || keep.some((j) => !Number.isFinite(cells[j]))) continue
    if (t.length && cells[0] <= t[t.length - 1]) return `Время в первом столбце должно расти: строка «${l}»`
    t.push(cells[0])
    keep.forEach((j, k) => vs[k].push(cells[j]))
  }
  if (t.length < 2) return 'В файле нет строк с числами'

  const step = Math.ceil(t.length / MAX_POINTS)
  const thin = (a: number[]) => (step > 1 ? a.filter((_, i) => i % step === 0) : a)
  const mHead = head[cols.indexOf('M')] ?? ''
  const math = MATH_OPS.find((o) => o.value !== 'off' && mHead === `M_${mathCsvName(o.value)}`)?.value ?? 'sub'
  return {
    t: thin(t),
    traces: keep.map((j, k) => ({ name: cols[j]!, v: thin(vs[k]) })),
    math,
    source,
    timeDiv: 0,
  }
}

// refAt is the value of a trace at time t, linear between the samples, or
// null outside the reference.
export function refAt(r: Reference, v: number[], t: number): number | null {
  const ts = r.t
  if (!(t >= ts[0] && t <= ts[ts.length - 1])) return null
  let lo = 0, hi = ts.length - 1
  while (hi - lo > 1) {
    const mid = (lo + hi) >> 1
    if (ts[mid] <= t) lo = mid
    else hi = mid
  }
  return hi === lo ? v[lo] : v[lo] + ((v[hi] - v[lo]) * (t - ts[lo])) / (ts[hi] - ts[lo])
}

// isReference checks what was kept between runs.
export function isReference(x: unknown): x is Reference {
  const r = x as Reference
  return !!r && Array.isArray(r.t) && r.t.length > 1 && Array.isArray(r.traces) && r.traces.length > 0 &&
    r.traces.every((c) => ['CH1', 'CH2', 'M'].includes(c.name) && Array.isArray(c.v) && c.v.length === r.t.length)
}
