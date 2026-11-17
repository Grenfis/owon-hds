<script setup lang="ts">
// Frequency response of a circuit: the generator feeds its input and CH1,
// CH2 sees its output. The sweep runs in Go; this panel sets it up and draws
// the gain and the phase of CH2 against CH1.
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { bodePoints, cutoffs, maximum } from '../bode'
import { eng, parseInput, signed, stamp } from '../format'
import { startSweep, stopSweep, sweep, sweeping } from '../state'
import type { SweepConfig } from '../types'
import Seg from './Seg.vue'

const props = defineProps<{ online: boolean }>()
const emit = defineEmits<{ save: [name: string, blob: Blob] }>()

const DIVY = 8
const M = { l: 58, r: 46, t: 10, b: 24 }

const SETTINGS_KEY = 'owon.bode'
const opts = reactive({ start: '100', stop: '1M', perDecade: '10', amp: '1', average: '1' })
try {
  Object.assign(opts, JSON.parse(localStorage.getItem(SETTINGS_KEY) ?? '{}'))
} catch {}
watch(opts, () => localStorage.setItem(SETTINGS_KEY, JSON.stringify(opts)))

const PER_DECADE = ['5', '10', '20'].map((v) => ({ value: v, label: v, title: `${v} точек на декаду частоты` }))
const AVERAGES = [
  { value: '1', label: '1', title: 'Один экран на точку' },
  { value: '2', label: '2', title: 'Среднее двух экранов на точку' },
  { value: '4', label: '4', title: 'Среднее четырёх экранов: меньше шума, но дольше' },
]

const start = computed(() => parseInput(opts.start))
const stop = computed(() => parseInput(opts.stop))
const amp = computed(() => parseInput(opts.amp))
const startOk = computed(() => start.value != null && start.value >= 10 && start.value < 25e6)
const stopOk = computed(() => stop.value != null && stop.value <= 25e6 && (!startOk.value || stop.value > start.value!))
const ampOk = computed(() => amp.value != null && amp.value >= 0.01 && amp.value <= 5)
const config = computed<SweepConfig | null>(() =>
  startOk.value && stopOk.value && ampOk.value
    ? { start: start.value!, stop: stop.value!, perDecade: +opts.perDecade, amp: amp.value!, average: +opts.average }
    : null,
)
const count = computed(() => {
  const c = config.value
  return c ? Math.max(2, Math.ceil(Math.log10(c.stop / c.start) * c.perDecade - 1e-9) + 1) : 0
})
const busy = computed(() => sweep.state === 'running')

function run() {
  if (config.value) startSweep(config.value)
}

// ---- results ----

const bp = computed(() => bodePoints(sweep.points))
const cut = computed(() => cutoffs(bp.value))
const peak = computed(() => maximum(bp.value))
const missing = computed(() => sweep.points.filter((p) => p.gain == null))

function deg(v: number) {
  return `${v.toFixed(0).replace('-', '−')}°`
}

// ---- drawing ----

const box = ref<HTMLDivElement>()
const canvas = ref<HTMLCanvasElement>()
const hover = ref<number | null>(null) // mouse x
let colors = { gain: '#6ea8ff', phase: '#f5a524' }
let size = { w: 0, h: 0, dpr: 1 }
let raf = 0

function plotRect() {
  return { x: M.l, y: M.t, w: Math.max(10, size.w - M.l - M.r), h: Math.max(10, size.h - M.t - M.b) }
}

function xRange(): [number, number] {
  if (sweep.start > 0 && sweep.stop > sweep.start) return [sweep.start, sweep.stop]
  if (config.value) return [config.value.start, config.value.stop]
  return [100, 1e6]
}

// axis picks the step and the top of a vertical axis that holds lo…hi in
// DIVY divisions.
function axis(lo: number, hi: number, steps: number[]) {
  for (const step of steps) {
    const top = Math.ceil(hi / step) * step
    if (top - DIVY * step <= lo) return { top, step }
  }
  const step = steps[steps.length - 1]
  return { top: Math.ceil(hi / step) * step, step }
}

