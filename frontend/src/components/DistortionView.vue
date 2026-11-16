<script setup lang="ts">
// Distortion against the amplitude: the generator feeds the input of a
// circuit and CH1 with a sine of one frequency and steps its amplitude, CH2
// sees the output. The sweep runs in Go; this panel sets it up and draws the
// distortion of CH2 and of CH1 and the gain.
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { eng, parseInput, signed, stamp } from '../format'
import { distortion as sweep, startSweep, stopSweep, sweeping } from '../state'
import { compression, median, onset, pct, thdPoints } from '../thd'
import type { SweepConfig } from '../types'
import Seg from './Seg.vue'

const props = defineProps<{ online: boolean }>()
const emit = defineEmits<{ save: [name: string, blob: Blob] }>()

const DIVY = 8
const M = { l: 58, r: 50, t: 10, b: 24 }

const SETTINGS_KEY = 'owon.thd'
const opts = reactive({ freq: '1k', start: '0.05', stop: '5', perDecade: '10', average: '4' })
try {
  Object.assign(opts, JSON.parse(localStorage.getItem(SETTINGS_KEY) ?? '{}'))
} catch {}
watch(opts, () => localStorage.setItem(SETTINGS_KEY, JSON.stringify(opts)))

const PER_DECADE = ['5', '10', '20'].map((v) => ({ value: v, label: v, title: `${v} точек на декаду размаха` }))
const AVERAGES = [
  { value: '1', label: '1', title: 'Один экран на точку' },
  { value: '4', label: '4', title: 'Среднее четырёх экранов: шум ниже вдвое' },
  { value: '8', label: '8', title: 'Среднее восьми экранов: шум ниже почти втрое, но дольше' },
]

