// Number formatting with Russian unit prefixes, and the device's own lists
// of vertical and horizontal scales.

const PREFIXES: [number, string][] = [
  [1e9, 'Г'], [1e6, 'М'], [1e3, 'к'], [1, ''], [1e-3, 'м'], [1e-6, 'мк'], [1e-9, 'н'], [1e-12, 'п'],
]

// eng prints a value with an SI prefix and sig significant digits; trim
// drops trailing zeros ("1 мс" instead of "1.00 мс").
export function eng(v: number | null | undefined, unit: string, sig = 4, trim = false): string {
  if (v == null || !Number.isFinite(v)) return '—'
  if (v === 0) return `0 ${unit}`
  const a = Math.abs(v)
  let [mult, prefix] = PREFIXES[PREFIXES.length - 1]
  for (const [m, p] of PREFIXES) {
    if (a >= m * 0.99995) {
      ;[mult, prefix] = [m, p]
      break
    }
  }
  const x = v / mult
  const digits = Math.min(6, Math.max(0, sig - 1 - Math.floor(Math.log10(Math.abs(x)))))
  let s = x.toFixed(digits)
  if (trim && s.includes('.')) s = s.replace(/\.?0+$/, '')
  return `${s.replace('-', '−')} ${prefix}${unit}`
}

// stamp is the local time for file names: 20261116_143005.
export function stamp(): string {
  const d = new Date(), p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}${p(d.getMonth() + 1)}${p(d.getDate())}_${p(d.getHours())}${p(d.getMinutes())}${p(d.getSeconds())}`
}

export function signed(v: number, digits = 2): string {
  const s = v.toFixed(digits)
  return v > 0 ? `+${s}` : s.replace('-', '−')
}

export interface Option {
  value: number
  cmd: string // as the device expects it, e.g. "100mV" or "200us"
  label: string
}

function fmt3(y: number): string {
  return y < 10 ? y.toFixed(2) : y < 100 ? y.toFixed(1) : y.toFixed(0)
}

const VDIV_1X = [0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10]

// Vertical scales for a probe factor, as listed in OWON's SCPI manual.
export function vdivOptions(probe: number): Option[] {
  return VDIV_1X.map((v) => {
    const x = +(v * probe).toPrecision(6)
    const cmd = x < 1 ? `${fmt3(x * 1000)}mV` : x >= 1000 ? `${fmt3(x / 1000)}kV` : `${fmt3(x)}V`
    return { value: x, cmd, label: eng(x, 'В', 3, true) }
  })
}

export const TIMEBASES: Option[] = (() => {
  const out: Option[] = []
  for (let e = -9; e <= 3; e++) {
    for (const m of [1, 2, 5]) {
      const t = +(m * 10 ** e).toPrecision(6)
      if (t < 2e-9 || t > 1000) continue
      const [unit, mult] = t < 1e-6 ? ['ns', 1e-9] : t < 1e-3 ? ['us', 1e-6] : t < 1 ? ['ms', 1e-3] : ['s', 1]
      const y = +(t / mult).toPrecision(6)
      out.push({ value: t, cmd: (y < 10 ? y.toFixed(1) : y.toFixed(0)) + unit, label: eng(t, 'с', 3, true) })
    }
  }
  return out
})()

export function closest(options: Option[], v: number): number {
  let best = 0
  for (let i = 1; i < options.length; i++) {
    if (Math.abs(Math.log(options[i].value / v)) < Math.abs(Math.log(options[best].value / v))) best = i
  }
  return best
}

// fitScale returns the finest vertical scale that keeps a swing of pp
// within six divisions.
export function fitScale(probe: string, pp: number): Option {
  const opts = vdivOptions(probeFactor(probe))
  return opts.find((o) => pp / o.value <= 6) ?? opts[opts.length - 1]
}

export function probeFactor(p: string): number {
  const f = parseFloat(p)
  return Number.isFinite(f) && f > 0 ? f : 1
}

// Parses a number typed by the user, accepting SI suffixes: "1.5k", "2M",
// "250m", "10u", and comma as the decimal separator.
export function parseInput(s: string): number | null {
  const m = s.trim().replace(',', '.').match(/^([-+−]?\d*\.?\d+(?:e[-+]?\d+)?)\s*(мк|[pnuµмmкkМMГG])?/i)
  if (!m) return null
  const mult: Record<string, number> = {
    p: 1e-12, n: 1e-9, u: 1e-6, µ: 1e-6, мк: 1e-6, m: 1e-3, м: 1e-3, k: 1e3, к: 1e3, M: 1e6, М: 1e6, G: 1e9, Г: 1e9,
  }
  const x = parseFloat(m[1].replace('−', '-'))
  const p = m[2]
  if (!p) return x
  // "m" is milli and "M" is mega; other prefixes are case-insensitive.
  return x * (mult[p] ?? mult[p.toLowerCase()] ?? 1)
}