function schedule() {
  if (!raf) raf = requestAnimationFrame(() => { raf = 0; draw() })
}

function draw() {
  const c = canvas.value
  if (!c) return
  const g = c.getContext('2d')!
  g.setTransform(size.dpr, 0, 0, size.dpr, 0, 0)
  g.clearRect(0, 0, size.w, size.h)
  const p = plotRect()
  g.fillStyle = '#090b0f'
  g.fillRect(p.x, p.y, p.w, p.h)

  const [f1, f2] = xRange()
  const xOf = (f: number) => p.x + (Math.log(f / f1) / Math.log(f2 / f1)) * p.w
  const b = bp.value
  const dbs = b.map((q) => q.db), phs = b.map((q) => q.phase)
  const ga = axis(b.length ? Math.min(...dbs) : -40, b.length ? Math.max(...dbs) + 0.5 : 0, [1, 2, 5, 10, 20])
  const pa = axis(b.length ? Math.min(...phs) : -90, b.length ? Math.max(...phs) + 2 : 0, [15, 30, 45, 90, 180])
  const yDb = (v: number) => p.y + ((ga.top - v) / (DIVY * ga.step)) * p.h
  const yPh = (v: number) => p.y + ((pa.top - v) / (DIVY * pa.step)) * p.h

  // Grid: decades solid, the 2…9 lines between them dotted.
  const decades = Math.log10(f2 / f1)
  const ticks: { f: number; major: boolean; label: boolean }[] = []
  for (let k = Math.floor(Math.log10(f1)); k <= Math.ceil(Math.log10(f2)); k++) {
    for (let m = 1; m <= 9; m++) {
      const f = m * 10 ** k
      if (f < f1 * 0.999 || f > f2 * 1.001) continue
      ticks.push({ f, major: m === 1, label: m === 1 || (decades <= 2.5 && (m === 2 || m === 5)) })
    }
  }
  g.lineWidth = 1
  g.strokeStyle = '#20252f'
  g.setLineDash([1, 3])
  g.beginPath()
  for (let i = 1; i < DIVY; i++) line(g, p.x, p.y + (i * p.h) / DIVY, p.x + p.w, p.y + (i * p.h) / DIVY)
  for (const t of ticks) if (!t.major) line(g, xOf(t.f), p.y, xOf(t.f), p.y + p.h)
  g.stroke()
  g.setLineDash([])
  g.strokeStyle = '#2a303c'
  g.beginPath()
  for (const t of ticks) if (t.major) line(g, xOf(t.f), p.y, xOf(t.f), p.y + p.h)
  g.stroke()
  g.strokeStyle = '#2b313d'
  g.strokeRect(p.x + 0.5, p.y + 0.5, p.w - 1, p.h - 1)

  g.font = '11px Cascadia Mono, Consolas, monospace'
  g.fillStyle = '#5e6678'
  g.textBaseline = 'top'
  g.textAlign = 'center'
  let lastX = -1e9
  for (const t of ticks) {
    const x = xOf(t.f)
    if (!t.label || x - lastX < 56) continue
    g.fillText(eng(t.f, 'Гц', 3, true), Math.min(p.x + p.w - 24, Math.max(p.x + 24, x)), p.y + p.h + 6)
    lastX = x
  }
  // Every other line is labelled, the ones at even multiples of the step,
  // so that 0 dB and 0° get a label.
  g.textBaseline = 'middle'
  for (let i = 0; i <= DIVY; i++) {
    const y = p.y + (i * p.h) / DIVY
    const v = ga.top - i * ga.step, w = pa.top - i * pa.step
    if (Math.round(v / ga.step) % 2 === 0) {
      g.textAlign = 'right'
      g.fillStyle = colors.gain
      g.fillText(`${signed(v, 0).replace('+0', '0')} дБ`, p.x - 6, y)
    }
    if (Math.round(w / pa.step) % 2 === 0) {
      g.textAlign = 'left'
      g.fillStyle = colors.phase
      g.fillText(deg(w), p.x + p.w + 6, y)
    }
  }

  g.save()
  g.beginPath()
  g.rect(p.x, p.y, p.w, p.h)
  g.clip()

  // The frequency being measured.
  if (busy.value && sweep.freq > 0) {
    g.fillStyle = 'rgba(110,168,255,.10)'
    g.fillRect(xOf(sweep.freq) - 3, p.y, 6, p.h)
  }
  // The -3 dB points.
  g.font = '10px Segoe UI, sans-serif'
  g.textAlign = 'left'
  g.textBaseline = 'top'
  for (const k of cut.value?.list ?? []) {
    const x = xOf(k.freq)
    g.strokeStyle = 'rgba(230,232,237,.35)'
    g.setLineDash([4, 3])
    g.beginPath()
    line(g, x, p.y, x, p.y + p.h)
    g.stroke()
    g.setLineDash([])
    g.fillStyle = '#8f97a8'
    g.fillText('−3 дБ', x + 4, p.y + 4)
  }

  const trace = (ys: number[], color: string, width: number) => {
    g.strokeStyle = color
    g.lineWidth = width
    g.beginPath()
    b.forEach((q, i) => (i ? g.lineTo(xOf(q.freq), ys[i]) : g.moveTo(xOf(q.freq), ys[i])))
    g.stroke()
    g.fillStyle = color
    for (let i = 0; i < b.length; i++) {
      g.beginPath()
      g.arc(xOf(b[i].freq), ys[i], 2.2, 0, Math.PI * 2)
      g.fill()
    }
  }
  trace(phs.map(yPh), colors.phase, 1.2)
  trace(dbs.map(yDb), colors.gain, 1.8)
  g.restore()

  if (hover.value != null && b.length) drawHover(g, p, xOf, yDb, yPh)
}

