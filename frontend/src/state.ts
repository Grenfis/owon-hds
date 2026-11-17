import { computed, reactive, ref, shallowRef, triggerRef, watch } from 'vue'
import { call, on } from './api'
import { DEFAULT_CURSORS, type Cursors } from './cursors'
import { TIMEBASES, closest, probeFactor, vdivOptions } from './format'
import { sweepAt, type GenSweep } from './gensweep'
import { fitMath, mathOf, type MathOp } from './math'
import type { PersistMode } from './persist'
import { MAX_FRAMES, type RecFrame } from './record'
import { isReference, refFromFrame, type Reference } from './ref'
import type { Frame, GenState, LogEntry, Status, SweepConfig, SweepEvent, SweepPoint } from './types'

// Screen cursors, kept between runs.
const CURSORS_KEY = 'owon.cursors'
const freshCursors = (): Cursors => ({ ...DEFAULT_CURSORS, t: [...DEFAULT_CURSORS.t], v: [...DEFAULT_CURSORS.v] })
export const cursors = reactive<Cursors>(freshCursors())
try {
  const c = JSON.parse(localStorage.getItem(CURSORS_KEY) ?? '{}')
  if (Array.isArray(c.t) && c.t.length === 2 && Array.isArray(c.v) && c.v.length === 2) Object.assign(cursors, c)
} catch {}
watch(cursors, () => localStorage.setItem(CURSORS_KEY, JSON.stringify(cursors)), { deep: true })

// resetCursors puts the cursors back to their places and keeps the mode.
export function resetCursors() {
  const { t, v } = freshCursors()
  Object.assign(cursors, { t, v })
}

export const status = ref<Status>({ connected: false, mode: '', idn: '', detail: '', error: '', reconnecting: false })
export const frame = shallowRef<Frame | null>(null)

// The math channel and the screen mode, kept between runs. The math channel
// is computed from every frame.
const DISPLAY_KEY = 'owon.display'
export const display = reactive({ math: 'off' as MathOp, mathScale: 1, mathOffset: 0, xy: false, ref: true, avg: 1, persist: 'off' as PersistMode })
try {
  const d = JSON.parse(localStorage.getItem(DISPLAY_KEY) ?? '{}')
  if (d && typeof d === 'object') Object.assign(display, d)
} catch {}
watch(display, () => localStorage.setItem(DISPLAY_KEY, JSON.stringify(display)))
export const mathTrace = computed(() => mathOf(frame.value, display.math))

// The Go side averages the frames; it is told the count at start and on
// every change.
const sendAverage = () => call('SetAverage', display.avg).catch((e) => (lastError.value = String(e.message ?? e)))
watch(() => display.avg, sendAverage)

// Bumped to wipe the persistence traces.
export const persistEpoch = ref(0)

// Recording of frames and its playback. While a recording is shown, the
// live frames still update the settings, but not the screen.
export const rec = reactive({ recording: false, viewing: false, index: 0 })
export const recFrames = shallowRef<RecFrame[]>([])
let liveFrame: Frame | null = null

export function recordFrame(f: Frame) {
  if (recFrames.value.length >= MAX_FRAMES) {
    rec.recording = false
    return
  }
  recFrames.value.push({ t: Date.now(), frame: f })
  triggerRef(recFrames)
}

export function showRecorded(i: number) {
  const r = recFrames.value
  if (!r.length) return
  rec.index = Math.max(0, Math.min(r.length - 1, i))
  rec.viewing = true
  rec.recording = false
  frame.value = r[rec.index].frame
}

export function stopViewing() {
  if (!rec.viewing) return
  rec.viewing = false
  frame.value = liveFrame
}

// fitMathToScreen picks the scale and the offset that show the whole math
// trace.
export function fitMathToScreen() {
  const m = mathTrace.value
  if (!m) return
  const { scale, offset } = fitMath(m.v)
  Object.assign(display, { mathScale: scale, mathOffset: offset })
}

// The reference waveform, kept between runs when it fits in the storage.
const REF_KEY = 'owon.ref'
export const reference = shallowRef<Reference | null>(null)
try {
  const r = JSON.parse(localStorage.getItem(REF_KEY) ?? 'null')
  if (isReference(r)) reference.value = r
} catch {}
watch(reference, (r) => {
  try {
    if (r) localStorage.setItem(REF_KEY, JSON.stringify(r))
    else localStorage.removeItem(REF_KEY)
  } catch {}
})

// saveReference keeps the channels and the math trace on the screen now.
export function saveReference() {
  const f = frame.value
  if (!f) return
  const r = refFromFrame(f, mathTrace.value?.v ?? null, display.math, `кадр ${new Date().toLocaleTimeString('ru-RU')}`)
  if (r) {
    reference.value = r
    display.ref = true
  }
}
export const lastError = ref('')
export const logs = ref<LogEntry[]>([])

