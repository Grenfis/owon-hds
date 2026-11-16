// Step response: CH1 sees a square wave at the input of a circuit and CH2
// its output. Each edge of CH1 starts a step. The output is scaled to run
// from 0, its level before the edge, to 1, its level at the end of the half
// period; times count from the 50 % crossing of the input.

export interface Step {
  pre: number // samples before the first one past the input edge
  x: number[] // input, scaled like y
  y: number[] // output
  gain: number // output step over input step
  swing: number // output step, volts
  drift: number // change of y over the last fifth: the output still moves
}

const PRE_MIN = 4
const POST_MIN = 20

function mean(v: ArrayLike<number>, a: number, b: number): number {
  let s = 0
  for (let i = a; i < b; i++) s += v[i]
  return s / (b - a)
}

// steps cuts the screen into steps at the rising or the falling edges of
// the input. A step runs to the next edge of the input; the level before
// it is taken from the end of the previous half period. A step cut short by
// the screen end may not have settled, so it counts only when the screen
// holds no whole one.
export function steps(input: number[], output: number[], rising: boolean): Step[] {
  const n = Math.min(input.length, output.length)
  if (n < POST_MIN) return []
  const sorted = input.slice(0, n).sort((a, b) => a - b)
  const lo = sorted[Math.floor(n * 0.05)], hi = sorted[Math.floor(n * 0.95)]
  if (!(hi > lo)) return []
  const mid = (lo + hi) / 2, hyst = (hi - lo) * 0.1

  // Edges: the first sample past the middle, found with hysteresis. The
  // first transition counts only when the level before it is known.
  const edges: { i: number; up: boolean }[] = []
  let high: boolean | null = input[0] > mid + hyst ? true : input[0] < mid - hyst ? false : null
  for (let i = 1; i < n; i++) {
    const v = input[i]
    if (high !== true && v > mid + hyst) {
      if (high === false) {
        let j = i
        while (j > 0 && input[j - 1] > mid) j--
        edges.push({ i: j, up: true })
      }
      high = true
    } else if (high !== false && v < mid - hyst) {
      if (high === true) {
        let j = i
        while (j > 0 && input[j - 1] < mid) j--
        edges.push({ i: j, up: false })
      }
      high = false
    }
  }

  const out: Step[] = [], cut: Step[] = []
  edges.forEach((e, k) => {
    if (e.up !== rising) return
    const start = e.i
    const end = k + 1 < edges.length ? edges[k + 1].i : n
    const prev = k > 0 ? edges[k - 1].i : 0
    const len = end - start
    const pre = Math.min(start - prev, Math.floor(len / 4))
    if (pre < PRE_MIN || len < POST_MIN) return
    // The level before: the samples just before the edge are left out, as
    // the edge of a slow generator starts there. The level after: the
    // last fifth of the step.
    const p0 = start - pre, p1 = start - Math.max(1, Math.floor(pre / 4))
    const f0 = end - Math.floor(len / 5)
    const inPre = mean(input, p0, p1), inStep = mean(input, f0, end) - inPre
    const outPre = mean(output, p0, p1), outStep = mean(output, f0, end) - outPre
    if (inStep === 0 || outStep === 0) return
    const x: number[] = [], y: number[] = []
    for (let i = p0; i < end; i++) {
      x.push((input[i] - inPre) / inStep)
      y.push((output[i] - outPre) / outStep)
    }
    const h = Math.max(1, Math.floor((end - f0) / 2))
    const drift = (mean(output, end - h, end) - mean(output, f0, f0 + h)) / outStep
    ;(end < n ? out : cut).push({ pre, x, y, gain: outStep / inStep, swing: outStep, drift })
  })
  return out.length ? out : cut
}

// average aligns the steps at the input edge and keeps the span they share.
export function average(list: Step[]): Step | null {
  if (!list.length) return null
  const pre = Math.min(...list.map((s) => s.pre))
  const post = Math.min(...list.map((s) => s.y.length - s.pre))
  const x = new Array<number>(pre + post).fill(0), y = new Array<number>(pre + post).fill(0)
  for (const s of list) {
    for (let k = 0; k < pre + post; k++) {
      x[k] += s.x[s.pre - pre + k]
      y[k] += s.y[s.pre - pre + k]
    }
  }
  const m = list.length
  const avg = (f: (s: Step) => number) => list.reduce((a, s) => a + f(s), 0) / m
  return {
    pre, x: x.map((v) => v / m), y: y.map((v) => v / m),
    gain: avg((s) => s.gain), swing: avg((s) => s.swing), drift: avg((s) => s.drift),
  }
}