function drawHover(
  g: CanvasRenderingContext2D, p: ReturnType<typeof plotRect>,
  xOf: (f: number) => number, yDb: (v: number) => number, yPh: (v: number) => number,
) {
  const mx = hover.value!
  if (mx < p.x || mx > p.x + p.w) return
  const b = bp.value
  let k = 0
  for (let i = 1; i < b.length; i++) if (Math.abs(xOf(b[i].freq) - mx) < Math.abs(xOf(b[k].freq) - mx)) k = i
  const q = b[k]
  const sp = sweep.points.find((s) => s.freq === q.freq)
  const x = xOf(q.freq)
  g.strokeStyle = 'rgba(230,232,237,.35)'
  g.setLineDash([3, 3])
  g.beginPath()
  line(g, x, p.y, x, p.y + p.h)
  g.stroke()
  g.setLineDash([])
  for (const [y, color] of [[yDb(q.db), colors.gain], [yPh(q.phase), colors.phase]] as const) {
    g.fillStyle = color
    g.beginPath()
    g.arc(x, y, 4, 0, Math.PI * 2)
    g.fill()
  }
  const rows: [string, string][] = [
    [eng(q.freq, 'Гц', 4), '#e6e8ed'],
    [`${signed(q.db, 2)} дБ`, colors.gain],
    [`×${(10 ** (q.db / 20)).toPrecision(3)}`, '#8f97a8'],
    [`${q.phase.toFixed(1).replace('-', '−')}°`, colors.phase],
  ]
  if (sp?.in != null && sp.out != null) rows.push([`${eng(sp.in, 'В', 3)} → ${eng(sp.out, 'В', 3)}`, '#8f97a8'])
  g.font = '12px Cascadia Mono, Consolas, monospace'
  const w = 168, h = rows.length * 17 + 8
  const bx = x + 12 + w > p.x + p.w ? x - 12 - w : x + 12
  g.fillStyle = 'rgba(20,23,30,.92)'
  g.strokeStyle = '#323947'
  g.beginPath()
  g.roundRect(bx, p.y + 8, w, h, 6)
  g.fill()
  g.stroke()
  g.textAlign = 'right'
  g.textBaseline = 'middle'
  rows.forEach(([t, color], i) => {
    g.fillStyle = color
    g.fillText(t, bx + w - 9, p.y + 21 + i * 17)
  })
}

