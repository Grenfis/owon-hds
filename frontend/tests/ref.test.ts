// Run with: npm test
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { MAX_POINTS, isReference, refAt, refFromCSV, refFromFrame, type Reference } from '../src/ref.ts'
import type { Frame, FrameChannel } from '../src/types.ts'

function channel(name: string, raw: number[] | null, on = true): FrameChannel {
  return { name, on, coupling: 'DC', probe: '1X', voltsPerDiv: 0.5, offset: 25, raw, meas: { clipped: false } as FrameChannel['meas'] }
}

test('a frame in volts and seconds from the trigger point', () => {
  const raw = Array.from({ length: 601 }, (_, i) => 25 + (i % 2) * 50)
  const f = { timeDiv: 1e-3, hOffset: 2, channels: [channel('CH1', raw), channel('CH2', null, false)] } as unknown as Frame
  const r = refFromFrame(f, raw.map(() => 1), 'mul', 'кадр')!
  assert.deepEqual(r.traces.map((c) => c.name), ['CH1', 'M'])
  assert.deepEqual(r.traces[0].v.slice(0, 3), [0, 1, 0])
  // The left edge is 6 divisions before the centre, the trigger point 2 after it.
  assert.ok(Math.abs(r.t[0] + 4e-3) < 1e-12)
  assert.ok(Math.abs(r.t[600] - 8e-3) < 1e-12)
  assert.equal(r.math, 'mul')
  assert.equal(refFromFrame({ ...f, channels: [channel('CH1', null, false)] } as Frame, null, 'off', ''), null)
})

test('the CSV of this program', () => {
  const r = refFromCSV('# OWON HDS272S\nt_s,CH1_V,CH2_V,M_CH1*CH2_V2\n-1e-3,0.5,1,0.5\n0,1,2,2\n', 'a.csv') as Reference
  assert.deepEqual(r.t, [-1e-3, 0])
  assert.deepEqual(r.traces, [{ name: 'CH1', v: [0.5, 1] }, { name: 'CH2', v: [1, 2] }, { name: 'M', v: [0.5, 2] }])
  assert.equal(r.math, 'mul')
  assert.equal(r.source, 'a.csv')
})

test('a spreadsheet table with ";" and a decimal comma, columns by position', () => {
  const r = refFromCSV('время;U2;U1\r\n0,001;1,5;-2\r\n0,002;2,5;-3\r\n', 'b.csv') as Reference
  assert.deepEqual(r.t, [0.001, 0.002])
  assert.deepEqual(r.traces, [{ name: 'CH1', v: [1.5, 2.5] }, { name: 'CH2', v: [-2, -3] }])
})

test('without a header, a named column keeps its channel', () => {
  const r = refFromCSV('0,1\n1,2\n', '') as Reference
  assert.deepEqual(r.traces, [{ name: 'CH1', v: [1, 2] }])
  const s = refFromCSV('t,x,CH1\n0,1,2\n1,3,4\n', '') as Reference
  assert.deepEqual(s.traces, [{ name: 'CH2', v: [1, 3] }, { name: 'CH1', v: [2, 4] }])
})

test('what is wrong with a file', () => {
  assert.equal(typeof refFromCSV('', ''), 'string')
  assert.equal(typeof refFromCSV('t\n1\n2\n', ''), 'string')
  assert.match(refFromCSV('0,1\n0,2\n', '') as string, /должно расти/)
  assert.equal(typeof refFromCSV('t,CH1\nа,б\n', ''), 'string')
})

test('a long file is thinned', () => {
  const rows = Array.from({ length: 3 * MAX_POINTS }, (_, i) => `${i},${i % 7}`).join('\n')
  const r = refFromCSV(rows, '') as Reference
  assert.equal(r.t.length, MAX_POINTS)
  assert.equal(r.traces[0].v.length, MAX_POINTS)
})

test('a value between the samples', () => {
  const r = { t: [0, 1, 3], traces: [] } as unknown as Reference
  const v = [0, 10, 30]
  assert.equal(refAt(r, v, 0.5), 5)
  assert.equal(refAt(r, v, 2), 20)
  assert.equal(refAt(r, v, 3), 30)
  assert.equal(refAt(r, v, -0.1), null)
  assert.equal(refAt(r, v, 3.1), null)
})

test('what was kept between runs is checked', () => {
  assert.ok(isReference({ t: [0, 1], traces: [{ name: 'CH1', v: [1, 2] }], math: 'off', source: '', timeDiv: 0 }))
  assert.ok(!isReference({ t: [0, 1], traces: [{ name: 'CH3', v: [1, 2] }] }))
  assert.ok(!isReference({ t: [0, 1], traces: [{ name: 'CH1', v: [1] }] }))
  assert.ok(!isReference(null))
})
