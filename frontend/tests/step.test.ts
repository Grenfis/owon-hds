// Run with: npm test
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { average, metrics, steps } from '../src/step.ts'

const near = (got: number | null, want: number, tol: number, what: string) =>
  assert.ok(got != null && Math.abs(got / want - 1) < tol, `${what}: ${got}, want ${want}`)

// screen simulates a square wave of `period` samples between 0.2 and 1.2 V
// through a system, sampled every dt. The edges fall halfway between
// samples, where the input crossing is found exactly. The system integrates
// in 100 steps per sample; `next` advances its state by h for an input u.
function screen(period: number, n: number, dt: number, next: (u: number, h: number) => number, quantum = 0) {
  const input: number[] = [], output: number[] = []
  const h = dt / 100
  let y = 0.2
  // Settle on the low level first.
  for (let i = 0; i < 20000; i++) y = next(0.2, h)
  for (let i = 0; i < n; i++) {
    const u = (i - 0.5 + period / 4) % period < period / 2 ? 0.2 : 1.2
    input.push(u)
    output.push(quantum ? Math.round(y / quantum) * quantum : y)
    for (let s = 0; s < 100; s++) {
      const t = i + s / 100
      y = next((t - 0.5 + period / 4) % period < period / 2 ? 0.2 : 1.2, h)
    }
  }
  return { input, output }
}

function rc(tau: number) {
  let y = 0.2
  return (u: number, h: number) => (y += (u - y) * (1 - Math.exp(-h / tau)))
}

function second(wn: number, zeta: number) {
  let x = 0.2, v = 0
  return (u: number, h: number) => {
    v += h * (wn * wn * (u - x) - 2 * zeta * wn * v)
    x += h * v
    return x
  }
}

test('RC: delay, rise, settling, no overshoot', () => {
  const dt = 1e-6, tau = 20e-6
  const { input, output } = screen(300, 600, dt, rc(tau))
  const ss = steps(input, output, true)
  // Rising edges at 76 and 376, falling ones at 226 and 526.
  assert.equal(ss.length, 2)
  const s = average(ss)!
  assert.equal(s.y.length - s.pre, 150)
  near(s.gain, 1, 0.01, 'gain')
  const m = metrics(s, dt, 0.05)
  near(m.delay, Math.LN2 * tau, 0.03, 'delay')
  near(m.rise, Math.log(9) * tau, 0.03, 'rise')
  near(m.settling, -Math.log(0.05) * tau, 0.04, 'settling')
  assert.ok(m.overshoot < 0.005, `overshoot ${m.overshoot}`)
  // No noise here, only the tail of the previous step: e^−7.5 of it.
  assert.ok(m.noise < 0.002, `noise ${m.noise}`)
  assert.equal(m.ring, null)
})

test('second order: overshoot and ringing', () => {
  const dt = 1e-6, wn = 2 * Math.PI * 20e3, zeta = 0.3
  const { input, output } = screen(400, 600, dt, second(wn, zeta))
  const m = metrics(average(steps(input, output, true))!, dt, 0.05)
  near(m.overshoot, Math.exp((-Math.PI * zeta) / Math.sqrt(1 - zeta * zeta)), 0.05, 'overshoot')
  near(m.ring, (wn * Math.sqrt(1 - zeta * zeta)) / (2 * Math.PI), 0.08, 'ringing')
  near(m.peak, Math.PI / (wn * Math.sqrt(1 - zeta * zeta)), 0.05, 'peak time')
  assert.ok(m.settling != null && m.settling > m.peak!, `settling ${m.settling}`)
})

test('falling edges, quantised to counts', () => {
  const dt = 1e-6, tau = 20e-6
  // 25 counts per division at 0.2 V/div: the step is 125 counts.
  const { input, output } = screen(300, 600, dt, rc(tau), 0.2 / 25)
  // The falling edge at 526 leaves 74 samples to the screen end and is
  // left out for the whole one at 226.
  const ss = steps(input, output, false)
  assert.equal(ss.length, 1)
  assert.equal(ss[0].y.length - ss[0].pre, 150)
  const s = average(ss)!
  near(s.swing, -1, 0.02, 'swing')
  const m = metrics(s, dt, 0.05)
  // Near 90 % the curve takes 1.6 samples to climb one count, and without
  // noise the counts are not dithered: the rise may be off by 8 %.
  near(m.rise, Math.log(9) * tau, 0.08, 'rise')
  near(m.delay, Math.LN2 * tau, 0.04, 'delay')
})

test('a step cut short by the next edge still moves', () => {
  const dt = 1e-6, tau = 40e-6
  const { input, output } = screen(100, 600, dt, rc(tau))
  const s = average(steps(input, output, true))!
  assert.ok(s.drift > 0.05, `drift ${s.drift}`)
})

test('a lone edge cut by the screen end still counts', () => {
  const dt = 1e-6, tau = 5e-6
  const { input, output } = screen(1000, 600, dt, rc(tau))
  const ss = steps(input, output, true)
  assert.equal(ss.length, 1)
  near(metrics(ss[0], dt, 0.05).rise, Math.log(9) * tau, 0.05, 'rise')
})

test('no edges on a flat input', () => {
  assert.deepEqual(steps(new Array(600).fill(0.5), new Array(600).fill(0.1), true), [])
})
