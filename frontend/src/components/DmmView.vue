<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { on } from '../api'
import { MAX_READINGS, elapsed, envelope, firstAt, logCSV, niceStep, side, timeStep } from '../dmmlog'
import { eng, parseInput, stamp } from '../format'
import { sd } from '../stats'
import { clearDmmLog, dmmLog, dmmLogging, logReading, set, status } from '../state'
import Seg from './Seg.vue'

defineProps<{ disabled: boolean }>()
const emit = defineEmits<{ save: [name: string, blob: Blob] }>()

const raw = ref('')
const mode = ref(localStorage.getItem('owon.dmmMode') ?? 'dcv')
const rel = ref(false)

const MODES = [
  { id: 'dcv', sym: 'V⎓', label: 'Пост. напряжение', key: 'dmm.voltage', value: 'DC', unit: 'В' },
  { id: 'acv', sym: 'V~', label: 'Перем. напряжение', key: 'dmm.voltage', value: 'AC', unit: 'В' },
  { id: 'dca', sym: 'A⎓', label: 'Пост. ток', key: 'dmm.current', value: 'DC', unit: 'А' },
  { id: 'aca', sym: 'A~', label: 'Перем. ток', key: 'dmm.current', value: 'AC', unit: 'А' },
  { id: 'res', sym: 'Ω', label: 'Сопротивление', key: 'dmm.func', value: 'RESistance', unit: 'Ом' },
  { id: 'diod', sym: '⊳|', label: 'Диод', key: 'dmm.func', value: 'DIODe', unit: 'В' },
  { id: 'cont', sym: '•))', label: 'Прозвонка', key: 'dmm.func', value: 'CONTinuity', unit: 'Ом' },
  { id: 'cap', sym: '⊣⊢', label: 'Ёмкость', key: 'dmm.func', value: 'CAPacitance', unit: 'Ф' },
]
const modeInfo = computed(() => MODES.find((m) => m.id === mode.value) ?? MODES[0])

// The log is of one mode, as the unit comes from the mode: a new mode
// starts a new log.
if (dmmLog.value.mode !== mode.value) clearDmmLog(mode.value)

function pick(m: (typeof MODES)[number]) {
  mode.value = m.id
  localStorage.setItem('owon.dmmMode', m.id)
  clearDmmLog(m.id)
  set(m.key, m.value)
}

// :DMM:MEAS? replies with a bare number in base units, "1.2345e+00", so the
// unit comes from the mode picked here. A mode changed on the device itself
// is not known to the program.
const unit = computed(() => modeInfo.value.unit)
const value = computed(() => {
  const v = parseFloat(raw.value)
  return Number.isFinite(v) ? v : null
})
const shown = computed(() => {
  if (value.value == null) return { num: raw.value || '—', unit: '' }
  const s = eng(value.value, unit.value, 5)
  const i = s.lastIndexOf(' ')
  return { num: s.slice(0, i), unit: s.slice(i + 1) }
})

// ---- limits ----

// Limits of each mode, in its base unit, kept between runs.
type Limits = { lo: number | null; hi: number | null }
const LIMITS_KEY = 'owon.dmmLimits'
const limits = reactive<Record<string, Limits>>({})
try {
  Object.assign(limits, JSON.parse(localStorage.getItem(LIMITS_KEY) ?? '{}'))
} catch {}
watch(limits, () => localStorage.setItem(LIMITS_KEY, JSON.stringify(limits)))
const lim = computed<Limits>(() => limits[mode.value] ?? { lo: null, hi: null })
const outNow = computed(() => (value.value == null ? null : side(value.value, lim.value.lo, lim.value.hi)))

const fmtLimit = (v: number | null) => (v == null ? '' : String(+v.toPrecision(6)))

