// The CSV of a frame: time from the trigger point and the volts of each
// channel that is on, and of the math trace when there is one. It is also
// what the reference waveform reads back.
import { mathCsvName, type MathOp } from './math.ts'
import type { Frame } from './types'

export function frameCSV(f: Frame, math: number[] | null, op: MathOp): string {
  const chans = f.channels.filter((c) => c.on && c.raw?.length)
  const n = Math.max(0, ...chans.map((c) => c.raw!.length))
  const m = math && op !== 'off' && math.length === n ? math : null
  const head = ['t_s', ...chans.map((c) => `${c.name}_V`), ...(m ? [`M_${mathCsvName(op)}`] : [])]
  const lines = [`# OWON HDS272S, ${f.timeScale}/div, ${f.sampleRate}; time from trigger point`, head.join(',')]
  for (let i = 0; i < n; i++) {
    // A positive offset puts the trigger point hOffset divisions left of centre.
    const t = ((i / (n - 1)) * 12 - 6 + f.hOffset) * f.timeDiv
    const vs = chans.map((c) => (((c.raw![i] - c.offset) / 25) * c.voltsPerDiv).toPrecision(5))
    lines.push([t.toPrecision(6), ...vs, ...(m ? [m[i].toPrecision(5)] : [])].join(','))
  }
  return lines.join('\n') + '\n'
}
