// Run with: npm test
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { changes, describe, merge, parsePresets, presetsJSON, snapshot } from '../src/presets.ts'

const scope = {
  'ch1.display': 'ON', 'ch1.coupling': 'DC', 'ch1.probe': '1X', 'ch1.scale': '500mV', 'ch1.offset': '0.00',
  'ch2.display': 'OFF', 'ch2.coupling': 'DC', 'ch2.probe': '1X', 'ch2.scale': '1.00V', 'ch2.offset': '0.00',
  'hor.scale': '200us', 'hor.offset': '0', 'trig.source': 'CH1', 'trig.edge': 'RISE', 'trig.sweep': 'AUTO',
  'trig.level': '0', 'trig.coupling': 'DC', 'acq.mode': 'SAMPle', 'acq.depmem': '4K', run: 'RUN',
}
const gen = { wave: 'SINE', freq: 1000, amp: 2, offset: 0, duty: 50, symmetry: 50, output: true }
const display = { math: 'off', mathScale: 1, mathOffset: 0, xy: false, ref: true, avg: 1, persist: 'off' }

test('a snapshot leaves out run/stop and the generator output', () => {
  const p = snapshot('a', scope, gen, display)
  assert.equal(p.scope.run, undefined)
  assert.equal(p.gen!.output, undefined)
  assert.equal(p.gen!.freq, 1000)
  assert.equal(snapshot('b', scope, null, display).gen, undefined)
})

test('only what differs goes out, the probe before the scale', () => {
  const p = snapshot('a', scope, null, display)
  assert.deepEqual(changes(p, scope), [])
  // The level reads back as another spelling of the same number.
  assert.deepEqual(changes(p, { ...scope, 'trig.level': '0.0' }), [])
  const cur = { ...scope, 'ch1.probe': '10X', 'ch1.scale': '500mV', 'hor.scale': '1.0ms' }
  assert.deepEqual(changes(p, cur), [['ch1.probe', '1X'], ['ch1.scale', '500mV'], ['hor.scale', '200us']])
})

test('presets go to a file and back, a name replaces its namesake', () => {
  const a = snapshot('a', scope, gen, display)
  const b = { ...snapshot('b', scope, null, display), at: 5 }
  assert.deepEqual(parsePresets(presetsJSON([a, b])), [a, b])
  assert.equal(typeof parsePresets('[]'), 'string')
  assert.equal(typeof parsePresets('{"format":"x"}'), 'string')
  const a2 = { ...a, at: 9 }
  assert.deepEqual(merge([a, b], [a2]), [a2, b])
})

test('a preset in a line', () => {
  assert.equal(describe(snapshot('a', scope, gen, display)), 'CH1 500mV/дел · 200us/дел · запуск авто · генератор SINE 1000 Гц')
})
