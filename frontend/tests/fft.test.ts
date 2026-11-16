// Run with: npm test
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { WINDOWS, isInterleaveImage, peaks, spectrum, thd } from '../src/fft.ts'

// One screen as the device sends it: 600 samples over 12 divisions of 500 µs.
const N = 600, DT = 6e-3 / N

function screen(f: (t: number) => number): number[] {
  return Array.from({ length: N }, (_, i) => f(i * DT))
}

test('sine level and frequency in every window', () => {
  // 1.07 kHz does not fit a whole number of periods on the screen.
  const v = screen((t) => 2 + 1.5 * Math.sin(2 * Math.PI * 1070 * t))
  for (const w of WINDOWS) {
    const s = spectrum(v, DT, w.value)
    const [p] = peaks(s, 1)
    assert.ok(Math.abs(p.freq - 1070) < 15, `${w.value}: freq ${p.freq}`)
    const tol = w.value === 'flattop' ? 0.01 : w.value === 'rect' ? 0.1 : 0.03
    assert.ok(Math.abs(p.rms / (1.5 / Math.SQRT2) - 1) < tol, `${w.value}: rms ${p.rms}`)
    assert.ok(Math.abs(s.dc - 2) < (w.value === 'rect' ? 0.1 : 0.01), `${w.value}: dc ${s.dc}`)
  }
})

test('side lobes of a sine are not peaks', () => {
  // 455 kHz at 1 µs/div: 5.5 periods on the screen, as on the device.
  const dt = 12e-6 / N
  const v = Array.from({ length: N }, (_, i) => 0.5 * Math.sin(2 * Math.PI * 455e3 * i * dt))
  for (const w of WINDOWS) {
    const s = spectrum(v, dt, w.value)
    const ps = peaks(s, 5)
    assert.equal(ps.length, 1, `${w.value}: ${ps.map((p) => p.freq.toFixed(0)).join(', ')}`)
  }
})

test('interleaved ADC gain mismatch mirrors the signal', () => {
  const dt = 12e-6 / N
  // Every other sample is 2 % larger: an image at fs/2 − f0.
  const v = Array.from({ length: N }, (_, i) => Math.sin(2 * Math.PI * 455e3 * i * dt) * (i % 2 ? 1.02 : 0.98))
  const s = spectrum(v, dt, 'hann')
  const ps = peaks(s, 3)
  assert.ok(Math.abs(ps[1].freq - (25e6 - 455e3)) < 30e3, `image at ${ps[1].freq}`)
  assert.ok(isInterleaveImage(s, ps[1], ps[0]))
  assert.ok(!isInterleaveImage(s, ps[0], ps[0]))
})

test('harmonic distortion of a sine with 10 % third harmonic', () => {
  const v = screen((t) => Math.sin(2 * Math.PI * 2000 * t) + 0.1 * Math.sin(2 * Math.PI * 6000 * t))
  const s = spectrum(v, DT, 'blackman')
  const ps = peaks(s, 3)
  assert.ok(Math.abs(ps[0].freq - 2000) < 20)
  assert.ok(Math.abs(ps[1].freq - 6000) < 20)
  const d = thd(s, ps[0])
  assert.ok(d != null && Math.abs(d - 0.1) < 0.01, `thd ${d}`)
})

test('two close tones stay two peaks', () => {
  const v = screen((t) => Math.sin(2 * Math.PI * 5000 * t) + 0.5 * Math.sin(2 * Math.PI * 6000 * t))
  const ps = peaks(spectrum(v, DT, 'hann'), 2)
  assert.equal(ps.length, 2)
  assert.ok(Math.abs(ps[1].freq - 6000) < 40, `second peak ${ps[1].freq}`)
})