function line(g: CanvasRenderingContext2D, x1: number, y1: number, x2: number, y2: number) {
  g.moveTo(Math.round(x1) + 0.5, Math.round(y1) + 0.5)
  g.lineTo(Math.round(x2) + 0.5, Math.round(y2) + 0.5)
}

function move(e: MouseEvent) {
  hover.value = e.clientX - canvas.value!.getBoundingClientRect().left
}

// ---- export ----

function exportCSV() {
  const lines = ['# OWON HDS272S frequency response, CH2 against CH1', 'f_Hz,gain,gain_dB,phase_deg,in_Vrms,out_Vrms,note']
  const unwrapped = new Map(bp.value.map((q) => [q.freq, q.phase]))
  const n = (v: number | null | undefined, p = 5) => (v == null ? '' : v.toPrecision(p))
  for (const s of sweep.points) {
    const db = s.gain != null && s.gain > 0 ? (20 * Math.log10(s.gain)).toFixed(3) : ''
    const ph = unwrapped.get(s.freq) ?? s.phase
    lines.push([n(s.freq, 6), n(s.gain), db, ph == null ? '' : ph.toFixed(2), n(s.in), n(s.out), s.note].join(','))
  }
  emit('save', `hds272s_bode_${stamp()}.csv`, new Blob([lines.join('\n') + '\n'], { type: 'text/csv' }))
}

async function exportPNG() {
  const b = await new Promise<Blob | null>((res) => canvas.value!.toBlob(res, 'image/png'))
  if (b) emit('save', `hds272s_bode_${stamp()}.png`, b)
}

let ro: ResizeObserver | undefined
onMounted(() => {
  const cs = getComputedStyle(document.documentElement)
  colors = { gain: cs.getPropertyValue('--accent').trim() || colors.gain, phase: cs.getPropertyValue('--warn').trim() || colors.phase }
  ro = new ResizeObserver(() => {
    const r = box.value!.getBoundingClientRect()
    size = { w: r.width, h: r.height, dpr: window.devicePixelRatio || 1 }
    canvas.value!.width = Math.round(r.width * size.dpr)
    canvas.value!.height = Math.round(r.height * size.dpr)
    draw()
  })
  ro.observe(box.value!)
})
onBeforeUnmount(() => {
  ro?.disconnect()
  cancelAnimationFrame(raf)
})
watch([() => sweep.points.length, () => sweep.freq, () => sweep.state, hover, () => opts.start, () => opts.stop], schedule)
</script>

