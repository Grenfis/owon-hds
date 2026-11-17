// Run with: npm test
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { push, sd, type Running } from '../src/stats.ts'

test('statistics of a series, one value at a time', () => {
  let s: Running | undefined
  for (const x of [2, 4, 4, 4, 5, 5, 7, 9]) s = push(s, x)
  assert.equal(s!.n, 8)
  assert.equal(s!.mean, 5)
  assert.equal(s!.min, 2)
  assert.equal(s!.max, 9)
  // The sample standard deviation of this series is √(32/7).
  assert.ok(Math.abs(sd(s!) - Math.sqrt(32 / 7)) < 1e-12)
})

test('a single value has no spread', () => {
  assert.equal(sd(push(undefined, 3)), 0)
})