const freq = computed(() => parseInput(opts.freq))
const start = computed(() => parseInput(opts.start))
const stop = computed(() => parseInput(opts.stop))
const freqOk = computed(() => freq.value != null && freq.value >= 10 && freq.value <= 1e6)
const startOk = computed(() => start.value != null && start.value >= 0.01 && start.value < 5)
const stopOk = computed(() => stop.value != null && stop.value <= 5 && (!startOk.value || stop.value > start.value!))
const config = computed<SweepConfig | null>(() =>
  freqOk.value && startOk.value && stopOk.value
    ? { mode: 'amp', start: start.value!, stop: stop.value!, perDecade: +opts.perDecade, freq: freq.value!, average: +opts.average }
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

const tp = computed(() => thdPoints(sweep.points))
const at1 = computed(() => onset(tp.value, 0.01))
const at10 = computed(() => onset(tp.value, 0.1))
const comp = computed(() => compression(tp.value))
const floor = computed(() => (tp.value.length ? median(tp.value.map((p) => p.thdIn)) : null))
const missing = computed(() => sweep.points.filter((p) => p.thd == null))
// The output is no more distorted than the generator and the ADC are.
const clean = computed(() => tp.value.length > 1 && tp.value.every((p) => p.thd < Math.max(2 * p.thdIn, 0.002)))

function harmDb(r: number | undefined) {
  return r != null && r > 0 ? `${(20 * Math.log10(r)).toFixed(0).replace('-', '−')} дБ` : '—'
}

// ---- drawing ----

const box = ref<HTMLDivElement>()
const canvas = ref<HTMLCanvasElement>()
const hover = ref<number | null>(null) // mouse x
let colors = { thd: '#6ea8ff', gain: '#f5a524' }
const INPUT = '#8f97a8'
let size = { w: 0, h: 0, dpr: 1 }
let raf = 0

function plotRect() {
  return { x: M.l, y: M.t, w: Math.max(10, size.w - M.l - M.r), h: Math.max(10, size.h - M.t - M.b) }
}

function xRange(): [number, number] {
  if (sweep.start > 0 && sweep.stop > sweep.start) return [sweep.start, sweep.stop]
  if (config.value) return [config.value.start, config.value.stop]
  return [0.05, 5]
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

// The distortion is drawn in dB, 20·lg of the fraction, so that its labels
// at every 20 dB fall on whole decades of per cent.
const dbOf = (frac: number) => 20 * Math.log10(Math.max(frac, 1e-6))

function draw() {
  const c = canvas.value
  if (!c) return
  const g = c.getContext('2d')!
  g.setTransform(size.dpr, 0, 0, size.dpr, 0, 0)
  g.clearRect(0, 0, size.w, size.h)
  const p = plotRect()
  g.fillStyle = '#090b0f'
  g.fillRect(p.x, p.y, p.w, p.h)

  const [a1, a2] = xRange()
  const xOf = (a: number) => p.x + (Math.log(a / a1) / Math.log(a2 / a1)) * p.w
  const b = tp.value
  const td = b.flatMap((q) => [dbOf(q.thd), dbOf(q.thdIn)]), gd = b.map((q) => q.db)
  const ta = axis(b.length ? Math.min(...td) : -60, b.length ? Math.max(-20, ...td) + 1 : -20, [10, 20])
  const ga = axis(b.length ? Math.min(...gd) : -10, b.length ? Math.max(...gd) + 0.5 : 0, [1, 2, 5, 10, 20])
  const yT = (v: number) => p.y + ((ta.top - v) / (DIVY * ta.step)) * p.h
  const yG = (v: number) => p.y + ((ga.top - v) / (DIVY * ga.step)) * p.h

  // Grid: decades solid, the 2…9 lines between them dotted.
  const decades = Math.log10(a2 / a1)
  const ticks: { a: number; major: boolean; label: boolean }[] = []
  for (let k = Math.floor(Math.log10(a1)); k <= Math.ceil(Math.log10(a2)); k++) {
    for (let m = 1; m <= 9; m++) {
      const a = m * 10 ** k
      if (a < a1 * 0.999 || a > a2 * 1.001) continue
      ticks.push({ a, major: m === 1, label: m === 1 || (decades <= 2.5 && (m === 2 || m === 5)) })
    }
  }
  g.lineWidth = 1
  g.strokeStyle = '#20252f'
  g.setLineDash([1, 3])
  g.beginPath()
  for (let i = 1; i < DIVY; i++) line(g, p.x, p.y + (i * p.h) / DIVY, p.x + p.w, p.y + (i * p.h) / DIVY)
  for (const t of ticks) if (!t.major) line(g, xOf(t.a), p.y, xOf(t.a), p.y + p.h)
  g.stroke()
  g.setLineDash([])
  g.strokeStyle = '#2a303c'
  g.beginPath()
  for (const t of ticks) if (t.major) line(g, xOf(t.a), p.y, xOf(t.a), p.y + p.h)
  g.stroke()
  g.strokeStyle = '#2b313d'
  g.strokeRect(p.x + 0.5, p.y + 0.5, p.w - 1, p.h - 1)

  g.font = '11px Cascadia Mono, Consolas, monospace'
  g.fillStyle = '#5e6678'
  g.textBaseline = 'top'
  g.textAlign = 'center'
  let lastX = -1e9
  for (const t of ticks) {
    const x = xOf(t.a)
    if (!t.label || x - lastX < 56) continue
    g.fillText(eng(t.a, 'В', 3, true), Math.min(p.x + p.w - 24, Math.max(p.x + 24, x)), p.y + p.h + 6)
    lastX = x
  }
  g.textBaseline = 'middle'
  for (let i = 0; i <= DIVY; i++) {
    const y = p.y + (i * p.h) / DIVY
    const v = ta.top - i * ta.step, w = ga.top - i * ga.step
    if (Math.round(v / 20) * 20 === v && v <= 0) {
      g.textAlign = 'right'
      g.fillStyle = colors.thd
      g.fillText(pct(10 ** (v / 20)), p.x - 6, y)
    }
    if (Math.round(w / ga.step) % 2 === 0) {
      g.textAlign = 'left'
      g.fillStyle = colors.gain
      g.fillText(`${signed(w, 0).replace('+0', '0')} дБ`, p.x + p.w + 6, y)
    }
  }

  g.save()
  g.beginPath()
  g.rect(p.x, p.y, p.w, p.h)
  g.clip()

  // The amplitude being measured.
  if (busy.value && sweep.amp > 0) {
    g.fillStyle = 'rgba(110,168,255,.10)'
    g.fillRect(xOf(sweep.amp) - 3, p.y, 6, p.h)
  }
  // The 1 % line, and where the distortion and the gain pass their limits.
  g.strokeStyle = 'rgba(110,168,255,.25)'
  g.setLineDash([2, 4])
  g.beginPath()
  line(g, p.x, yT(-40), p.x + p.w, yT(-40))
  g.stroke()
  g.font = '10px Segoe UI, sans-serif'
  g.textAlign = 'left'
  g.textBaseline = 'top'
  for (const [lv, label] of [[at1.value, 'КНИ 1 %'], [comp.value, '−1 дБ']] as const) {
    if (!lv) continue
    const x = xOf(lv.amp)
    g.strokeStyle = 'rgba(230,232,237,.35)'
    g.setLineDash([4, 3])
    g.beginPath()
    line(g, x, p.y, x, p.y + p.h)
    g.stroke()
    g.fillStyle = '#8f97a8'
    g.fillText(label, x + 4, p.y + (label === '−1 дБ' ? 18 : 4))
  }
  g.setLineDash([])

  const trace = (ys: number[], color: string, width: number, dash: number[] = []) => {
    g.strokeStyle = color
    g.lineWidth = width
    g.setLineDash(dash)
    g.beginPath()
    b.forEach((q, i) => (i ? g.lineTo(xOf(q.amp), ys[i]) : g.moveTo(xOf(q.amp), ys[i])))
    g.stroke()
    g.setLineDash([])
    g.fillStyle = color
    for (let i = 0; i < b.length; i++) {
      g.beginPath()
      g.arc(xOf(b[i].amp), ys[i], 2.2, 0, Math.PI * 2)
      g.fill()
    }
  }
  trace(b.map((q) => yT(dbOf(q.thdIn))), INPUT, 1, [4, 3])
  trace(b.map((q) => yG(q.db)), colors.gain, 1.2)
  trace(b.map((q) => yT(dbOf(q.thd))), colors.thd, 1.8)
  g.restore()

  if (hover.value != null && b.length) drawHover(g, p, xOf, yT, yG)
}

function drawHover(
  g: CanvasRenderingContext2D, p: ReturnType<typeof plotRect>,
  xOf: (a: number) => number, yT: (v: number) => number, yG: (v: number) => number,
) {
  const mx = hover.value!
  if (mx < p.x || mx > p.x + p.w) return
  const b = tp.value
  let k = 0
  for (let i = 1; i < b.length; i++) if (Math.abs(xOf(b[i].amp) - mx) < Math.abs(xOf(b[k].amp) - mx)) k = i
  const q = b[k]
  const x = xOf(q.amp)
  g.strokeStyle = 'rgba(230,232,237,.35)'
  g.setLineDash([3, 3])
  g.beginPath()
  line(g, x, p.y, x, p.y + p.h)
  g.stroke()
  g.setLineDash([])
  for (const [y, color] of [[yT(dbOf(q.thd)), colors.thd], [yG(q.db), colors.gain]] as const) {
    g.fillStyle = color
    g.beginPath()
    g.arc(x, y, 4, 0, Math.PI * 2)
    g.fill()
  }
  const rows: [string, string][] = [
    [`размах ${eng(q.amp, 'В', 3)}`, '#e6e8ed'],
    [`СКЗ ${eng(q.in, 'В', 3)} → ${eng(q.out, 'В', 3)}`, '#8f97a8'],
    [`КНИ ${pct(q.thd)}`, colors.thd],
    [`2-я ${harmDb(q.harm[0])}, 3-я ${harmDb(q.harm[1])}`, colors.thd],
    [`вход ${pct(q.thdIn)}`, INPUT],
    [`${signed(q.db, 2)} дБ`, colors.gain],
  ]
  g.font = '12px Cascadia Mono, Consolas, monospace'
  const w = 196, h = rows.length * 17 + 8
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
  const harms = Math.max(0, ...sweep.points.map((p) => p.harm?.length ?? 0))
  const lines = [
    `# OWON HDS272S distortion against the amplitude at ${eng(sweep.points[0]?.freq ?? 0, 'Hz', 4)}, CH2 against CH1; harmonics of CH2 against its fundamental`,
    ['gen_Vpp', 'in_Vrms', 'out_Vrms', 'gain_dB', 'thd_pct', 'thd_in_pct', ...Array.from({ length: harms }, (_, i) => `h${i + 2}_dB`), 'note'].join(','),
  ]
  const n = (v: number | null | undefined, p = 5) => (v == null ? '' : v.toPrecision(p))
  for (const s of sweep.points) {
    const db = s.gain != null && s.gain > 0 ? (20 * Math.log10(s.gain)).toFixed(3) : ''
    const hs = Array.from({ length: harms }, (_, i) => {
      const r = s.harm?.[i]
      return r != null && r > 0 ? (20 * Math.log10(r)).toFixed(1) : ''
    })
    lines.push([n(s.amp, 4), n(s.in), n(s.out), db, n(s.thd != null ? s.thd * 100 : null, 4), n(s.thdIn != null ? s.thdIn * 100 : null, 4), ...hs, s.note].join(','))
  }
  emit('save', `hds272s_thd_${stamp()}.csv`, new Blob([lines.join('\n') + '\n'], { type: 'text/csv' }))
}

async function exportPNG() {
  const b = await new Promise<Blob | null>((res) => canvas.value!.toBlob(res, 'image/png'))
  if (b) emit('save', `hds272s_thd_${stamp()}.png`, b)
}

let ro: ResizeObserver | undefined
onMounted(() => {
  const cs = getComputedStyle(document.documentElement)
  colors = { thd: cs.getPropertyValue('--accent').trim() || colors.thd, gain: cs.getPropertyValue('--warn').trim() || colors.gain }
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
watch([() => sweep.points.length, () => sweep.amp, () => sweep.state, hover, () => opts.start, () => opts.stop], schedule)
</script>

<template>
  <div class="thd">
    <div class="bar">
      <span class="title" title="Коэффициент гармоник выхода цепи (CH2) в зависимости от размаха генератора">КНИ</span>
      <span class="lbl">частота</span>
      <input v-model="opts.freq" type="text" class="num" :class="{ bad: !freqOk }" :disabled="busy" title="Частота синуса, от 10 Гц до 1 МГц: 1k, 400" />
      <span class="lbl">Гц</span>
      <span class="lbl">размах от</span>
      <input v-model="opts.start" type="text" class="num short" :class="{ bad: !startOk }" :disabled="busy" title="Начальный размах генератора, от 0.01 В" />
      <span class="lbl">до</span>
      <input v-model="opts.stop" type="text" class="num short" :class="{ bad: !stopOk }" :disabled="busy" title="Конечный размах генератора, до 5 В" />
      <span class="lbl">В</span>
      <span class="lbl">точек/дек</span>
      <Seg v-model="opts.perDecade" :options="PER_DECADE" :disabled="busy" />
      <span class="lbl">усреднение</span>
      <Seg v-model="opts.average" :options="AVERAGES" :disabled="busy" />
      <button
        v-if="!busy"
        class="primary"
        :disabled="!props.online || !config || sweeping"
        :title="config ? `${count} точек` : 'Проверьте частоту и размах'"
        @click="run"
      >
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
        <p class="muted">Программа включит синус, пройдёт по размахам генератора и подберёт масштабы. Потом она вернёт все настройки осциллографа и генератора.</p>
      </div>
    </div>
    <div class="facts">
      <span v-if="busy" class="mono">
        {{ sweep.total ? `Точка ${Math.min(sweep.index + 1, sweep.total)} из ${sweep.total} · размах ${eng(sweep.amp, 'В', 3)}` : 'Подготовка…' }}
      </span>
      <template v-if="tp.length">
        <span v-if="at1" class="mono kv" title="Размах генератора и СКЗ выхода, где КНИ выхода превышает 1 %">
          КНИ 1 %: размах {{ eng(at1.amp, 'В', 3) }}, выход {{ eng(at1.out, 'В', 3) }} СКЗ
        </span>
        <span v-else-if="tp[0].thd >= 0.01" class="mono kv">КНИ выше 1 % уже на размахе {{ eng(tp[0].amp, 'В', 3) }}</span>
        <span v-else-if="!busy" class="mono dim kv">КНИ ниже 1 % до размаха {{ eng(tp[tp.length - 1].amp, 'В', 3) }}</span>
        <span v-if="at10" class="mono kv" title="Размах генератора и СКЗ выхода, где КНИ выхода превышает 10 %">
          10 %: размах {{ eng(at10.amp, 'В', 3) }}, выход {{ eng(at10.out, 'В', 3) }} СКЗ
        </span>
        <span v-if="comp" class="mono kv" :title="`Усиление на 1 дБ ниже, чем на малых уровнях (${signed(comp.ref, 1)} дБ)`">
          Сжатие −1 дБ: размах {{ eng(comp.amp, 'В', 3) }}, выход {{ eng(comp.out, 'В', 3) }} СКЗ
        </span>
        <span v-if="floor != null" class="mono dim" title="Медиана КНИ на CH1: генератор и АЦП прибора. Искажения цепи ниже этого уровня не различить">
          свой КНИ ≈ {{ pct(floor) }}
        </span>
        <span v-if="clean && !busy" class="dim">Выход искажён не больше, чем генератор и АЦП: искажения цепи ниже порога прибора</span>
      </template>
      <span v-if="missing.length" class="warn" :title="missing.map((p) => `${eng(p.amp, 'В', 3)}: ${p.note}`).join('\n')">
        Без результата: {{ missing.length }} из {{ sweep.points.length }} точек
      </span>
      <span v-if="sweep.state === 'error'" class="warn">{{ sweep.error }}</span>
      <span v-else-if="sweep.state === 'stopped'" class="dim">Остановлено, настройки возвращены</span>
      <span v-else-if="sweep.state === 'done'" class="dim">Готово, настройки возвращены</span>
    </div>
  </div>
</template>

<style scoped>
.thd { display: flex; flex-direction: column; gap: 6px; }
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
