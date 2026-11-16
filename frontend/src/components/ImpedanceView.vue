<script setup lang="ts">
// Impedance of a part from a reference resistor: CH1 on the generator, CH2
// on the node between the part and the resistor. Every frame gives CH2/CH1
// at the fundamental; the panel averages it and turns it into C, L or R.
import { computed, reactive, ref, shallowRef, watch } from 'vue'
import { TIMEBASES, closest, eng, fitScale, parseInput, probeFactor, signed, vdivOptions } from '../format'
import { bestFreq, cdiv, impedance, inputLoad, polar, read, type C, type Reading } from '../impedance'
import { gen, set, setGen, setTrigLevel } from '../state'
import type { Frame } from '../types'
import Seg from './Seg.vue'

const props = defineProps<{ frame: Frame | null; online: boolean }>()

const SETTINGS_KEY = 'owon.imp'
const opts = reactive({ rRef: '1k', scheme: 'top', model: 'series', avg: '4', load: true })
try {
  Object.assign(opts, JSON.parse(localStorage.getItem(SETTINGS_KEY) ?? '{}'))
} catch {}
watch(opts, () => localStorage.setItem(SETTINGS_KEY, JSON.stringify(opts)))

const SCHEMES = [
  { value: 'top', label: 'деталь к земле', title: 'Генератор → R → деталь → земля; CH2 на детали' },
  { value: 'bottom', label: 'R к земле', title: 'Генератор → деталь → R → земля; CH2 на резисторе' },
]
const MODELS = [
  { value: 'series', label: 'послед.', title: 'Последовательная схема замещения: ESR и C или L' },
  { value: 'parallel', label: 'паралл.', title: 'Параллельная схема замещения: сопротивление утечки и C или L' },
]
const AVGS = [
  { value: '1', label: '1', title: 'Только этот кадр' },
  { value: '4', label: '4', title: 'Среднее по 4 кадрам' },
  { value: '16', label: '16', title: 'Среднее по 16 кадрам' },
]

const rRef = computed(() => parseInput(opts.rRef))
const rOk = computed(() => rRef.value != null && rRef.value > 0)

// The channels differ by up to 3 % in gain each; with both probes on one
// point their ratio shows the difference, which then divides every ratio
// taken at the same scales.
interface Cal {
  h: C
  key: string
  f: number
}
const CAL_KEY = 'owon.impCal'
const cal = ref<Cal | null>(null)
try {
  cal.value = JSON.parse(localStorage.getItem(CAL_KEY) ?? 'null')
} catch {}
watch(cal, (c) => localStorage.setItem(CAL_KEY, JSON.stringify(c)))

interface Result {
  h: C // averaged CH2/CH1, before the channel correction
  f: number
  scaleKey: string
  ch2Divs: number // CH2 swing in divisions
  clipped: boolean
  probe2: number
}

const result = shallowRef<Result | null>(null)
let history: C[] = []
let historyKey = ''

function compute(fr: Frame | null) {
  const c1 = fr?.channels.find((c) => c.name === 'CH1')
  const c2 = fr?.channels.find((c) => c.name === 'CH2')
  const p = fr?.pair
  const f = c1?.meas.freq
  if (!fr || !c1?.on || !c2?.on || !p || p.gain == null || p.phase == null || !f) {
    result.value = null
    return
  }
  const scaleKey = [c1.voltsPerDiv, c2.voltsPerDiv, c1.probe, c2.probe].join('|')
  // A new scale or a frequency more than 2 % away starts the average over.
  const key = `${scaleKey}|${Math.round(Math.log(f) / 0.02)}`
  if (key !== historyKey) {
    history = []
    historyKey = key
  }
  history.push(polar(p.gain, p.phase))
  if (history.length > +opts.avg) history.splice(0, history.length - +opts.avg)
  const h: C = [history.reduce((a, v) => a + v[0], 0) / history.length, history.reduce((a, v) => a + v[1], 0) / history.length]
  result.value = {
    h, f, scaleKey,
    ch2Divs: (c2.meas.pp ?? 0) / c2.voltsPerDiv,
    clipped: c1.meas.clipped || c2.meas.clipped,
    probe2: probeFactor(c2.probe),
  }
}
watch(() => props.frame, compute)
watch(() => opts.avg, () => {
  historyKey = ''
  compute(props.frame)
})

// The correction applies at the scales it was taken at, within a factor of
// two in frequency: the input capacitances of the channels differ.
const calState = computed(() => {
  const r = result.value, c = cal.value
  if (!c) return 'none'
  if (!r) return 'idle'
  return c.key === r.scaleKey && Math.abs(Math.log(r.f / c.f)) < Math.LN2 ? 'on' : 'off'
})

