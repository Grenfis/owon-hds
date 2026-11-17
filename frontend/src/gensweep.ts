// A software sweep of the generator frequency: from f1 to f2 over a time,
// in equal steps of frequency or of its logarithm, once or round and round.
// The device has no sweep of its own; each step is one frequency command.

export interface GenSweep {
  f1: number
  f2: number
  time: number // seconds from f1 to f2
  log: boolean
  repeat: boolean
}

// sweepAt is the frequency t seconds after the start, to 0.1 Hz as the
// generator takes it, or null once a single sweep is over.
export function sweepAt(s: GenSweep, t: number): number | null {
  if (t >= s.time && !s.repeat) return null
  const x = (t % s.time) / s.time
  const f = s.log ? s.f1 * (s.f2 / s.f1) ** x : s.f1 + (s.f2 - s.f1) * x
  return Math.round(f * 10) / 10
}

// sweepError tells what is wrong with the settings, or ''.
export function sweepError(s: GenSweep, maxFreq: number): string {
  for (const f of [s.f1, s.f2]) {
    if (!(f >= 0.1 && f <= maxFreq)) return `Частота вне 0.1 Гц…${maxFreq / 1e6} МГц`
  }
  if (s.f1 === s.f2) return 'Начало и конец совпадают'
  if (!(s.time >= 1 && s.time <= 3600)) return 'Время от 1 с до часа'
  return ''
}

// fmtFreq writes a frequency the way parseInput reads it back: 10k, 2.5M.
export function fmtFreq(f: number): string {
  const [d, s] = f >= 1e6 ? [1e6, 'M'] : f >= 1e3 ? [1e3, 'k'] : [1, '']
  return `${+(f / d).toPrecision(6)}${s}`
}
