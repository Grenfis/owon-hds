// Run with: npm test
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { levelAt, sampleAt, snapT, snapV, timeAt } from '../src/cursors.ts'

test('time from the trigger point', () => {
  // The centre at 1 ms/div is the trigger point without an offset.
  assert.equal(timeAt(6, 1e-3, 0), 0)
  assert.equal(timeAt(9, 1e-3, 0), 3e-3)
  // An offset of 4.5 divisions puts the trigger point 1.5 divisions from
  // the left edge.
  assert.ok(Math.abs(timeAt(1.5, 1e-3, 4.5)) < 1e-12)
})

test('a trace between its samples', () => {
  const raw = Array.from({ length: 601 }, (_, i) => i % 2 ? 10 : 0)
  // 601 samples: one every 1/50 division.
  assert.equal(sampleAt(raw, 0), 0)
  assert.equal(sampleAt(raw, 0.02), 10)
  assert.ok(Math.abs(sampleAt(raw, 0.01) - 5) < 1e-9)
  assert.equal(sampleAt(raw, 12), 0)
  assert.equal(sampleAt(raw, 20), 0)
})

test('a voltage cursor against the channel offset', () => {
  // Two divisions up at 0.5 V/div with the channel a division up: 0.5 V.
  assert.equal(levelAt(2, 25, 0.5), 0.5)
  assert.equal(levelAt(-1, 0, 0.2), -0.2)
})

test('cursors snap to samples and counts on the screen', () => {
  assert.equal(snapT(3.013), 3.02)
  assert.equal(snapT(-1), 0)
  assert.equal(snapT(13), 12)
  assert.equal(snapV(1.01), 1)
  assert.equal(snapV(1.03), 1.04)
  assert.equal(snapV(9), 4)
})