const reading = computed<Reading | null>(() => {
  const r = result.value
  if (!r || !rOk.value) return null
  const h = calState.value === 'on' ? cdiv(r.h, cal.value!.h) : r.h
  const z = impedance(h, rRef.value!, r.f, opts.scheme as 'top' | 'bottom', opts.load ? inputLoad(r.probe2) : null)
  if (!Number.isFinite(z[0]) || !Number.isFinite(z[1])) return null
  return read(z, r.f)
})

const model = computed(() => (reading.value ? reading.value[opts.model === 'parallel' ? 'parallel' : 'series'] : null))

const main = computed(() => {
  const rd = reading.value, m = model.value
  if (!rd || !m) return ''
  if (rd.kind === 'C') return `C = ${eng(m.value, 'Ф', 4)}`
  if (rd.kind === 'L') return `L = ${eng(m.value, 'Гн', 4)}`
  return `R = ${eng(m.value, 'Ом', 4)}`
})

const notes = computed(() => {
  const r = result.value, rd = reading.value
  const out: string[] = []
  if (!r) return out
  if (r.clipped) out.push('Сигнал выходит за экран: нажмите «Подготовить»')
  if (r.ch2Divs < 1) out.push('На CH2 меньше деления: уменьшите В/дел у CH2')
  if (rd && rOk.value) {
    const q = rd.mag / rRef.value!
    if (q > 10 || q < 0.1) {
      const more = q > 10
      const fix = rd.kind === 'R' ? '' : (rd.kind === 'C') === more ? ' Повысьте частоту или' : ' Понизьте частоту или'
      out.push(`|Z| в ${(more ? q : 1 / q).toFixed(0)} раз ${more ? 'больше' : 'меньше'} эталона: точность падает.${fix || ''} ${fix ? 'возьмите' : 'Возьмите'} ${more ? 'больший' : 'меньший'} резистор`)
    }
  }
  if (calState.value === 'off') out.push('Сверка каналов была на других масштабах или частоте: не учтена')
  return out
})

// ---- actions ----

// prepare sets a sine without offset, the trigger on CH1 and a timebase of
// four to ten periods, and gives both channels the scale that fits CH1, so
// that a check of the channels with both probes on one point fits too.
function prepare() {
  if (!gen.wave.toUpperCase().startsWith('SIN')) setGen('wave', 'SINE')
  if (gen.offset !== 0) setGen('offset', 0)
  if (!gen.output) setGen('output', true)
  set('trig.source', 'CH1')
  set('trig.edge', 'RISE')
  setTrigLevel(0)
  setTimebase(gen.freq)
  const fr = props.frame
  const c1 = fr?.channels.find((c) => c.name === 'CH1')
  const pp = c1?.meas.pp != null && !c1.meas.clipped ? c1.meas.pp : gen.amp
  const v1 = fitScale(c1?.probe ?? '1X', pp)
  for (const ch of fr?.channels ?? []) {
    const k = ch.name.toLowerCase()
    if (k !== 'ch1' && k !== 'ch2') continue
    if (!ch.on) set(`${k}.display`, 'ON')
    const scales = vdivOptions(probeFactor(ch.probe))
    const v = scales[closest(scales, v1.value)]
    if (v.value !== scales[closest(scales, ch.voltsPerDiv)].value) set(`${k}.scale`, v.cmd)
    set(`${k}.offset`, '0.00')
  }
}

function setTimebase(f: number) {
  const tb = TIMEBASES.find((t) => f * 12 * t.value >= 4) ?? TIMEBASES[TIMEBASES.length - 1]
  set('hor.scale', tb.cmd)
}

const target = computed(() => {
  const rd = reading.value
  if (!rd || !rOk.value) return null
  const f = bestFreq(rd, rRef.value!)
  if (f == null || !Number.isFinite(f)) return null
  return +Math.min(5e6, Math.max(10, f)).toPrecision(3)
})

function tune() {
  const f = target.value
  if (f == null) return
  setGen('freq', f)
  setTimebase(f)
}

// With both probes on one point the channels differ by a few per cent and
// degrees at most; anything more means the probes are elsewhere.
const calError = ref('')
function calibrate() {
  const r = result.value
  if (!r) return
  const g = Math.hypot(r.h[0], r.h[1]), ph = (Math.atan2(r.h[1], r.h[0]) * 180) / Math.PI
  if (Math.abs(g - 1) > 0.15 || Math.abs(ph) > 10) {
    calError.value = `Щупы не на одной точке: CH2/CH1 = ${g.toFixed(3)}, ${signed(ph, 1)}°. Сверка не сохранена`
    setTimeout(() => (calError.value = ''), 8000)
    return
  }
  calError.value = ''
  cal.value = { h: r.h, key: r.scaleKey, f: r.f }
}

function fmtR(v: number) {
  return v < 0 ? '≈ 0' : eng(v, 'Ом', 3)
}
</script>