export interface StepMetrics {
  t0: number // input 50 % crossing, in samples from the curve start
  delay: number | null // output 50 % after input 50 %, seconds
  rise: number | null // output 10–90 %
  inRise: number | null // input 10–90 %
  overshoot: number // peak above 1, as a fraction of the step
  noise: number // standard deviation of the output before the edge, same scale
  peak: number | null // time of the peak after the input edge
  settling: number | null // into the band for good; null when not within the curve
  ring: number | null // frequency of the ringing about the final level, Hz
}

// cross returns where v first passes level after from, going up, as a
// fractional index.
function cross(v: number[], from: number, level: number): number | null {
  for (let i = Math.max(1, from); i < v.length; i++) {
    if (v[i] >= level && v[i - 1] < level) return i - 1 + (level - v[i - 1]) / (v[i] - v[i - 1])
  }
  return null
}

// riseOf returns the 10 % and 90 % points around the 50 % point t50: the
// last 10 % crossing before it and the first 90 % one after it, so that
// noise far from the edge does not count.
function riseOf(v: number[], t50: number): [number | null, number | null] {
  let t10: number | null = null, t90: number | null = null
  for (let j = Math.floor(t50); j >= 0; j--) {
    if (v[j] < 0.1) {
      t10 = j + (0.1 - v[j]) / (v[j + 1] - v[j])
      break
    }
  }
  for (let i = Math.max(1, Math.ceil(t50)); i < v.length; i++) {
    if (v[i] >= 0.9) {
      t90 = i - 1 + (0.9 - v[i - 1]) / (v[i] - v[i - 1])
      break
    }
  }
  return [t10, t90]
}

export function metrics(s: Step, dt: number, band: number): StepMetrics {
  const y = s.y, n = y.length
  const t0 = cross(s.x, s.pre - 2, 0.5) ?? s.pre - 0.5
  const m: StepMetrics = { t0, delay: null, rise: null, inRise: null, overshoot: 0, noise: 0, peak: null, settling: null, ring: null }
  const quiet = y.slice(0, Math.max(2, s.pre - 1))
  const q = quiet.reduce((a, v) => a + v, 0) / quiet.length
  m.noise = Math.sqrt(quiet.reduce((a, v) => a + (v - q) ** 2, 0) / quiet.length)
  const [i10, i90] = riseOf(s.x, t0)
  if (i10 != null && i90 != null) m.inRise = (i90 - i10) * dt
  const t50 = cross(y, s.pre - 2, 0.5)
  if (t50 == null) return m
  m.delay = (t50 - t0) * dt
  const [t10, t90] = riseOf(y, t50)
  if (t10 != null && t90 != null) m.rise = (t90 - t10) * dt

  let k = Math.floor(t50)
  for (let i = k; i < n; i++) if (y[i] > y[k]) k = i
  m.overshoot = Math.max(0, y[k] - 1)
  m.peak = (k - t0) * dt

  // Settling: the last sample outside the band, unless it lies in the last
  // fifth, which sets the final level and so cannot show it.
  let last = -1
  for (let i = s.pre; i < n; i++) if (Math.abs(y[i] - 1) > band) last = i
  if (last < 0) m.settling = Math.max(0, (s.pre - t0) * dt)
  else if (last < n - 1 && last < s.pre + 0.8 * (n - s.pre)) {
    const a = Math.abs(y[last] - 1), b = Math.abs(y[last + 1] - 1)
    m.settling = (last + (a - band) / (a - b) - t0) * dt
  }

  // Ringing: crossings of the final level after the peak. A crossing
  // counts once the curve goes half the band beyond it, so that noise about
  // the final level does not; each one is half a period of the ringing.
  if (m.overshoot > band) {
    const times: number[] = []
    let above = true, last = -1
    for (let i = k + 1; i < n; i++) {
      if ((y[i - 1] - 1) * (y[i] - 1) <= 0 && y[i] !== y[i - 1]) last = i - 1 + (1 - y[i - 1]) / (y[i] - y[i - 1])
      if (above ? y[i] < 1 - band / 2 : y[i] > 1 + band / 2) {
        above = !above
        times.push(last)
      }
    }
    if (times.length >= 2) m.ring = (times.length - 1) / (2 * (times[times.length - 1] - times[0]) * dt)
  }
  return m
}
