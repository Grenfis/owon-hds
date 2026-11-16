// Screen cursors. They sit at positions in divisions, as on the device, so
// a change of a scale keeps them in place and changes what they read: the
// time cursors from the left edge, 0 to 12, the voltage cursors from the
// centre, −4 to 4.

export const DIVX = 12, DIVY = 8
const PX = 25 // sample counts per vertical division

export type CursorMode = 'off' | 't' | 'v' | 'tv'

export interface Cursors {
  mode: CursorMode
  src: 'ch1' | 'ch2' // the channel the voltage cursors read
  t: [number, number]
  v: [number, number]
}

export const DEFAULT_CURSORS: Cursors = { mode: 'off', src: 'ch1', t: [3, 9], v: [2, -2] }

export const hasT = (m: CursorMode) => m === 't' || m === 'tv'
export const hasV = (m: CursorMode) => m === 'v' || m === 'tv'

// snapT keeps a time cursor on the screen and on a sample: 600 samples over
// 12 divisions are 50 a division. snapV keeps a voltage cursor on a count.
export const snapT = (x: number) => Math.round(Math.max(0, Math.min(DIVX, x)) * 50) / 50
export const snapV = (y: number) => Math.round(Math.max(-DIVY / 2, Math.min(DIVY / 2, y)) * PX) / PX

// timeAt is the time of a position from the trigger point. A positive
// horizontal offset moves the trigger point left of the centre.
export function timeAt(x: number, timeDiv: number, hOffsetDiv: number): number {
  return (x - DIVX / 2 + hOffsetDiv) * timeDiv
}

// sampleAt reads a trace at a position, between the samples linearly; the
// screen shows the samples from edge to edge.
export function sampleAt(raw: ArrayLike<number>, x: number): number {
  const n = raw.length
  const p = Math.max(0, Math.min(n - 1, (x / DIVX) * (n - 1)))
  const i = Math.min(n - 2, Math.floor(p))
  return raw[i] + (p - i) * (raw[i + 1] - raw[i])
}

// voltsAt turns a screen height in counts into volts of a channel.
export function voltsAt(counts: number, offsetCounts: number, voltsPerDiv: number): number {
  return ((counts - offsetCounts) / PX) * voltsPerDiv
}

// levelAt is the voltage of a channel at a voltage cursor, given in
// divisions from the centre.
export function levelAt(y: number, offsetCounts: number, voltsPerDiv: number): number {
  return voltsAt(y * PX, offsetCounts, voltsPerDiv)
}
