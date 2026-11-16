// Spectrum of one screen of samples, the way the device's own FFT menu and
// OWON's PC software show it: a window, zero padding to 2048 points and the
// level of every bin as an RMS voltage.

export type WindowName = 'hann' | 'hamming' | 'blackman' | 'flattop' | 'rect'

// lobe is the half-width of the main lobe in screen bins (1/screen time).
// range is how far below the strongest peak other peaks are still told
// apart from its side lobes, checked on single sines across the band.
export const WINDOWS: { value: WindowName; label: string; title: string; lobe: number; range: number }[] = [
  { value: 'hann', label: 'Ханн', title: 'Окно общего назначения: узкие пики и малые утечки', lobe: 2, range: 50 },
  { value: 'hamming', label: 'Хэмминг', title: 'Чуть лучше разделяет близкие частоты, чем Ханн', lobe: 2, range: 34 },
  { value: 'blackman', label: 'Блэкман', title: 'Самые малые утечки: видно слабые гармоники рядом с сильным пиком', lobe: 3, range: 60 },
  { value: 'flattop', label: 'Плоское', title: 'Точная амплитуда пиков ценой широких пиков', lobe: 5, range: 60 },
  { value: 'rect', label: 'Прямоуг.', title: 'Без окна: для импульсов и сигналов, целиком лежащих на экране', lobe: 1, range: 25 },
]

const MIN_FFT = 2048

function windowCoefs(name: WindowName, n: number): Float64Array {
  const w = new Float64Array(n)
  for (let i = 0; i < n; i++) {
    const x = (2 * Math.PI * i) / (n - 1)
    switch (name) {
      case 'hann': w[i] = 0.5 - 0.5 * Math.cos(x); break
      case 'hamming': w[i] = 0.54 - 0.46 * Math.cos(x); break
      case 'blackman': w[i] = 0.42 - 0.5 * Math.cos(x) + 0.08 * Math.cos(2 * x); break
      case 'flattop':
        w[i] = 0.21557895 - 0.41663158 * Math.cos(x) + 0.277263158 * Math.cos(2 * x) -
          0.083578947 * Math.cos(3 * x) + 0.006947368 * Math.cos(4 * x)
        break
      default: w[i] = 1
    }
  }
  return w
}

// fft transforms re and im in place; their length is a power of two.
function fft(re: Float64Array, im: Float64Array) {
  const n = re.length
  for (let i = 1, j = 0; i < n; i++) {
    let bit = n >> 1
    for (; j & bit; bit >>= 1) j ^= bit
    j ^= bit
    if (i < j) {
      ;[re[i], re[j]] = [re[j], re[i]]
      ;[im[i], im[j]] = [im[j], im[i]]
    }
  }
  for (let len = 2; len <= n; len <<= 1) {
    const ang = (-2 * Math.PI) / len
    const wr = Math.cos(ang), wi = Math.sin(ang)
    for (let i = 0; i < n; i += len) {
      let cr = 1, ci = 0
      for (let k = 0; k < len / 2; k++) {
        const a = i + k, b = a + len / 2
        const tr = re[b] * cr - im[b] * ci
        const ti = re[b] * ci + im[b] * cr
        re[b] = re[a] - tr
        im[b] = im[a] - ti
        re[a] += tr
        im[a] += ti
        ;[cr, ci] = [cr * wr - ci * wi, cr * wi + ci * wr]
      }
    }
  }
}

export interface Spectrum {
  df: number // hertz per bin
  rms: Float64Array // RMS volts of each bin from 0 to the Nyquist frequency
  dc: number // window-weighted mean of the samples, removed before the transform
  // lobe is the half-width of a peak in bins: closer peaks are one peak.
  lobe: number
  range: number // see WINDOWS
}