// Scope settings as the controls show them. They follow the device through
// the frame header, except for a setting the user has just changed: that one
// keeps the user's value for a moment, so the control does not jump back
// while the device has not applied it yet.
export const scope = reactive<Record<string, string>>({
  'ch1.display': 'ON', 'ch1.coupling': 'DC', 'ch1.probe': '1X', 'ch1.scale': '1.00V', 'ch1.offset': '0',
  'ch2.display': 'ON', 'ch2.coupling': 'DC', 'ch2.probe': '1X', 'ch2.scale': '1.00V', 'ch2.offset': '0',
  'hor.scale': '1.0ms', 'hor.offset': '0',
  'trig.source': 'CH1', 'trig.edge': 'RISE', 'trig.sweep': 'AUTO', 'trig.level': '0', 'trig.coupling': 'DC',
  'acq.mode': 'SAMPle', 'acq.depmem': '4K', run: 'RUN',
})

// Generator settings live here only: the device does not report them in the
// frame, so they are read once on connect and then kept by the UI.
export const gen = reactive({
  wave: 'SINE', freq: 1000, amp: 1, offset: 0, duty: 50, symmetry: 50, output: false,
})

const holdUntil = new Map<string, number>()

// set changes a setting on the device. Values are queued in Go and collapse
// into the last one, so it is fine to call this on every mouse move.
export function set(key: string, value: string | number, holdMs = 1200) {
  const v = String(value)
  if (key in scope) {
    scope[key] = v
    holdUntil.set(key, performance.now() + holdMs)
  }
  call('Set', key, v).catch((e) => (lastError.value = String(e.message ?? e)))
}

// setTrigLevel sends the trigger level in millivolts and keeps it in volts
// in the store, the unit the frame header is parsed into.
export function setTrigLevel(volts: number) {
  set('trig.level', `${(volts * 1000).toFixed(1)}mv`)
  scope['trig.level'] = String(volts)
}

type GenKey = keyof typeof gen

export function setGen<K extends GenKey>(key: K, value: (typeof gen)[K]) {
  gen[key] = value
  const v = typeof value === 'boolean' ? (value ? 'ON' : 'OFF') : String(value)
  set(`gen.${key}`, v)
}

function follow(key: string, value: string) {
  if ((holdUntil.get(key) ?? 0) > performance.now()) return
  if (scope[key] !== value) scope[key] = value
}

const SWEEPS: Record<string, string> = { auto: 'AUTO', normal: 'NORMal', norm: 'NORMal', single: 'SINGle', sing: 'SINGle' }

function applyFrame(f: Frame) {
  for (const ch of f.channels) {
    const k = ch.name.toLowerCase()
    if (k !== 'ch1' && k !== 'ch2') continue
    follow(`${k}.display`, ch.on ? 'ON' : 'OFF')
    follow(`${k}.coupling`, ch.coupling.toUpperCase())
    follow(`${k}.probe`, ch.probe.toUpperCase())
    if (ch.voltsPerDiv > 0) {
      const opts = vdivOptions(probeFactor(ch.probe))
      follow(`${k}.scale`, opts[closest(opts, ch.voltsPerDiv)].cmd)
    }
    follow(`${k}.offset`, (ch.offset / 25).toFixed(2))
  }
  if (f.timeDiv > 0) follow('hor.scale', TIMEBASES[closest(TIMEBASES, f.timeDiv)].cmd)
  follow('hor.offset', String(f.hOffset))
  if (f.trig.source) follow('trig.source', f.trig.source.toUpperCase())
  if (f.trig.edge) follow('trig.edge', f.trig.edge.startsWith('f') ? 'FALL' : 'RISE')
  if (f.trig.sweep) follow('trig.sweep', SWEEPS[f.trig.sweep] ?? 'AUTO')
  if (f.trig.coupling) follow('trig.coupling', f.trig.coupling.toUpperCase())
  if (f.trig.levelStr) follow('trig.level', String(f.trig.level))
  if (f.acqMode) follow('acq.mode', f.acqMode.startsWith('p') ? 'PEAK' : 'SAMPle')
  if (f.depMem) follow('acq.depmem', f.depMem.toUpperCase())
  follow('run', f.run === 'stop' ? 'STOP' : 'RUN')
}

export async function readGen() {
  const g = await call<GenState>('ReadGen')
  if (g.wave) gen.wave = g.wave
  if (g.freq != null) gen.freq = g.freq
  if (g.amp != null) gen.amp = g.amp
  if (g.offset != null) gen.offset = g.offset
  if (g.duty != null) gen.duty = g.duty
  if (g.symmetry != null) gen.symmetry = g.symmetry
  if (g.output != null) gen.output = g.output
}

