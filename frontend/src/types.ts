// Mirrors the JSON of internal/app.

export interface Status {
  connected: boolean
  mode: '' | 'usb' | 'sim'
  idn: string
  detail: string
  error: string
  reconnecting: boolean
}

export interface Measurements {
  max: number | null
  min: number | null
  pp: number | null
  top: number | null
  base: number | null
  amp: number | null
  overshoot: number | null
  preshoot: number | null
  avg: number | null
  rms: number | null
  cycAvg: number | null
  cycRms: number | null
  acRms: number | null
  freq: number | null
  period: number | null
  duty: number | null
  rise: number | null
  fall: number | null
  pWidth: number | null
  nWidth: number | null
  clipped: boolean
}

export interface Pair {
  delay: number | null
  phase: number | null
  gain: number | null
}

export interface FrameChannel {
  name: string
  on: boolean
  coupling: string
  probe: string
  voltsPerDiv: number
  offset: number // sample counts from screen centre, 25 per division
  raw: number[] | null
  meas: Measurements
}

export interface Frame {
  seq: number
  run: string
  timeDiv: number
  timeScale: string
  hOffset: number
  sampleRate: string
  acqMode: string
  depMem: string
  trig: { source: string; edge: string; sweep: string; coupling: string; level: number; levelStr: string }
  channels: FrameChannel[]
  pair: Pair | null
  fps: number
  averaged?: number // frames in the average, 0 without one
}

export interface GenState {
  wave: string
  freq: number | null
  amp: number | null
  offset: number | null
  duty: number | null
  symmetry: number | null
  output: boolean | null
}

export interface LogEntry {
  dir: 'out' | 'in' | 'err'
  text: string
  time: number
}

export interface ConsoleResult {
  reply: string
  kind: 'text' | 'data' | 'none'
  bytes: number
  ms: number
}

// A sweep steps the frequency from start to stop hertz at amp, or with mode
// 'amp' the amplitude from start to stop volts peak to peak at freq.
export interface SweepConfig {
  mode?: 'amp'
  start: number
  stop: number
  perDecade: number
  amp?: number // generator Vpp
  freq?: number // Hz
  average: number
}

export interface SweepPoint {
  freq: number
  amp: number // generator Vpp
  gain: number | null // CH2/CH1
  phase: number | null // degrees, negative when CH2 lags
  in: number | null // AC RMS, volts
  out: number | null
  // A distortion sweep: the harmonics of CH2 from the 2nd on against its
  // fundamental, and the total distortion of CH2 and CH1.
  harm: number[] | null
  thd: number | null
  thdIn: number | null
  note: string
}

export interface SweepEvent {
  mode: string
  state: 'running' | 'done' | 'stopped' | 'error'
  index: number
  total: number
  freq: number
  amp: number
  point: SweepPoint | null
  error: string
}
