// Run with: npm test
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { fmtFreq, sweepAt, sweepError } from '../src/gensweep.ts'

test('a log sweep passes the geometric middle halfway', () => {
  const s = { f1: 100, f2: 10000, time: 10, log: true, repeat: false }
  assert.equal(sweepAt(s, 0), 100)
  assert.equal(sweepAt(s, 5), 1000)
  assert.equal(sweepAt(s, 9.999)! > 9990, true)
  assert.equal(sweepAt(s, 10), null)
})

test('a linear sweep goes round when repeated', () => {
  const s = { f1: 1000, f2: 2000, time: 4, log: false, repeat: true }
  assert.equal(sweepAt(s, 1), 1250)
  assert.equal(sweepAt(s, 5), 1250)
  // Down as well as up.
  assert.equal(sweepAt({ ...s, f1: 2000, f2: 1000 }, 1), 1750)
})

test('settings out of range', () => {
  const s = { f1: 100, f2: 10000, time: 10, log: true, repeat: false }
  assert.equal(sweepError(s, 25e6), '')
  assert.notEqual(sweepError({ ...s, f2: 30e6 }, 25e6), '')
  assert.notEqual(sweepError({ ...s, f2: 100 }, 25e6), '')
  assert.notEqual(sweepError({ ...s, time: 0.5 }, 25e6), '')
})

test('frequencies are written as they are typed', () => {
  assert.equal(fmtFreq(100), '100')
  assert.equal(fmtFreq(12500), '12.5k')
  assert.equal(fmtFreq(2.5e6), '2.5M')
})