// The last frequency response sweep and the last distortion sweep. They live
// here rather than in their panels, so that hiding a panel keeps the result.
function newSweep() {
  return reactive({
    state: 'idle' as 'idle' | SweepEvent['state'],
    index: 0,
    total: 0,
    freq: 0,
    amp: 0,
    start: 0,
    stop: 0,
    points: [] as SweepPoint[],
    error: '',
  })
}
export const sweep = newSweep()
export const distortion = newSweep()
const sweepOf = (mode?: string) => (mode === 'amp' ? distortion : sweep)
export const sweeping = computed(() => sweep.state === 'running' || distortion.state === 'running')

// The software sweep of the generator frequency. Its settings are kept
// between runs; a step goes out every 100 ms, about as often as the loop
// reads frames. A sweep of the Go side owns the generator, and so does a
// lost connection: both stop it.
const GEN_SWEEP_KEY = 'owon.genSweep'
export const genSweep = reactive({ f1: 100, f2: 100e3, time: 10, log: true, repeat: false, running: false, at: 0 })
try {
  const s = JSON.parse(localStorage.getItem(GEN_SWEEP_KEY) ?? '{}')
  for (const k of ['f1', 'f2', 'time'] as const) if (typeof s[k] === 'number') genSweep[k] = s[k]
  for (const k of ['log', 'repeat'] as const) if (typeof s[k] === 'boolean') genSweep[k] = s[k]
} catch {}
watch(
  () => [genSweep.f1, genSweep.f2, genSweep.time, genSweep.log, genSweep.repeat],
  () => {
    const { f1, f2, time, log, repeat } = genSweep
    localStorage.setItem(GEN_SWEEP_KEY, JSON.stringify({ f1, f2, time, log, repeat }))
  },
)
let genTimer = 0, genStart = 0, freqBefore = 0

export function startGenSweep() {
  if (genSweep.running) return
  freqBefore = gen.freq
  genSweep.running = true
  genStart = performance.now()
  genStep()
  genTimer = window.setInterval(genStep, 100)
}

function genStep() {
  const t = (performance.now() - genStart) / 1000
  const f = sweepAt(genSweep as GenSweep, t)
  if (f == null) {
    stopGenSweep()
    return
  }
  genSweep.at = (t % genSweep.time) / genSweep.time
  if (f !== gen.freq) setGen('freq', f)
}

// stopGenSweep puts back the frequency there was before the sweep, unless
// the generator is no longer the sweep's to change.
export function stopGenSweep(restore = true) {
  if (!genSweep.running) return
  clearInterval(genTimer)
  genSweep.running = false
  genSweep.at = 0
  if (restore) setGen('freq', freqBefore)
}
watch(sweeping, (s) => s && stopGenSweep(false))

export async function startSweep(cfg: SweepConfig) {
  const s = sweepOf(cfg.mode)
  Object.assign(s, { state: 'running', index: 0, total: 0, freq: 0, amp: 0, start: cfg.start, stop: cfg.stop, points: [], error: '' })
  try {
    await call('StartSweep', cfg)
  } catch (e: any) {
    s.state = 'error'
    s.error = String(e.message ?? e)
  }
}

export function stopSweep() {
  call('StopSweep').catch(() => {})
}

export async function connect(mode: 'usb' | 'sim') {
  lastError.value = ''
  try {
    status.value = await call<Status>('Connect', mode)
    localStorage.setItem('owon.mode', mode)
    await readGen().catch(() => {})
  } catch (e: any) {
    lastError.value = String(e.message ?? e)
  }
}

export async function disconnect() {
  await call('Disconnect')
  frame.value = null
}

export function init() {
  sendAverage()
  on('status', (s: Status) => {
    const was = status.value.connected
    status.value = s
    if (!s.connected) {
      liveFrame = null
      stopGenSweep(false)
    }
    if (!s.connected && !rec.viewing) frame.value = null
    // Also after an automatic reconnect: the generator may have been changed
    // on the device meanwhile.
    else if (!was) readGen().catch(() => {})
  })
  on('frame', (f: Frame) => {
    applyFrame(f)
    liveFrame = f
    if (rec.recording) recordFrame(f)
    if (!rec.viewing) frame.value = f
  })
  on('sweep', (e: SweepEvent) => {
    const s = sweepOf(e.mode)
    Object.assign(s, { state: e.state, index: e.index, total: e.total, freq: e.freq, amp: e.amp })
    if (e.point) s.points.push(e.point)
    if (e.state === 'error') s.error = e.error
    // The sweep put the generator back; the panel reads it again to be sure.
    if (e.state !== 'running') readGen().catch(() => {})
  })
  on('log', (e: LogEntry) => {
    logs.value.push(e)
    if (logs.value.length > 500) logs.value.splice(0, logs.value.length - 500)
  })
  call<Status>('GetStatus')
    .then((s) => {
      status.value = s
      if (s.connected) return readGen()
    })
    .catch(() => {})
}
