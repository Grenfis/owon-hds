// Run with: npm test
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { frameCSV } from '../src/export.ts'
import { parseRecording, recordingJSON, sameData, scopeOf } from '../src/record.ts'
import type { Frame } from '../src/types.ts'

function frame(raw1: number[], raw2: number[] | null = null): Frame {
  const ch = (name: string, raw: number[] | null) => ({ name, on: !!raw, coupling: 'DC', probe: '1X', voltsPerDiv: 1, offset: 25, raw, meas: {} })
  return {
    seq: 1, run: 'stop', timeDiv: 1e-3, timeScale: '1.0ms', hOffset: 2, sampleRate: '50kSa/s', acqMode: 'sample', depMem: '4K',
    trig: { source: 'ch1', edge: 'rise', sweep: 'single', coupling: 'dc', level: 0.5, levelStr: '500mV' },
    channels: [ch('CH1', raw1), ch('CH2', raw2)], pair: null, fps: 0,
  } as unknown as Frame
}

test('a recording goes to a file and back', () => {
  const rec = [{ t: 1, frame: frame([1, 2]) }, { t: 2, frame: frame([3, 4]) }]
  assert.deepEqual(parseRecording(recordingJSON(rec)), rec)
  assert.equal(typeof parseRecording('{'), 'string')
  assert.equal(typeof parseRecording('{"format":"x","frames":[]}'), 'string')
  assert.equal(typeof parseRecording(recordingJSON([])), 'string')
})

test('a repeated capture is the same data', () => {
  assert.ok(sameData(frame([1, 2], [3, 4]), frame([1, 2], [3, 4])))
  assert.ok(!sameData(frame([1, 2]), frame([1, 3])))
  assert.ok(!sameData(frame([1, 2]), frame([1, 2], [0, 0])))
})

test('the screen of a recorded frame', () => {
  const s = scopeOf(frame([1, 2]), { 'ch2.display': 'ON', 'hor.scale': '1.0ms' })
  assert.equal(s['ch1.display'], 'ON')
  assert.equal(s['ch2.display'], 'OFF')
  assert.equal(s['ch1.offset'], '1.00')
  assert.equal(s['hor.offset'], '2')
  assert.equal(s['trig.source'], 'CH1')
  assert.equal(s['trig.level'], '0.5')
  assert.equal(s['hor.scale'], '1.0ms')
})

test('the CSV of a frame', () => {
  // Offset 25 counts: raw 50 is one division, 1 V.
  const csv = frameCSV(frame([25, 50]), [7, 8], 'add').split('\n')
  assert.equal(csv[1], 't_s,CH1_V,M_CH1+CH2_V')
  // 600 samples would span 12 divisions; two span them too, from −6 + 2 div.
  assert.equal(csv[2], '-0.00400000,0.0000,7.0000')
  assert.equal(csv[3], '0.00800000,1.0000,8.0000')
})