<template>
  <div class="bode">
    <div class="bar">
      <span class="title" title="Амплитудно- и фазочастотная характеристика: усиление и фаза CH2 относительно CH1">АЧХ</span>
      <span class="lbl">от</span>
      <input v-model="opts.start" type="text" class="num" :class="{ bad: !startOk }" :disabled="busy" title="Начальная частота, от 10 Гц: 100, 1k, 2.5k" />
      <span class="lbl">до</span>
      <input v-model="opts.stop" type="text" class="num" :class="{ bad: !stopOk }" :disabled="busy" title="Конечная частота, до 25 МГц: 100k, 1M" />
      <span class="lbl">Гц</span>
      <span class="lbl">точек/дек</span>
      <Seg v-model="opts.perDecade" :options="PER_DECADE" :disabled="busy" />
      <span class="lbl">генератор</span>
      <input v-model="opts.amp" type="text" class="num short" :class="{ bad: !ampOk }" :disabled="busy" title="Размах синуса генератора, 0.01…5 В" />
      <span class="lbl">В</span>
      <span class="lbl">усреднение</span>
      <Seg v-model="opts.average" :options="AVERAGES" :disabled="busy" />
      <button v-if="!busy" class="primary" :disabled="!props.online || !config || sweeping" :title="config ? `${count} точек` : 'Проверьте частоты и размах'" @click="run">
        ▶ Измерить
      </button>
      <button v-else class="stop" title="Остановить и вернуть настройки" @click="stopSweep">■ Стоп</button>
      <span class="grow" />
      <button class="ghost" :disabled="!sweep.points.length" title="Сохранить точки в CSV" @click="exportCSV">CSV</button>
      <button class="ghost" :disabled="!sweep.points.length" title="Сохранить картинку" @click="exportPNG">PNG</button>
    </div>
    <div ref="box" class="plot">
      <canvas ref="canvas" @mousemove="move" @mouseleave="hover = null" />
      <div v-if="!sweep.points.length && !busy" class="empty">
        <p>Выход генератора подайте на вход цепи и на CH1, выход цепи — на CH2.</p>
        <p class="muted">Программа включит выход генератора синусом, переберёт частоты и подберёт масштабы. Потом она вернёт все настройки осциллографа и генератора.</p>
      </div>
    </div>
    <div class="facts">
      <span v-if="busy" class="mono">
        {{ sweep.total ? `Точка ${Math.min(sweep.index + 1, sweep.total)} из ${sweep.total} · ${eng(sweep.freq, 'Гц', 3)}` : 'Подготовка…' }}
      </span>
      <span
        v-for="(k, i) in cut?.list ?? []"
        :key="i"
        class="mono kv"
        :title="`На 3 дБ ниже максимума усиления (${signed(cut!.ref, 1)} дБ)`"
      >
        {{ k.rising ? 'Нижний срез' : 'Срез' }} −3 дБ: {{ eng(k.freq, 'Гц', 3) }}, {{ deg(k.phase) }}
      </span>
      <span v-if="peak" class="mono kv">Максимум {{ signed(peak.db, 1) }} дБ на {{ eng(peak.freq, 'Гц', 3) }}</span>
      <span v-else-if="cut && !cut.list.length && bp.length > 1" class="mono dim kv">
        Спада на 3 дБ нет до {{ eng(bp[bp.length - 1].freq, 'Гц', 3) }}
      </span>
      <span v-if="missing.length" class="warn" :title="missing.map((p) => `${eng(p.freq, 'Гц', 3)}: ${p.note}`).join('\n')">
        Без результата: {{ missing.length }} из {{ sweep.points.length }} точек
      </span>
      <span v-if="sweep.state === 'error'" class="warn">{{ sweep.error }}</span>
      <span v-else-if="sweep.state === 'stopped'" class="dim">Остановлено, настройки возвращены</span>
      <span v-else-if="sweep.state === 'done'" class="dim">Готово, настройки возвращены</span>
    </div>
  </div>
</template>

<style scoped>
.bode { display: flex; flex-direction: column; gap: 6px; }
.bar { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; font-size: 12px; }
.title { font-weight: 600; color: var(--muted); text-transform: uppercase; letter-spacing: .06em; font-size: 12px; margin-right: 4px; }
.lbl { color: var(--muted); }
.grow { flex: 1; }
.num { width: 64px; font-size: 12px; padding: 3px 6px; font-family: var(--mono); }
.num.short { width: 44px; }
.num.bad { border-color: var(--err); }
.stop { background: var(--err-bg); border-color: #6b2a2f; color: #ffd0d0; }
.plot { position: relative; flex: 1; min-height: 140px; }
canvas { position: absolute; inset: 0; width: 100%; height: 100%; cursor: crosshair; }
.empty {
  position: absolute;
  inset: 0;
  display: grid;
  place-content: center;
  text-align: center;
  font-size: 13px;
  line-height: 1.5;
  padding: 0 24px;
  pointer-events: none;
}
.empty p { margin: 2px 0; max-width: 560px; }
.facts { display: flex; align-items: center; gap: 14px; flex-wrap: wrap; font-size: 12px; min-height: 18px; }
.kv { color: var(--text); }
.warn { color: var(--warn); }
.dim { color: var(--dim); }
</style>
