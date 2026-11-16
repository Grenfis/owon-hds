// Math channel: a function of CH1 and CH2, sample by sample, from the 600
// samples of a frame. The device has no math commands, so it is computed
// here. It has its own scale and offset on the screen, as a channel does.
import type { Frame, FrameChannel } from './types'

export type MathOp = 'off' | 'add' | 'sub' | 'rsub' | 'mul'

export const MATH_OPS: { value: MathOp; label: string; title?: string }[] = [
  { value: 'off', label: 'Выкл' },
  { value: 'add', label: '1+2', title: 'CH1 + CH2' },
  { value: 'sub', label: '1−2', title: 'CH1 − CH2: например, напряжение между двумя точками схемы, ни одна из которых не земля' },
  { value: 'rsub', label: '2−1', title: 'CH2 − CH1' },
  { value: 'mul', label: '1×2', title: 'CH1 × CH2: например, мгновенная мощность, если CH2 показывает ток' },
]

// The product is in В·В rather than В²: with a prefix, "мВ²" would read as
// (мВ)², a thousand times less than meant.
export const mathUnit = (op: MathOp) => (op === 'mul' ? 'В·В' : 'В')

export const mathName = (op: MathOp) => ({ off: '', add: 'CH1+CH2', sub: 'CH1−CH2', rsub: 'CH2−CH1', mul: 'CH1×CH2' })[op]
export const mathCsvName = (op: MathOp) => ({ off: '', add: 'CH1+CH2_V', sub: 'CH1-CH2_V', rsub: 'CH2-CH1_V', mul: 'CH1*CH2_V2' })[op]

// The scales of the math channel, 1-2-5 from 1 m to 1 k per division.
export const MATH_SCALES: number[] = (() => {
  const out: number[] = []
  for (let e = -3; e <= 3; e++) for (const m of [1, 2, 5]) out.push(+(m * 10 ** e).toPrecision(6))
  return out.filter((v) => v <= 1000)
})()

export function channelVolts(c: FrameChannel): number[] {
  return (c.raw ?? []).map((r) => ((r - c.offset) / 25) * c.voltsPerDiv)
}

export interface MathTrace {
  v: number[]
  clipped: boolean // a source channel is beyond the screen, so is the result
}

// mathOf computes the math channel of a frame; it needs both channels on.
export function mathOf(f: Frame | null, op: MathOp): MathTrace | null {
  if (!f || op === 'off') return null
  const c1 = f.channels.find((c) => c.name === 'CH1'), c2 = f.channels.find((c) => c.name === 'CH2')
  if (!c1?.on || !c2?.on || !c1.raw?.length || c1.raw.length !== c2.raw?.length) return null
  const a = channelVolts(c1), b = channelVolts(c2)
  const fn = { add: (x: number, y: number) => x + y, sub: (x: number, y: number) => x - y, rsub: (x: number, y: number) => y - x, mul: (x: number, y: number) => x * y }[op]
  return { v: a.map((x, i) => fn(x, b[i])), clipped: c1.meas.clipped || c2.meas.clipped }
}

// fitMath picks the finest scale that keeps the trace within six divisions
// and the offset that centres it.
export function fitMath(v: number[]): { scale: number; offset: number } {
  const lo = Math.min(...v), hi = Math.max(...v)
  const s = MATH_SCALES.find((x) => (hi - lo) / x <= 6) ?? MATH_SCALES[MATH_SCALES.length - 1]
  const offset = Math.max(-4, Math.min(4, -(hi + lo) / 2 / s))
  return { scale: s, offset: Math.round(offset * 25) / 25 || 0 }
}

export interface MathStats {
  pp: number
  max: number
  min: number
  avg: number
  rms: number
}

// mathStats measures the trace over the whole screen.
export function mathStats(v: number[]): MathStats {
  let lo = Infinity, hi = -Infinity, sum = 0, sq = 0
  for (const x of v) {
    lo = Math.min(lo, x)
    hi = Math.max(hi, x)
    sum += x
    sq += x * x
  }
  return { pp: hi - lo, max: hi, min: lo, avg: sum / v.length, rms: Math.sqrt(sq / v.length) }
}