// An empty field takes the limit away; a number may have a prefix: 250m, 4.7k.
function setLimit(k: keyof Limits, e: Event) {
  const input = e.target as HTMLInputElement
  const s = input.value.trim()
  const v = s === '' ? null : parseInput(s)
  if (s === '' || v != null) limits[mode.value] = { ...lim.value, [k]: v }
  input.value = fmtLimit(lim.value[k])
}

const SOUND_KEY = 'owon.dmmSound'
const sound = ref(localStorage.getItem(SOUND_KEY) === '1')
// Turning the sound on plays it once: the user hears what to wait for, and
// the browser lets a page play sound after a press.
watch(sound, (v) => {
  localStorage.setItem(SOUND_KEY, v ? '1' : '0')
  if (v) beep()
})

let audio: AudioContext | null = null
function beep() {
  try {
    audio ??= new AudioContext()
    void audio.resume()
    const t = audio.currentTime
    for (const d of [0, 0.22]) {
      const o = audio.createOscillator(), g = audio.createGain()
      o.frequency.value = 1500
      g.gain.setValueAtTime(0.0001, t + d)
      g.gain.exponentialRampToValueAtTime(0.25, t + d + 0.01)
      g.gain.exponentialRampToValueAtTime(0.0001, t + d + 0.15)
      o.connect(g).connect(audio.destination)
      o.start(t + d)
      o.stop(t + d + 0.16)
    }
  } catch {}
}

// ---- log ----

const n = computed(() => dmmLog.value.t.length)
const stats = computed(() => {
  const s = dmmLog.value.stats
  return s ? { min: s.min, max: s.max, mean: s.mean, sd: sd(s) } : null
})
const duration = computed(() => (n.value > 1 ? (dmmLog.value.t[n.value - 1] - dmmLog.value.t[0]) / 1000 : 0))
const clock = (t: number) => new Date(t).toLocaleTimeString('ru-RU')

const WINDOWS = [
  { value: '60', label: '1 мин' },
  { value: '600', label: '10 мин' },
  { value: '3600', label: '1 ч' },
  { value: 'all', label: 'Всё', title: 'Весь журнал' },
]
const WIN_KEY = 'owon.dmmWindow'
const win = ref(localStorage.getItem(WIN_KEY) ?? 'all')
watch(win, (v) => localStorage.setItem(WIN_KEY, v))

function exportCSV() {
  const l = dmmLog.value
  emit('save', `hds272s_dmm_${stamp()}.csv`, new Blob([logCSV(l.t, l.v, unit.value, modeInfo.value.label)], { type: 'text/csv' }))
}

// ---- chart ----

const box = ref<HTMLDivElement>()
const canvas = ref<HTMLCanvasElement>()
const hover = ref<number | null>(null)
const M = { l: 78, r: 12, t: 10, b: 22 }
let colors: Record<string, string> = {}
let size = { w: 0, h: 0, dpr: 1 }
let raf = 0

function schedule() {
  if (!raf) raf = requestAnimationFrame(() => { raf = 0; draw() })
}
watch([dmmLog, win, lim], schedule)

// span is the time the chart shows. A window starts at the first reading
// until it fills, then follows the last one.
function span(l: typeof dmmLog.value): [number, number] {
  const first = l.t[0], last = l.t[l.t.length - 1]
  if (win.value === 'all') return [first, Math.max(last, first + 10_000)]
  const w = +win.value * 1000
  const t1 = Math.max(last, first + w)
  return [t1 - w, t1]
}

