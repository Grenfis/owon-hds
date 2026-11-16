// Run with: npm test
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { MATH_SCALES, fitMath, mathOf, mathStats } from '../src/math.ts'
import type { Frame, FrameChannel } from '../src/types.ts'

function channel(name: string, raw: number[], voltsPerDiv: number, offset = 0, on = true): FrameChannel {
  return { name, on, coupling: 'DC', probe: '1X', voltsPerDiv, offset, raw, meas: { clipped: false } as FrameChannel['meas'] }
}
const frame = (c1: FrameChannel, c2: FrameChannel) => ({ channels: [c1, c2] }) as Frame

test('the functions, in volts of each channel', () => {
  // CH1 at 1 V/div a division up: raw 50 with offset 25 is 1 V. CH2 at
  // 0.5 V/div: raw −50 is −1 V.
  const f = frame(channel('CH1', [50, 25], 1, 25), channel('CH2', [-50, 0], 0.5))
  assert.deepEqual(mathOf(f, 'add')!.v, [0, 0])
  assert.deepEqual(mathOf(f, 'sub')!.v, [2, 0])
  assert.deepEqual(mathOf(f, 'rsub')!.v, [-2, 0])
  assert.deepEqual(mathOf(f, 'mul')!.v, [-1, 0])
  assert.equal(mathOf(f, 'off'), null)
})

test('both channels are needed', () => {
  const f = frame(channel('CH1', [1, 2], 1), channel('CH2', [1, 2], 1, 0, false))
  assert.equal(mathOf(f, 'add'), null)
})

test('the scale fits the swing in six divisions and the offset centres it', () => {
  // 0.5…3.5 V: a 3 V swing fits 0.5 V/div; the middle at 2 V goes 4 divisions down.
  assert.deepEqual(fitMath([0.5, 3.5]), { scale: 0.5, offset: -4 })
  assert.deepEqual(fitMath([-0.1, 0.1]), { scale: 0.05, offset: 0 })
  assert.equal(MATH_SCALES[0], 0.001)
  assert.equal(MATH_SCALES[MATH_SCALES.length - 1], 1000)
})

test('statistics over the screen', () => {
  const m = mathStats([1, -1, 1, -1])
  assert.deepEqual(m, { pp: 2, max: 1, min: -1, avg: 0, rms: 1 })
})
