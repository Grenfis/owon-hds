// Run with: npm test
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { analyze } from '../src/curve.ts'

const near = (got: number | undefined, want: number, tol: number, what: string) =>
  assert.ok(got != null && Math.abs(got / want - 1) < tol, `${what}: ${got}, want ${want}`)

// The screen: 600 samples of a 100 Hz sine, ±2.5 V from the generator, at
// 1 V/div (40 mV a count) on both channels and 5 ms/div.
const N = 600, F = 100, DT = (5e-3 * 12) / N, R = 1e3, Q = 0.04
const gen = Array.from({ length: N }, (_, k) => 2.5 * Math.sin(2 * Math.PI * F * DT * k + 0.3))
const count = (x: number) => Math.round(x / Q) * Q

// part returns the screen for a part given as the voltage across it for a
// generator voltage, quantised to counts.
function screen(across: (vg: number, k: number) => number) {
  return { ch1: gen.map(count), ch2: gen.map((vg, k) => count(across(vg, k))) }
}

// linear returns the voltage across a linear part of impedance z = [re, im]
// in the divider with R, at the generator's phase.
function linear(z: [number, number]) {
  const w = 2 * Math.PI * F
  const [a, b] = z
  // V = Vg · z / (z + R), as a complex gain applied to the sine.
  const m = (a + R) ** 2 + b * b
  const gr = (a * (a + R) + b * b) / m, gi = (b * (a + R) - a * b) / m
  return (_: number, k: number) => 2.5 * Math.hypot(gr, gi) * Math.sin(w * DT * k + 0.3 + Math.atan2(gi, gr))
}

// diode solves Vg = I·R + V for I = Is·(e^(V/nVt) − 1), with the 1N4148
// model's Is and n, and a breakdown that passes 1 mA at −vz when given.
const IS = 2.52e-9, NVT = 1.752 * 0.02585, IZ = 1e-3, VZT = 0.02
const fwdAt = (i: number) => NVT * Math.log(i / IS + 1)
const revAt = (i: number, vz: number) => -(vz + VZT * Math.log(i / IZ))
function diode(up: boolean, vz = 0) {
  const cur = (v: number) => IS * (Math.exp(v / NVT) - 1) - (vz ? IZ * Math.exp((-v - vz) / VZT) : 0)
  return (vg: number) => {
    const s = up ? 1 : -1
    let lo = -3, hi = 3
    for (let k = 0; k < 80; k++) {
      const v = (lo + hi) / 2
      if (cur(v) * R + v > s * vg) hi = v
      else lo = v
    }
    return s * (lo + hi) / 2
  }
}

const run = (s: { ch1: number[]; ch2: number[] }) => analyze(s.ch1, s.ch2, R, F, DT, Q, Q)

test('open and short', () => {
  assert.equal(run(screen((vg) => vg)).kind, 'open')
  assert.equal(run(screen(() => 0)).kind, 'short')
})

test('resistor', () => {
  const r = run(screen(linear([470, 0])))
  assert.equal(r.kind, 'resistor')
  near(r.r, 470, 0.03, 'R')
})

test('capacitor', () => {
  const c = 1e-6
  const r = run(screen(linear([0, -1 / (2 * Math.PI * F * c)])))
  assert.equal(r.kind, 'capacitor')
  near(r.c, c, 0.04, 'C')
  assert.equal(r.r, undefined)
})

test('inductor with series resistance', () => {
  const l = 1, rs = 100
  const r = run(screen(linear([rs, 2 * Math.PI * F * l])))
  assert.equal(r.kind, 'inductor')
  near(r.l, l, 0.04, 'L')
  near(r.r, rs, 0.3, 'Rs')
})

test('diode either way round', () => {
  const up = run(screen(diode(true)))
  assert.equal(up.kind, 'diode')
  assert.equal(up.anodeUp, true)
  // The knee is where the current passes the threshold, within two counts.
  assert.ok(Math.abs(up.fwd! - fwdAt(up.ith)) < 2 * Q, `fwd ${up.fwd}, want ${fwdAt(up.ith)}`)
  const down = run(screen(diode(false)))
  assert.equal(down.kind, 'diode')
  assert.equal(down.anodeUp, false)
  near(down.fwd, up.fwd!, 0.1, 'fwd')
})

test('zener and two diodes back to back', () => {
  const z = run(screen(diode(true, 1.8)))
  assert.equal(z.kind, 'zener')
  assert.equal(z.anodeUp, true)
  assert.ok(Math.abs(z.fwd! - fwdAt(z.ith)) < 2 * Q, `fwd ${z.fwd}, want ${fwdAt(z.ith)}`)
  assert.ok(Math.abs(z.rev! - revAt(z.ith, 1.8)) < 2 * Q, `rev ${z.rev}, want ${revAt(z.ith, 1.8)}`)
  // The same zener the other way round.
  const zd = run(screen(diode(false, 1.8)))
  assert.equal(zd.kind, 'zener')
  assert.equal(zd.anodeUp, false)
  near(zd.rev, z.rev!, 0.05, 'rev')
  const d = diode(true)
  const t = run(screen((vg) => (vg >= 0 ? d(vg) : -d(-vg))))
  assert.equal(t.kind, 'twoway')
})