function draw() {
  const c = canvas.value
  if (!c) return
  const g = c.getContext('2d')!
  g.setTransform(size.dpr, 0, 0, size.dpr, 0, 0)
  g.clearRect(0, 0, size.w, size.h)
  const p = { x: M.l, y: M.t, w: Math.max(10, size.w - M.l - M.r), h: Math.max(10, size.h - M.t - M.b) }
  g.fillStyle = '#090b0f'
  g.fillRect(p.x, p.y, p.w, p.h)
  g.strokeStyle = '#2b313d'
  g.strokeRect(p.x + 0.5, p.y + 0.5, p.w - 1, p.h - 1)
  g.font = '11px Cascadia Mono, Consolas, monospace'
  g.fillStyle = '#5e6678'
  const l = dmmLog.value
  if (!l.t.length) {
    g.textAlign = 'center'
    g.textBaseline = 'middle'
    g.fillText(dmmLogging.value ? 'Ждём отсчётов с прибора' : 'Журнал на паузе', p.x + p.w / 2, p.y + p.h / 2)
    return
  }

  const [t0, t1] = span(l)
  const cols = Math.max(1, Math.floor(p.w))
  const env = envelope(l.t, l.v, t0, t1, cols)
  let lo = Infinity, hi = -Infinity
  for (let i = 0; i < cols; i++) {
    if (env.lo[i] < lo) lo = env.lo[i]
    if (env.hi[i] > hi) hi = env.hi[i]
  }
  if (!Number.isFinite(lo)) return
  const d = hi - lo
  if (d <= Math.abs(hi) * 1e-6 || d === 0) {
    const pad = Math.max(Math.abs(hi) * 1e-3, 1e-9)
    lo -= pad
    hi += pad
  } else {
    lo -= d * 0.08
    hi += d * 0.08
  }
  const xOf = (t: number) => p.x + ((t - t0) / (t1 - t0)) * p.w
  const yOf = (v: number) => p.y + ((hi - v) / (hi - lo)) * p.h

  // Grid and labels: values with as many digits as the step needs, time
  // from the first reading.
  const vs = niceStep(hi - lo, 5)
  const sig = Math.min(7, Math.max(3, Math.ceil(Math.log10(Math.max(Math.abs(lo), Math.abs(hi)) / vs)) + 1))
  const ts = timeStep((t1 - t0) / 1000, 8)
  g.lineWidth = 1
  g.strokeStyle = '#20252f'
  g.setLineDash([1, 3])
  g.beginPath()
  for (let v = Math.ceil(lo / vs) * vs; v <= hi; v += vs) line(g, p.x, yOf(v), p.x + p.w, yOf(v))
  for (let s = Math.ceil((t0 - l.t[0]) / 1000 / ts) * ts; l.t[0] + s * 1000 <= t1; s += ts) line(g, xOf(l.t[0] + s * 1000), p.y, xOf(l.t[0] + s * 1000), p.y + p.h)
  g.stroke()
  g.setLineDash([])
  g.textAlign = 'right'
  g.textBaseline = 'middle'
  for (let v = Math.ceil(lo / vs) * vs; v <= hi; v += vs) g.fillText(eng(Math.abs(v) < vs / 1e6 ? 0 : v, unit.value, sig), p.x - 6, yOf(v))
  g.textAlign = 'center'
  g.textBaseline = 'top'
  let lastX = -1e9
  for (let s = Math.ceil((t0 - l.t[0]) / 1000 / ts) * ts; l.t[0] + s * 1000 <= t1; s += ts) {
    const x = xOf(l.t[0] + s * 1000)
    if (x - lastX < 50) continue
    g.fillText(elapsed(s), Math.min(p.x + p.w - 20, Math.max(p.x + 20, x)), p.y + p.h + 6)
    lastX = x
  }

  // The readings: a column's extremes joined to the next column, unless
  // more than 5 s passed between them, as when the tab was left.
  const path = new Path2D()
  const msPerCol = (t1 - t0) / cols
  let prev = -1e9
  for (let i = 0; i < cols; i++) {
    if (Number.isNaN(env.lo[i])) continue
    const x = p.x + i + 0.5
    if ((i - prev) * msPerCol > 5000) path.moveTo(x, yOf(env.hi[i]))
    else path.lineTo(x, yOf(env.hi[i]))
    path.lineTo(x, yOf(env.lo[i]))
    prev = i
  }
  g.save()
  g.beginPath()
  g.rect(p.x, p.y, p.w, p.h)
  g.clip()
  g.strokeStyle = colors.ok
  g.lineWidth = 1.5
  g.lineJoin = 'round'
  g.stroke(path)
  // Beyond the limits the line is red, and the limits are dashed lines.
  const { lo: llo, hi: lhi } = lim.value
  if (llo != null || lhi != null) {
    g.save()
    g.beginPath()
    if (lhi != null) g.rect(p.x, p.y, p.w, yOf(lhi) - p.y)
    if (llo != null) g.rect(p.x, yOf(llo), p.w, p.y + p.h - yOf(llo))
    g.clip()
    g.strokeStyle = colors.err
    g.stroke(path)
    g.restore()
    g.strokeStyle = colors.err
    g.setLineDash([5, 4])
    g.beginPath()
    for (const v of [llo, lhi]) if (v != null && v >= lo && v <= hi) line(g, p.x, yOf(v), p.x + p.w, yOf(v))
    g.stroke()
    g.setLineDash([])
  }
  g.restore()
  // A limit off the chart is named at its edge.
  g.font = '10px Segoe UI, sans-serif'
  g.fillStyle = colors.err
  g.textAlign = 'right'
  if (lhi != null && lhi > hi) {
    g.textBaseline = 'top'
    g.fillText(`↑ верхний предел ${eng(lhi, unit.value, 4)}`, p.x + p.w - 6, p.y + 4)
  }
  if (llo != null && llo < lo) {
    g.textBaseline = 'bottom'
    g.fillText(`↓ нижний предел ${eng(llo, unit.value, 4)}`, p.x + p.w - 6, p.y + p.h - 4)
  }

  // The reading under the mouse.
  if (hover.value != null && hover.value >= p.x && hover.value <= p.x + p.w) {
    const t = t0 + ((hover.value - p.x) / p.w) * (t1 - t0)
    let i = firstAt(l.t, t)
    if (i >= l.t.length || (i > 0 && t - l.t[i - 1] < l.t[i] - t)) i--
    if (i < 0 || l.t[i] < t0) return
    const x = xOf(l.t[i]), y = yOf(l.v[i])
    g.strokeStyle = 'rgba(230,232,237,.35)'
    g.setLineDash([3, 3])
    g.beginPath()
    line(g, x, p.y, x, p.y + p.h)
    g.stroke()
    g.setLineDash([])
    g.fillStyle = colors.ok
    g.beginPath()
    g.arc(x, y, 3, 0, 2 * Math.PI)
    g.fill()
    const rows = [`${elapsed((l.t[i] - l.t[0]) / 1000)} · ${clock(l.t[i])}`, eng(l.v[i], unit.value, 5)]
    g.font = '12px Cascadia Mono, Consolas, monospace'
    const w = 170, h = rows.length * 17 + 8
    const bx = x + 12 + w > p.x + p.w ? x - 12 - w : x + 12
    g.fillStyle = 'rgba(20,23,30,.92)'
    g.strokeStyle = '#323947'
    g.beginPath()
    g.roundRect(bx, p.y + 8, w, h, 6)
    g.fill()
    g.stroke()
    g.textAlign = 'right'
    g.textBaseline = 'middle'
    rows.forEach((txt, k) => {
      g.fillStyle = k ? colors.ok : '#e6e8ed'
      g.fillText(txt, bx + w - 9, p.y + 21 + k * 17)
    })
  }
}