// spectrum computes the RMS level of every frequency bin of samples taken
// dt seconds apart. The DC level is removed first, so that it does not smear
// over the low frequencies. It is the window-weighted mean: a plain mean over
// a screen that holds a part of a period is off by that part.
export function spectrum(volts: ArrayLike<number>, dt: number, win: WindowName): Spectrum {
  const n = volts.length
  let nfft = MIN_FFT
  while (nfft < n) nfft <<= 1
  const w = windowCoefs(win, n)
  let mean = 0, wsum = 0
  for (let i = 0; i < n; i++) {
    mean += volts[i] * w[i]
    wsum += w[i]
  }
  mean /= wsum
  const re = new Float64Array(nfft), im = new Float64Array(nfft)
  for (let i = 0; i < n; i++) re[i] = (volts[i] - mean) * w[i]
  fft(re, im)
  // A sine of amplitude A gives a bin of A·Σw/2; its RMS is A/√2.
  const rms = new Float64Array(nfft / 2 + 1)
  for (let k = 0; k < rms.length; k++) rms[k] = (Math.SQRT2 * Math.hypot(re[k], im[k])) / wsum
  const { lobe, range } = WINDOWS.find((w) => w.value === win) ?? WINDOWS[0]
  return { df: 1 / (nfft * dt), rms, dc: mean, lobe: Math.ceil((lobe * nfft) / n), range }
}

export interface Peak {
  freq: number
  rms: number
}

// peaks returns up to count of the strongest peaks, strongest first, down to
// the window's range below the strongest. A peak is the highest bin within
// two lobe widths on both sides: the near side lobes of a strong peak have a
// higher neighbour that close, so they do not count as peaks. The DC level is
// already removed, so the search starts right after 0 Hz: a few periods on
// the screen put the main lobe there.
export function peaks(s: Spectrum, count: number): Peak[] {
  const r = s.rms
  let floor = 0
  for (let k = 1; k < r.length; k++) floor = Math.max(floor, r[k])
  floor *= 10 ** (-s.range / 20)
  const cand: number[] = []
  for (let k = 1; k < r.length; k++) {
    if (r[k] <= floor) continue
    let top = true
    for (let j = Math.max(1, k - 2 * s.lobe); j <= Math.min(r.length - 1, k + 2 * s.lobe) && top; j++) {
      // Strict on the left only: of a flat top, the leftmost bin wins.
      top = j < k ? r[k] > r[j] : r[k] >= r[j]
    }
    if (top) cand.push(k)
  }
  cand.sort((a, b) => r[b] - r[a])
  return cand.slice(0, count).map((k) => {
    // The power centroid of the peak above half its level places the
    // frequency between bins; it works for the flat top too, where a
    // parabola through three bins does not.
    let p = 0, pk = 0
    for (let j = k; j >= 1 && r[j] >= r[k] / 2; j--) [p, pk] = [p + r[j] ** 2, pk + j * r[j] ** 2]
    for (let j = k + 1; j < r.length && r[j] >= r[k] / 2; j++) [p, pk] = [p + r[j] ** 2, pk + j * r[j] ** 2]
    return { freq: (pk / p) * s.df, rms: r[k] }
  })
}

// isInterleaveImage tells whether p sits at fs/2 − h·f1 for h = 0…5. The
// device fills the screen from two ADCs in turn, and their small mismatch
// mirrors the signal and its harmonics around half the sample rate.
export function isInterleaveImage(s: Spectrum, p: Peak, f1: Peak): boolean {
  if (p === f1) return false
  const nyq = (s.rms.length - 1) * s.df
  for (let h = 0; h <= 5; h++) {
    if (Math.abs(p.freq - Math.abs(nyq - h * f1.freq)) <= s.lobe * s.df) return true
  }
  return false
}

// thd is the total harmonic distortion of the fundamental f1, as a fraction:
// the RMS sum of harmonics 2…10 below the Nyquist frequency over the
// fundamental. It is null when no harmonic fits in the spectrum.
export function thd(s: Spectrum, f1: Peak): number | null {
  let sum = 0, found = 0
  for (let h = 2; h <= 10; h++) {
    const k = Math.round((h * f1.freq) / s.df)
    if (k + s.lobe >= s.rms.length) break
    let m = 0
    for (let j = k - s.lobe; j <= k + s.lobe; j++) m = Math.max(m, s.rms[j])
    sum += m * m
    found++
  }
  return found ? Math.sqrt(sum) / f1.rms : null
}

export function db(v: number): number {
  return 20 * Math.log10(Math.max(v, 1e-12))
}