<template>
  <div class="imp">
    <div class="bar">
      <span class="title" title="Импеданс детали через эталонный резистор">Импеданс</span>
      <span class="lbl">R эталон</span>
      <input v-model="opts.rRef" type="text" class="num" :class="{ bad: !rOk }" title="Сопротивление эталонного резистора: 100, 1k, 10k" />
      <span class="lbl">Ом</span>
      <Seg v-model="opts.scheme" :options="SCHEMES" />
      <Seg v-model="opts.model" :options="MODELS" />
      <span class="lbl">усреднение</span>
      <Seg v-model="opts.avg" :options="AVGS" />
      <label class="chk" title="Вычесть вход прибора на CH2: 1 МОм и 16 пФ при щупе 1×, при делителе только сопротивление">
        <input v-model="opts.load" type="checkbox" /> вход прибора
      </label>
      <span class="grow" />
      <button :disabled="!props.online" title="Синус без смещения, синхронизация по CH1, развёртка на 4–10 периодов, одинаковый масштаб каналов" @click="prepare">
        Подготовить
      </button>
      <button
        :disabled="!props.online || target == null"
        :title="target ? `Поставить ${eng(target, 'Гц', 3)}: на этой частоте |Z| детали равен эталону, и точность лучше всего` : 'Нужен конденсатор или катушка'"
        @click="tune"
      >
        Подобрать частоту
      </button>
      <button
        :class="{ on: calState === 'on' }"
        :disabled="!result"
        title="Поставьте оба щупа на одну точку, например на выход генератора, и нажмите. Программа запомнит разницу каналов на этих масштабах и частоте и будет её вычитать"
        @click="calibrate"
      >
        {{ calState === 'on' ? 'Сверка ✓' : 'Сверка каналов' }}
      </button>
      <button v-if="cal" class="ghost" title="Забыть сверку каналов" @click="cal = null">✕</button>
    </div>
    <div v-if="calError" class="warn">{{ calError }}</div>
    <div v-if="reading && model" class="readout">
      <div class="main mono">{{ main }}</div>
      <div class="facts">
        <span class="mono kv" title="Модуль импеданса">|Z| {{ eng(reading.mag, 'Ом', 4) }}</span>
        <span class="mono kv" title="Фаза импеданса: −90° у идеального конденсатора, +90° у идеальной катушки">θ {{ signed(reading.phase, 1) }}°</span>
        <span v-if="reading.kind !== 'R'" class="mono kv" :title="opts.model === 'series' ? 'Последовательное сопротивление потерь' : 'Параллельное сопротивление утечки'">
          {{ opts.model === 'series' ? 'ESR' : 'Rп' }} {{ fmtR(model.r) }}
        </span>
        <span v-else class="mono kv" title="Реактивное сопротивление">X {{ eng(model.x, 'Ом', 3) }}</span>
        <span v-if="reading.kind === 'C'" class="mono kv" title="Тангенс угла потерь: ESR / |X|">tg δ {{ reading.series.r < 0 ? '≈ 0' : reading.d.toPrecision(2) }}</span>
        <span v-if="reading.kind === 'L'" class="mono kv" title="Добротность: X / R">Q {{ reading.series.r <= 0 ? '—' : (1 / reading.d).toPrecision(3) }}</span>
        <span class="mono dim">на {{ eng(result!.f, 'Гц', 4) }}</span>
        <span v-for="(t, i) in notes" :key="i" class="warn">{{ t }}</span>
      </div>
    </div>
    <div v-else class="empty">
      <template v-if="opts.scheme === 'top'">Генератор → R эталон → деталь → земля. CH1 на выходе генератора, CH2 на детали.</template>
      <template v-else>Генератор → деталь → R эталон → земля. CH1 на выходе генератора, CH2 на резисторе.</template>
      <span class="muted"> Нужны оба канала и синус; «Подготовить» настроит генератор и экран.</span>
    </div>
  </div>
</template>

<style scoped>
.imp { display: flex; flex-direction: column; gap: 8px; }
.bar { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; font-size: 12px; }
.title { font-weight: 600; color: var(--muted); text-transform: uppercase; letter-spacing: .06em; font-size: 12px; margin-right: 4px; }
.lbl { color: var(--muted); }
.grow { flex: 1; }
.num { width: 58px; font-size: 12px; padding: 3px 6px; font-family: var(--mono); }
.num.bad { border-color: var(--err); }
.chk { display: inline-flex; align-items: center; gap: 4px; color: var(--muted); margin-left: 4px; }
button.on { color: var(--ok); border-color: #23603b; }
.readout { display: flex; align-items: baseline; gap: 18px; flex-wrap: wrap; padding: 6px 4px 2px; }
.main { font-size: 28px; font-weight: 600; letter-spacing: .02em; }
.facts { display: flex; align-items: center; gap: 14px; flex-wrap: wrap; font-size: 12px; }
.kv { color: var(--text); }
.warn { color: var(--warn); }
.dim { color: var(--dim); }
.empty { font-size: 13px; padding: 6px 4px; line-height: 1.5; }
</style>
