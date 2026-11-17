// Running statistics of a measurement over frames: count, mean, minimum,
// maximum and standard deviation, updated one value at a time (Welford), so
// that no frames need to be kept.

export interface Running {
  n: number
  mean: number
  m2: number // sum of squared deviations from the mean
  min: number
  max: number
}

export function push(s: Running | undefined, x: number): Running {
  if (!s) return { n: 1, mean: x, m2: 0, min: x, max: x }
  const n = s.n + 1
  const d = x - s.mean
  const mean = s.mean + d / n
  return { n, mean, m2: s.m2 + d * (x - mean), min: Math.min(s.min, x), max: Math.max(s.max, x) }
}

// sd is the standard deviation of the sample, 0 for a single value.
export function sd(s: Running): number {
  return s.n > 1 ? Math.sqrt(s.m2 / (s.n - 1)) : 0
}
