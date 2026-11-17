// Run with: npm test
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { elapsed, envelope, firstAt, logCSV, side, timeStep } from '../src/dmmlog.ts'

test('a reading against the limits', () => {
  assert.equal(side(5, 1, 10), null)
  assert.equal(side(0.5, 1, 10), 'lo')
  assert.equal(side(11, 1, 10), 'hi')
  assert.equal(side(-100, null, 10), null)
  assert.equal(side(100, 1, null), null)
})

test('chart columns keep the extremes', () => {
  const ts = [0, 1, 2, 3, 4, 5, 6, 7]
  const vs = [1, 9, 2, 2, 3, -4, 5, 5]
  assert.equal(firstAt(ts, 2.5), 3)
  assert.equal(firstAt(ts, 100), 8)
  const { lo, hi } = envelope(ts, vs, 0, 8, 2)
  assert.deepEqual([...lo], [1, -4])
  assert.deepEqual([...hi], [9, 5])
  // A window with a gap leaves its columns empty.
  const e = envelope([0, 10], [1, 2], 0, 10, 4)
  assert.ok(Number.isNaN(e.lo[1]))
  assert.equal(e.hi[3], 2)
})

test('the time axis', () => {
  assert.equal(timeStep(60, 8), 10)
  assert.equal(timeStep(3600, 8), 600)
  assert.equal(elapsed(5), '0:05')
  assert.equal(elapsed(750), '12:30')
  assert.equal(elapsed(3720), '1:02:00')
})

test('the CSV of a log', () => {
  const t0 = new Date(2026, 10, 17, 9, 5, 7, 42).getTime()
  const csv = logCSV([t0, t0 + 250], [1.2345, 1.2346], 'В', 'Пост. напряжение').split('\n')
  assert.equal(csv[1], 'time,t_s,value_V')
  assert.equal(csv[2], '2026-11-17 09:05:07.042,0.000,1.2345')
  assert.equal(csv[3], '2026-11-17 09:05:07.292,0.250,1.2346')
})