function line(g: CanvasRenderingContext2D, x1: number, y1: number, x2: number, y2: number) {
  g.moveTo(Math.round(x1) + 0.5, Math.round(y1) + 0.5)
  g.lineTo(Math.round(x2) + 0.5, Math.round(y2) + 0.5)
}

function move(e: MouseEvent) {
  hover.value = e.clientX - canvas.value!.getBoundingClientRect().left
  schedule()
}

let off: (() => void) | undefined
let ro: ResizeObserver | undefined
onMounted(() => {
  const cs = getComputedStyle(document.documentElement)
  colors = { ok: cs.getPropertyValue('--ok').trim(), err: cs.getPropertyValue('--err').trim() }
  ro = new ResizeObserver(() => {
    const b = box.value!.getBoundingClientRect()
    size = { w: b.width, h: b.height, dpr: window.devicePixelRatio || 1 }
    canvas.value!.width = Math.round(b.width * size.dpr)
    canvas.value!.height = Math.round(b.height * size.dpr)
    draw()
  })
  ro.observe(box.value!)
  off = on('dmm', (d: { raw: string; time: number }) => {
    raw.value = d.raw
    const v = value.value
    if (v != null && logReading(d.time, v, side(v, lim.value.lo, lim.value.hi)) && sound.value) beep()
  })
})
onBeforeUnmount(() => {
  off?.()
  ro?.disconnect()
  cancelAnimationFrame(raf)
  void audio?.close()
})
</script>

