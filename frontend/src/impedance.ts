// Impedance from a reference resistor and the ratio of the two channels.
// CH1 sees the generator, CH2 the node between the part and the resistor:
// with the part to ground ("top": generator → R → part → ground) CH2 is
// across the part, with the resistor to ground ("bottom": generator → part
// → R → ground) across the resistor. h is CH2/CH1 at the fundamental.

export type C = [number, number] // re, im

export const cmul = (a: C, b: C): C => [a[0] * b[0] - a[1] * b[1], a[0] * b[1] + a[1] * b[0]]
export const cdiv = (a: C, b: C): C => {
  const m = b[0] * b[0] + b[1] * b[1]
  return [(a[0] * b[0] + a[1] * b[1]) / m, (a[1] * b[0] - a[0] * b[1]) / m]
}
export const polar = (mag: number, deg: number): C => [mag * Math.cos((deg * Math.PI) / 180), mag * Math.sin((deg * Math.PI) / 180)]

export type Scheme = 'top' | 'bottom'

// The scope input across CH2, in parallel with the part or the resistor.
export interface Load {
  r: number // ohms
  c: number // farads
}

// inputLoad is the input of the device as its manual gives it, 1 MΩ with
// 16 pF, seen through a probe. Only the 1X input capacitance is known; a
// divider probe has its own, so it is left out then.
export function inputLoad(probe: number): Load {
  return probe === 1 ? { r: 1e6, c: 16e-12 } : { r: 1e6 * probe, c: 0 }
}

export function impedance(h: C, rRef: number, f: number, scheme: Scheme, load: Load | null): C {
  const w = 2 * Math.PI * f
  const yIn: C = load ? [1 / load.r, w * load.c] : [0, 0]
  const one: C = [1, 0]
  if (scheme === 'top') {
    // CH2 across the part: the measured impedance is the part with the input.
    const zm = cdiv([rRef * h[0], rRef * h[1]], [1 - h[0], -h[1]])
    const y = cdiv(one, zm)
    return cdiv(one, [y[0] - yIn[0], y[1] - yIn[1]])
  }
  // CH2 across the resistor, which the input shunts.
  const r = cdiv(one, [1 / rRef + yIn[0], yIn[1]])
  return cmul(r, cdiv([1 - h[0], -h[1]], h))
}

export interface Reading {
  kind: 'R' | 'C' | 'L'
  mag: number // |Z|
  phase: number // degrees
  series: { r: number; x: number; value: number } // value: farads or henries, or ohms for R
  parallel: { r: number; x: number; value: number }
  d: number // dissipation factor R/|X| of the series model; Q = 1/d
}

// read names the part from its impedance: a resistor when the reactance is
// under a tenth of the resistance.
export function read(z: C, f: number): Reading {
  const w = 2 * Math.PI * f
  const [rs, xs] = z
  const y = cdiv([1, 0], z)
  const rp = 1 / y[0], xp = -1 / y[1] // the parallel reactance, same sign as xs
  const kind = Math.abs(xs) < 0.1 * Math.abs(rs) ? 'R' : xs < 0 ? 'C' : 'L'
  const value = (r: number, x: number) => (kind === 'R' ? r : kind === 'C' ? -1 / (w * x) : x / w)
  return {
    kind,
    mag: Math.hypot(rs, xs),
    phase: (Math.atan2(xs, rs) * 180) / Math.PI,
    series: { r: rs, x: xs, value: value(rs, xs) },
    parallel: { r: rp, x: xp, value: value(rp, xp) },
    d: Math.abs(rs / xs),
  }
}

// bestFreq is the frequency at which a capacitor or an inductor has the
// impedance of the reference, where the ratio of the channels is the most
// sensitive to it.
export function bestFreq(r: Reading, rRef: number): number | null {
  if (r.kind === 'C') return 1 / (2 * Math.PI * rRef * r.series.value)
  if (r.kind === 'L') return rRef / (2 * Math.PI * r.series.value)
  return null
}
