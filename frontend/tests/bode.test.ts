// Run with: npm test
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { bodePoints, cutoffs, maximum } from '../src/bode.ts'
import type { SweepPoint } from '../src/types.ts'

// sweep samples a response at `perDecade` points from f1 to f2, with the
// phase wrapped into ±180° as the device side reports it.
function sweep(f1: number, f2: number, perDecade: number, h: (f: number) => [number, number]): SweepPoint[] {
  const n = Math.round(Math.log10(f2 / f1) * perDecade) + 1
  return Array.from({ length: n }, (_, i) => {
    const freq = f1 * (f2 / f1) ** (i / (n - 1))
    const [re, im] = h(freq)
    return { freq, gain: Math.hypot(re, im), phase: (Math.atan2(im, re) * 180) / Math.PI, in: null, out: null, note: '' }
  })
}

// div divides complex numbers given as [re, im].
const div = ([a, b]: number[], [c, d]: number[]): [number, number] => {
  const m = c * c + d * d
  return [(a * c + b * d) / m, (b * c - a * d) / m]
}

test('RC low pass: cutoff and its phase', () => {
  const b = bodePoints(sweep(100, 1e6, 10, (f) => div([1, 0], [1, f / 10e3])))
  const c = cutoffs(b)!
  assert.equal(c.list.length, 1)
  assert.ok(Math.abs(c.list[0].freq / 10e3 - 1) < 0.02, `cutoff ${c.list[0].freq}`)
  assert.ok(Math.abs(c.list[0].phase + 45) < 2, `phase ${c.list[0].phase}`)
  assert.equal(c.list[0].rising, false)
  assert.equal(maximum(b), null)
})

test('third order: the phase goes on below −180°', () => {
  const b = bodePoints(sweep(100, 1e6, 10, (f) => {
    let h: [number, number] = [1, 0]
    for (let i = 0; i < 3; i++) h = div(h, [1, f / 10e3])
    return h
  }))
  const last = b[b.length - 1].phase
  assert.ok(last < -260 && last > -270, `phase at 1 MHz ${last}`)
  for (let i = 1; i < b.length; i++) assert.ok(b[i].phase <= b[i - 1].phase + 1e-9)
})

test('band pass: both cutoffs and the centre', () => {
  // A series RLC at 1.59 kHz with Q = 2: the cutoffs lie at f0·(√(1+1/4Q²) ∓ 1/2Q).
  const f0 = 1590, Q = 2
  const b = bodePoints(sweep(100, 100e3, 10, (f) => div([1, 0], [1, Q * (f / f0 - f0 / f)])))
  const m = maximum(b)!
  assert.ok(Math.abs(m.freq / f0 - 1) < 0.05, `centre ${m.freq}`)
  assert.ok(Math.abs(m.db) < 0.3, `peak ${m.db}`)
  const c = cutoffs(b)!
  const r = Math.sqrt(1 + 1 / (4 * Q * Q))
  assert.deepEqual(c.list.map((x) => x.rising), [true, false])
  assert.ok(Math.abs(c.list[0].freq / (f0 * (r - 1 / (2 * Q))) - 1) < 0.03, `lower ${c.list[0].freq}`)
  assert.ok(Math.abs(c.list[1].freq / (f0 * (r + 1 / (2 * Q))) - 1) < 0.03, `upper ${c.list[1].freq}`)
})

test('points without a result are skipped', () => {
  const ps = sweep(100, 1e4, 1, () => [1, 0])
  ps[1] = { ...ps[1], gain: null, phase: null, note: 'нет сигнала' }
  assert.equal(bodePoints(ps).length, 2)
})