<template>
  <div class="dmm">
    <div class="note">
      Пока открыта эта вкладка, прибор показывает экран мультиметра. На вкладке осциллографа он вернётся к осциллографу.
      Прибор присылает только число, без единицы. Единица берётся из режима, выбранного справа, поэтому выберите тот же режим, что на приборе.
    </div>

    <div class="main">
      <div class="readout" :class="{ stale: !status.connected, out: outNow }">
        <div class="value mono">{{ shown.num }}</div>
        <div class="unit">{{ shown.unit }}</div>
        <div class="raw mono">
          ответ прибора: {{ raw || 'нет данных' }}
          <span v-if="outNow" class="outtxt">{{ outNow === 'hi' ? 'выше верхнего предела' : 'ниже нижнего предела' }}</span>
        </div>
      </div>

      <div class="side">
        <div class="modes">
          <button
            v-for="m in MODES"
            :key="m.id"
            class="mode"
            :class="{ on: mode === m.id }"
            :disabled="disabled"
            :title="n ? 'Смена режима начнёт новый журнал' : ''"
            @click="pick(m)"
          >
            <span class="sym">{{ m.sym }}</span>
            <span class="lbl">{{ m.label }}</span>
          </button>
        </div>
        <div class="actions">
          <button :disabled="disabled" @click="set('dmm.auto', '')">Авто диапазон</button>
          <button :disabled="disabled" @click="set('dmm.range', '')">Следующий диапазон</button>
          <button :class="{ primary: rel }" :disabled="disabled" @click="rel = !rel; set('dmm.rel', rel ? 'ON' : 'OFF')">
            Относительный (REL)
          </button>
        </div>
      </div>
    </div>

    <section class="log">
      <div class="bar">
        <h3>Регистратор</h3>
        <button v-if="dmmLogging" class="ghost" title="Не записывать отсчёты в журнал; пределы проверяются и на паузе" @click="dmmLogging = false">⏸ Пауза</button>
        <button v-else :disabled="n >= MAX_READINGS" :title="n >= MAX_READINGS ? 'Журнал полон' : 'Записывать отсчёты дальше'" @click="dmmLogging = true">● Продолжить</button>
        <Seg v-model="win" :options="WINDOWS" />
        <span class="grow" />
        <button class="ghost" :disabled="!n" title="Сохранить журнал в CSV" @click="exportCSV">CSV</button>
        <button class="ghost" :disabled="!n && !dmmLog.exits" @click="clearDmmLog(mode)">Очистить</button>
      </div>
      <div ref="box" class="chart" @mousemove="move" @mouseleave="hover = null; schedule()">
        <canvas ref="canvas" />
      </div>
      <div class="stats mono">
        <span>{{ n }} отсч. · {{ elapsed(duration) }}</span>
        <template v-if="stats">
          <span>мин <b>{{ eng(stats.min, unit, 5) }}</b></span>
          <span>макс <b>{{ eng(stats.max, unit, 5) }}</b></span>
          <span>ср. <b>{{ eng(stats.mean, unit, 5) }}</b></span>
          <span>σ <b>{{ eng(stats.sd, unit, 3) }}</b></span>
        </template>
        <span v-if="n >= MAX_READINGS" class="warn">журнал полон ({{ MAX_READINGS.toLocaleString('ru-RU') }})</span>
      </div>
      <div class="bar limits">
        <span class="lbl">Пределы</span>
        <label>ниже <input class="num mono" :value="fmtLimit(lim.lo)" placeholder="нет" :title="`Нижний предел, ${unit}; можно с приставкой: 250m, 4.7k. Пусто — без предела`" @change="setLimit('lo', $event)" /></label>
        <label>выше <input class="num mono" :value="fmtLimit(lim.hi)" placeholder="нет" :title="`Верхний предел, ${unit}`" @change="setLimit('hi', $event)" /></label>
        <span class="dim">{{ unit }}</span>
        <label title="Двойной сигнал, когда отсчёт выходит за предел"><input v-model="sound" type="checkbox" /> звук</label>
        <span v-if="dmmLog.exits" class="exits">выходов за пределы: {{ dmmLog.exits }}, последний {{ clock(dmmLog.lastExit) }}</span>
      </div>
    </section>
  </div>
