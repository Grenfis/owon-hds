// Run with: npm test
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { compression, onset, pct, thdPoints } from '../src/thd.ts'
import type { SweepPoint } from '../src/types.ts'

const pt = (amp: number, db: number, thd: number, extra: Partial<SweepPoint> = {}): SweepPoint => ({
  freq: 1000, amp, gain: 10 ** (db / 20), phase: 0, in: amp / 2.83, out: (amp / 2.83) * 10 ** (db / 20),
  harm: [thd], thd, thdIn: 0.001, note: '', ...extra,
})

test('points without a result are left out', () => {
  const b = thdPoints([pt(0.1, 10, 0.002), pt(0.2, 10, 0.002, { thd: null, note: 'нет сигнала' }), pt(0.4, 10, 0.003)])
  assert.deepEqual(b.map((p) => p.amp), [0.1, 0.4])
  assert.ok(Math.abs(b[0].db - 10) < 1e-9)
})

test('onset on log scales', () => {
  const b = thdPoints([pt(0.5, 20, 0.002), pt(1, 20, 0.005), pt(2, 19, 0.02), pt(4, 15, 0.2)])
  const one = onset(b, 0.01)!
  // 1 % lies halfway between 0.5 % and 2 % on a log scale.
  assert.ok(Math.abs(one.amp - Math.SQRT2) < 1e-9, `amp ${one.amp}`)
  assert.ok(Math.abs(one.out / Math.sqrt(b[1].out * b[2].out) - 1) < 1e-9)
  const ten = onset(b, 0.1)!
  assert.ok(ten.amp > 2 && ten.amp < 4)
  assert.equal(onset(b, 0.5), null)
  // Above the limit from the first point.
  assert.equal(onset(thdPoints([pt(0.5, 20, 0.02), pt(1, 20, 0.05)]), 0.01), null)
})

test('compression against the gain at small amplitudes', () => {
  const b = thdPoints([pt(0.1, 10.1, 0.002), pt(0.2, 9.9, 0.002), pt(0.4, 10, 0.002), pt(0.8, 9.5, 0.004), pt(1.6, 8.5, 0.03), pt(3.2, 6, 0.2)])
  const c = compression(b)!
  assert.equal(c.ref, 10)
  // −1 dB lies halfway between 9.5 and 8.5 dB.
  assert.ok(Math.abs(c.amp - Math.sqrt(0.8 * 1.6)) < 1e-9, `amp ${c.amp}`)
  assert.equal(compression(b.slice(0, 4)), null)
})

test('per cent', () => {
  assert.equal(pct(0.01), '1 %')
  assert.equal(pct(0.00123), '0.12 %')
  assert.equal(pct(0.256), '26 %')
})
