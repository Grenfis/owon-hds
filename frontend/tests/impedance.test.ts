// Run with: npm test
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { bestFreq, cdiv, impedance, inputLoad, read, type C, type Load, type Scheme } from '../src/impedance.ts'

const near = (got: number, want: number, tol: number, what: string) =>
  assert.ok(Math.abs(got / want - 1) < tol, `${what}: ${got}, want ${want}`)

const par = (a: C, b: C): C => cdiv([1, 0], [cdiv([1, 0], a)[0] + cdiv([1, 0], b)[0], cdiv([1, 0], a)[1] + cdiv([1, 0], b)[1]])

// ratio is CH2/CH1 for a part z, the scope input across CH2 included.
function ratio(z: C, rRef: number, f: number, scheme: Scheme, load: Load | null): C {
  const w = 2 * Math.PI * f
  const zin: C | null = load ? cdiv([1, 0], [1 / load.r, w * load.c]) : null
  if (scheme === 'top') {
    const zl = zin ? par(z, zin) : z
    return cdiv(zl, [zl[0] + rRef, zl[1]])
  }
  const rl = zin ? par([rRef, 0], zin) : ([rRef, 0] as C)
  return cdiv(rl, [rl[0] + z[0], rl[1] + z[1]])
}

test('capacitor with ESR, part to ground, input load removed', () => {
  const f = 1e3, cap = 100e-9, esr = 2
  const z: C = [esr, -1 / (2 * Math.PI * f * cap)]
  const load = inputLoad(1)
  const r = read(impedance(ratio(z, 1e3, f, 'top', load), 1e3, f, 'top', load), f)
  assert.equal(r.kind, 'C')
  near(r.series.value, cap, 1e-9, 'C')
  near(r.series.r, esr, 1e-6, 'ESR')
  near(r.d, esr * 2 * Math.PI * f * cap, 1e-6, 'D')
})

test('without the correction a small capacitor reads 16 pF high', () => {
  const f = 10e3, cap = 1e-9
  const z: C = [0, -1 / (2 * Math.PI * f * cap)]
  const r = read(impedance(ratio(z, 10e3, f, 'top', inputLoad(1)), 10e3, f, 'top', null), f)
  near(r.parallel.value, cap + 16e-12, 1e-6, 'Cp')
})

test('inductor with resistance, resistor to ground', () => {
  const f = 100e3, ind = 1e-3, rs = 5
  const z: C = [rs, 2 * Math.PI * f * ind]
  const load = inputLoad(10)
  const r = read(impedance(ratio(z, 470, f, 'bottom', load), 470, f, 'bottom', load), f)
  assert.equal(r.kind, 'L')
  near(r.series.value, ind, 1e-9, 'L')
  near(r.series.r, rs, 1e-6, 'R')
  near(1 / r.d, (2 * Math.PI * f * ind) / rs, 1e-6, 'Q')
})

test('resistor and the parallel model', () => {
  const r = read([330, -1], 1e3)
  assert.equal(r.kind, 'R')
  near(r.series.value, 330, 1e-9, 'R')
  // A capacitor with a leak: 10 nF in parallel with 100 kΩ.
  const f = 1e3, w = 2 * Math.PI * f
  const z = cdiv([1, 0], [1 / 100e3, w * 10e-9])
  const p = read(z, f).parallel
  near(p.value, 10e-9, 1e-9, 'Cp')
  near(p.r, 100e3, 1e-9, 'Rp')
})

test('best frequency puts the part at the reference', () => {
  const rd = read([0, -1 / (2 * Math.PI * 1e3 * 100e-9)], 1e3)
  near(bestFreq(rd, 1e3)!, 1 / (2 * Math.PI * 1e3 * 100e-9), 1e-9, 'f')
  const rl = read([0, 2 * Math.PI * 1e3 * 10e-3], 1e3)
  near(bestFreq(rl, 100)!, 100 / (2 * Math.PI * 10e-3), 1e-9, 'f')
})
