// Component tester: the generator drives the part through a reference
// resistor, CH1 sees the generator and CH2 the part. The voltage across the
// part is CH2 and the current through it (CH1 − CH2) / R; plotted against
// each other they make the current–voltage curve of the part.

export interface Curve {
  v: number[] // across the part, volts
  i: number[] // through it, amperes; positive into the CH2 point
}

export function curve(ch1: number[], ch2: number[], r: number): Curve {
  const n = Math.min(ch1.length, ch2.length)
  const v = ch2.slice(0, n)
  return { v, i: v.map((x, k) => (ch1[k] - x) / r) }
}

export type Kind = 'open' | 'short' | 'resistor' | 'capacitor' | 'inductor' | 'diode' | 'zener' | 'twoway' | 'other'

export interface CurveReading {
  kind: Kind
  r?: number // resistor; the leak of a capacitor; the series resistance of an inductor
  c?: number
  l?: number
  // Diode-like parts: the voltage at which the current passes the threshold,
  // in the direction it conducts first (fwd) and in the other one (rev, a
  // negative voltage when the anode is at the CH2 point).
  fwd?: number
  rev?: number
  anodeUp?: boolean // the anode, or the forward side, is at the CH2 point
  ith: number // the current that counts as conduction
  vth: number // the voltage under which the part counts as a short
  iMax: number // the largest current, either way
  vMax: number // the largest voltage, either way
}

function percentile(v: number[], p: number): number {
  const s = [...v].sort((a, b) => a - b)
  return s[Math.min(s.length - 1, Math.max(0, Math.round(p * (s.length - 1))))]
}

// analyze names the part. q1 and q2 are one ADC count of each channel in
// volts; f is the generator frequency and dt the sample time.
export function analyze(ch1: number[], ch2: number[], r: number, f: number, dt: number, q1: number, q2: number): CurveReading {
  const { v, i } = curve(ch1, ch2, r)
  const n = v.length
  const abs = (x: number[]) => x.map(Math.abs)
  const vg = percentile(abs(ch1), 0.98)
  const iMax = percentile(abs(i), 0.98), vMax = percentile(abs(v), 0.98)
  // Conduction: a current clearly above the noise of the two channels, and
  // above 2 % of what the generator drives into a short.
  const ith = Math.max((1.5 * Math.hypot(q1, q2)) / r, (0.02 * vg) / r)
  const vth = Math.max(1.5 * q2, 0.02 * vg)
  const out: CurveReading = { kind: 'other', ith, vth, iMax, vMax }
  if (iMax < ith) return { ...out, kind: 'open' }
  if (vMax < vth) return { ...out, kind: 'short' }

  // A linear part draws I = g·V + k·Vq, with Vq the fundamental of V a
  // quarter period ahead: g is its conductance and k its susceptance.
  const w = 2 * Math.PI * f * dt // radians per sample
  const m = f > 0 && dt > 0 ? Math.min(n, Math.round(Math.floor(n * dt * f) / (f * dt))) : 0
  let vq: number[] | null = null
  if (m >= 2 && Math.floor(n * dt * f) >= 1) {
    let re = 0, im = 0, mean = 0
    for (let k = 0; k < m; k++) mean += v[k] / m
    for (let k = 0; k < m; k++) {
      re += ((v[k] - mean) * Math.cos(w * k) * 2) / m
      im -= ((v[k] - mean) * Math.sin(w * k) * 2) / m
    }
    // V ≈ re·cos − im·sin; a quarter period ahead: −re·sin − im·cos.
    vq = v.map((_, k) => -re * Math.sin(w * k) - im * Math.cos(w * k))
  }
  const len = vq ? m : n
  let svv = 0, svq = 0, sqq = 0, svi = 0, sqi = 0, sii = 0
  for (let k = 0; k < len; k++) {
    const q = vq ? vq[k] : 0
    svv += v[k] * v[k]
    svq += v[k] * q
    sqq += q * q
    svi += v[k] * i[k]
    sqi += q * i[k]
    sii += i[k] * i[k]
  }
  const det = svv * sqq - svq * svq
  const [g, b] = vq && det > 0 ? [(svi * sqq - sqi * svq) / det, (sqi * svv - svi * svq) / det] : [svi / svv, 0]
  let res = 0
  for (let k = 0; k < len; k++) res += (i[k] - g * v[k] - b * (vq ? vq[k] : 0)) ** 2
  // Linear within the noise: what is left of the current is under a tenth
  // of it, or under two counts of the current.
  const left = Math.sqrt(res / len)
  if (left < Math.max(0.1 * Math.sqrt(sii / len), Math.hypot(q1, q2) / r)) {
    const omega = 2 * Math.PI * f
    if (Math.abs(b) < 0.2 * Math.abs(g)) return { ...out, kind: 'resistor', r: 1 / g }
    if (b > 0) return { ...out, kind: 'capacitor', c: b / omega, r: g > 0.05 * b ? 1 / g : undefined }
    // An inductor reads better as a series circuit: Z = 1 / (g + jb).
    const z2 = g * g + b * b
    return { ...out, kind: 'inductor', l: b / z2 / -omega, r: g / z2 }
  }

  // Not linear: where does it start to conduct either way?
  const pos = v.filter((_, k) => i[k] > ith), neg = v.filter((_, k) => i[k] < -ith)
  const knee = (side: number[], sign: number) => (side.length >= 3 ? percentile(side.map((x) => sign * x), 0.05) * sign : null)
  const kp = knee(pos, 1), kn = knee(neg, -1)
  if (kp != null && kn == null) return { ...out, kind: 'diode', fwd: kp, anodeUp: true }
  if (kn != null && kp == null) return { ...out, kind: 'diode', fwd: -kn, anodeUp: false }
  if (kp != null && kn != null) {
    const [a, c] = [Math.abs(kp), Math.abs(kn)]
    if (Math.max(a, c) / Math.min(a, c) < 1.25) return { ...out, kind: 'twoway', fwd: kp, rev: kn, anodeUp: true }
    // A zener: forward at the lower voltage, breakdown at the higher.
    return a < c ? { ...out, kind: 'zener', fwd: kp, rev: kn, anodeUp: true } : { ...out, kind: 'zener', fwd: -kn, rev: -kp, anodeUp: false }
  }
  return out
}
