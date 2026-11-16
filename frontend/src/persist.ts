// Persistence: the traces of successive frames pile up in a buffer of screen
// pixels. Each pixel counts the frames whose trace passed it, and the counts
// fade with a time constant or never. The brightness of a pixel then shows
// how often the signal goes there: a steady trace is bright, jitter is a
// dimmer band around it, and a rare glitch stays faint but visible.

export type PersistMode = 'off' | '1' | '5' | 'inf'

export const PERSIST_MODES: { value: PersistMode; label: string; title: string }[] = [
  { value: 'off', label: 'Выкл', title: 'Только последний кадр' },
  { value: '1', label: '1 с', title: 'Следы гаснут примерно за секунду' },
  { value: '5', label: '5 с', title: 'Следы гаснут примерно за пять секунд' },
  { value: 'inf', label: '∞', title: 'Следы копятся, пока не сбросить или не сменить настройки' },
]

// fadeFactor is how much of the counts stays after dt seconds.
export function fadeFactor(mode: PersistMode, dt: number): number {
  if (mode === 'inf' || mode === 'off') return 1
  return Math.exp(-Math.min(dt, 2) / +mode)
}

export class Layer {
  readonly w: number
  readonly h: number
  readonly hits: Float32Array
  total = 0 // the frames added, faded as the counts are
  private stamp: Uint32Array
  private frame = 0

  constructor(w: number, h: number) {
    this.w = Math.max(1, Math.round(w))
    this.h = Math.max(1, Math.round(h))
    this.hits = new Float32Array(this.w * this.h)
    this.stamp = new Uint32Array(this.w * this.h)
  }

  fade(k: number) {
    if (k >= 1) return
    for (let i = 0; i < this.hits.length; i++) this.hits[i] *= k
    this.total *= k
  }

  // add draws the polyline through the points, in pixels of the layer, as
  // one frame: a pixel counts once however often the line passes it.
  add(xs: ArrayLike<number>, ys: ArrayLike<number>) {
    const n = Math.min(xs.length, ys.length)
    if (!n) return
    this.frame++
    this.total++
    // Points far beyond the screen would make long segments; a little past
    // its edge is as good.
    const cx = (x: number) => Math.max(-2, Math.min(this.w + 1, x))
    const cy = (y: number) => Math.max(-2, Math.min(this.h + 1, y))
    let x0 = cx(xs[0]), y0 = cy(ys[0])
    this.plot(x0, y0)
    for (let i = 1; i < n; i++) {
      const x1 = cx(xs[i]), y1 = cy(ys[i])
      const steps = Math.ceil(Math.max(Math.abs(x1 - x0), Math.abs(y1 - y0)))
      for (let s = 1; s <= steps; s++) this.plot(x0 + ((x1 - x0) * s) / steps, y0 + ((y1 - y0) * s) / steps)
      ;[x0, y0] = [x1, y1]
    }
  }

  private plot(x: number, y: number) {
    const px = Math.round(x), py = Math.round(y)
    if (px < 0 || py < 0 || px >= this.w || py >= this.h) return
    const i = py * this.w + px
    if (this.stamp[i] !== this.frame) {
      this.stamp[i] = this.frame
      this.hits[i]++
    }
  }

  // render paints the layer in one colour. The opacity grows with the share
  // of frames that passed a pixel, and a pixel left behind fades as its
  // count decays below one frame.
  render(img: ImageData, rgb: [number, number, number]) {
    const d = img.data, [r, g, b] = rgb
    for (let i = 0; i < this.hits.length; i++) {
      const v = this.hits[i]
      const j = i * 4
      if (v < 0.02 || !this.total) {
        d[j + 3] = 0
        continue
      }
      d[j] = r
      d[j + 1] = g
      d[j + 2] = b
      d[j + 3] = 255 * opacity(v, this.total)
    }
  }
}

export function opacity(v: number, total: number): number {
  return Math.min(1, v) * (0.3 + 0.7 * Math.sqrt(Math.min(1, v / total)))
}

export function hexRgb(hex: string): [number, number, number] {
  const m = hex.trim().match(/^#?([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i)
  return m ? [parseInt(m[1], 16), parseInt(m[2], 16), parseInt(m[3], 16)] : [255, 255, 255]
}
