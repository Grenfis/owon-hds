// Run with: npm test
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { Layer, fadeFactor, hexRgb, opacity } from '../src/persist.ts'

const at = (l: Layer, x: number, y: number) => l.hits[y * l.w + x]

test('a line counts each pixel once per frame', () => {
  const l = new Layer(10, 10)
  // There and back over the same pixels: still one frame.
  l.add([0, 9, 0], [5, 5, 5])
  for (let x = 0; x < 10; x++) assert.equal(at(l, x, 5), 1)
  assert.equal(at(l, 0, 4), 0)
  l.add([0, 9], [5, 5])
  assert.equal(at(l, 3, 5), 2)
  assert.equal(l.total, 2)
})

test('a steep edge fills the pixels between the samples', () => {
  const l = new Layer(10, 10)
  l.add([2, 3], [0, 9])
  let n = 0
  for (let y = 0; y < 10; y++) n += at(l, 2, y) + at(l, 3, y)
  assert.equal(n, 10)
})

test('points beyond the screen are clipped', () => {
  const l = new Layer(4, 4)
  l.add([0, 1e9], [1, 1])
  assert.equal(at(l, 3, 1), 1)
})

test('counts fade with the time constant', () => {
  const l = new Layer(2, 1)
  l.add([0], [0])
  l.fade(fadeFactor('1', 1))
  assert.ok(Math.abs(at(l, 0, 0) - Math.exp(-1)) < 1e-6)
  assert.equal(fadeFactor('inf', 10), 1)
  // A long pause fades no more than two seconds would.
  assert.equal(fadeFactor('5', 100), Math.exp(-2 / 5))
})

test('brightness grades how often a pixel is passed', () => {
  // Every frame of a hundred: full; one of a hundred: faint but visible.
  assert.equal(opacity(100, 100), 1)
  assert.ok(Math.abs(opacity(1, 100) - 0.37) < 1e-9)
  // Left behind and fading: dimmer still.
  assert.ok(opacity(0.5, 100) < opacity(1, 100))
})

test('colours from CSS', () => {
  assert.deepEqual(hexRgb('#f4c84a'), [244, 200, 74])
  assert.deepEqual(hexRgb('bad'), [255, 255, 255])
})