</template>

<style scoped>
.dmm { display: flex; flex-direction: column; gap: 12px; padding: 14px; height: 100%; overflow: auto; }
.note {
  font-size: 12px;
  color: var(--muted);
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: var(--radius);
  padding: 8px 12px;
}
.main { display: flex; gap: 14px; align-items: flex-start; flex-wrap: wrap; }
.readout {
  flex: 1 1 520px;
  background: #090b0f;
  border: 1px solid var(--line);
  border-radius: 12px;
  padding: 24px 28px 18px;
  display: grid;
  grid-template-columns: auto 1fr;
  align-items: baseline;
  column-gap: 16px;
}
.readout.stale { opacity: .55; }
.value { font-size: 88px; line-height: 1; color: var(--ok); letter-spacing: -.02em; }
.readout.out { border-color: #6b2a2f; }
.readout.out .value { color: var(--err); }
.unit { font-size: 34px; color: var(--muted); }
.raw { grid-column: 1 / -1; font-size: 12px; color: var(--dim); margin-top: 10px; user-select: text; }
.outtxt { color: var(--err); margin-left: 12px; }
.side { flex: 0 1 380px; display: flex; flex-direction: column; gap: 12px; }
.modes { display: grid; grid-template-columns: repeat(4, 1fr); gap: 6px; }
.mode { display: flex; flex-direction: column; align-items: center; gap: 4px; padding: 10px 4px; }
.mode .sym { font-size: 18px; font-family: var(--mono); }
.mode .lbl { font-size: 11px; color: var(--muted); text-align: center; }
.mode.on { background: var(--panel-3); border-color: #4a5366; }
.mode.on .sym { color: var(--ok); }
.actions { display: flex; flex-wrap: wrap; gap: 6px; }
.log {
  display: flex;
  flex-direction: column;
  gap: 8px;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  padding: 10px 12px;
  background: var(--panel);
}
.bar { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; font-size: 12px; }
h3 { margin: 0 6px 0 0; font-size: 12px; font-weight: 600; color: var(--muted); text-transform: uppercase; letter-spacing: .06em; }
.grow { flex: 1; }
.chart { position: relative; height: 240px; min-height: 160px; }
.chart canvas { position: absolute; inset: 0; width: 100%; height: 100%; }
.stats { display: flex; gap: 16px; flex-wrap: wrap; font-size: 12px; color: var(--muted); }
.stats b { color: var(--text); font-weight: 500; }
.limits .lbl { color: var(--muted); margin-right: 4px; }
.limits label { display: flex; align-items: center; gap: 4px; color: var(--muted); cursor: pointer; }
.limits .num { width: 84px; font-size: 12px; padding: 3px 6px; }
.exits { color: var(--err); }
.warn { color: var(--warn); }
</style>
