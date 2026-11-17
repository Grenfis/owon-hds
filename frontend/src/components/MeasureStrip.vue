<script setup lang="ts">
// Measurements of each channel and of CH2 against CH1. The set to show is
// chosen in a menu and kept between runs. With statistics on, each
// measurement also shows its mean, minimum, maximum and standard deviation
// over the frames since the settings last changed.
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { eng } from '../format'
import { mathName, mathStats, mathUnit } from '../math'
import { display, mathTrace } from '../state'
import { push, sd, type Running } from '../stats'
import type { Frame, FrameChannel, Measurements, Pair } from '../types'

const props = defineProps<{ frame: Frame | null }>()

interface Def {
  id: string
  label: string
  abbr: string
  title: string
  get: (m: Measurements) => number | null
  fmt: (v: number | null) => string
  // show overrides fmt for the value of the frame.
  show?: (m: Measurements, dt: number) => string
}

const V = (k: keyof Measurements) => ({ get: (m: Measurements) => m[k] as number | null, fmt: (v: number | null) => eng(v, 'В', 3) })
const pct = (v: number | null) => (v == null ? '—' : `${v.toFixed(1)} %`)
// An edge shorter than two samples is only known to be that short.
const edge = (k: 'rise' | 'fall') => ({
  get: (m: Measurements) => m[k],
  fmt: (v: number | null) => eng(v, 'с', 3),
  show: (m: Measurements, dt: number) => {
    const v = m[k]
    return v != null && v < 2 * dt ? `< ${eng(2 * dt, 'с', 3, true)}` : eng(v, 'с', 3)
  },
})
const time = (k: keyof Measurements, sig: number) => ({ get: (m: Measurements) => m[k] as number | null, fmt: (v: number | null) => eng(v, 'с', sig) })

const GROUPS: { name: string; defs: Def[] }[] = [
  {
    name: 'Напряжение',
    defs: [
      { id: 'pp', label: 'Размах', abbr: 'Vpp', title: 'От минимума до максимума, с выбросами и шумом', ...V('pp') },
      { id: 'amp', label: 'Амплитуда', abbr: 'Vamp', title: 'От низа до верха: размах без выбросов. У синуса равна размаху', ...V('amp') },
      { id: 'max', label: 'Максимум', abbr: 'Vmax', title: 'Самая высокая точка на экране', ...V('max') },
      { id: 'min', label: 'Минимум', abbr: 'Vmin', title: 'Самая низкая точка на экране', ...V('min') },
      { id: 'top', label: 'Верх', abbr: 'Vtop', title: 'Вершина импульса без выброса: самый частый уровень в верхней половине. У синуса равен максимуму', ...V('top') },
      { id: 'base', label: 'Низ', abbr: 'Vbase', title: 'Основание импульса: самый частый уровень в нижней половине. У синуса равно минимуму', ...V('base') },
      { id: 'overshoot', label: 'Выброс', abbr: '', title: 'На сколько максимум выше верха, в процентах амплитуды', get: (m) => m.overshoot, fmt: pct },
      { id: 'preshoot', label: 'Предвыброс', abbr: '', title: 'На сколько минимум ниже низа, в процентах амплитуды', get: (m) => m.preshoot, fmt: pct },
    ],
  },
  {
    name: 'Средние',
    defs: [
      { id: 'avg', label: 'Среднее', abbr: 'Vavg', title: 'Среднее по всему экрану', ...V('avg') },
      { id: 'rms', label: 'СКЗ', abbr: 'Vrms', title: 'Среднеквадратичное по всему экрану, вместе с постоянной составляющей', ...V('rms') },
      { id: 'cycAvg', label: 'Среднее пер.', abbr: 'Vavg', title: 'Среднее по целым периодам: часть периода на краю экрана не искажает результат', ...V('cycAvg') },
      { id: 'cycRms', label: 'СКЗ пер.', abbr: 'Vrms', title: 'Среднеквадратичное по целым периодам, вместе с постоянной составляющей', ...V('cycRms') },
      { id: 'acRms', label: 'СКЗ перем.', abbr: 'AC', title: 'Среднеквадратичное переменной составляющей по целым периодам: то, что покажет мультиметр в режиме ~V', ...V('acRms') },
    ],
  },
  {
    name: 'Время',
    defs: [
      { id: 'freq', label: 'Частота', abbr: 'f', title: 'Частоту считает сам прибор', get: (m) => m.freq, fmt: (v) => eng(v, 'Гц', 5) },
      { id: 'period', label: 'Период', abbr: 'T', title: 'Период по частоте, которую считает прибор', ...time('period', 4) },
      { id: 'rise', label: 'Фронт', abbr: 'tr', title: 'Время нарастания от 10 до 90 % амплитуды', ...edge('rise') },
      { id: 'fall', label: 'Спад', abbr: 'tf', title: 'Время спада от 90 до 10 % амплитуды', ...edge('fall') },
      { id: 'pWidth', label: 'Ширина +', abbr: '+W', title: 'Длительность импульса выше уровня 50 %', ...time('pWidth', 4) },
      { id: 'nWidth', label: 'Ширина −', abbr: '−W', title: 'Длительность паузы ниже уровня 50 %', ...time('nWidth', 4) },
      { id: 'duty', label: 'Заполн. +', abbr: '+D', title: 'Доля периода выше уровня 50 %', get: (m) => m.duty, fmt: pct },
      { id: 'nDuty', label: 'Заполн. −', abbr: '−D', title: 'Доля периода ниже уровня 50 %', get: (m) => (m.duty == null ? null : 100 - m.duty), fmt: pct },
    ],
  },
]

interface PairDef {
  id: string
  label: string
  abbr: string
  title: string
  get: (p: Pair) => number | null
  fmt: (v: number | null) => string
  fmtSd?: (v: number) => string
}

const PAIR: PairDef[] = [
  { id: 'gain', label: 'Усиление', abbr: 'K', title: 'Отношение амплитуд основной гармоники CH2 и CH1, в разах и в децибелах, как на графике АЧХ. Шум и постоянная составляющая на него почти не влияют', get: (p) => p.gain, fmt: (v) => (v == null ? '—' : `${v.toPrecision(3)} · ${signedDb(v)}`), fmtSd: (v) => v.toPrecision(2) },
  { id: 'phase', label: 'Фаза', abbr: 'φ', title: 'Сдвиг фазы основной гармоники CH2 относительно CH1. Минус: CH2 отстаёт, как на выходе ФНЧ', get: (p) => p.phase, fmt: (v) => (v == null ? '—' : `${v.toFixed(1).replace('-', '−')}°`), fmtSd: (v) => `${v.toFixed(2)}°` },
  { id: 'delay', label: 'Задержка', abbr: 'Δt', title: 'Время от фронта CH1 до ближайшего фронта CH2 по уровню 50 %, в пределах полупериода', get: (p) => p.delay, fmt: (v) => eng(v, 'с', 4) },
]

function signedDb(k: number) {
  if (!(k > 0)) return '—'
  const d = 20 * Math.log10(k)
  return `${d >= 0 ? '+' : '−'}${Math.abs(d).toFixed(1)} дБ`
}

const ALL = GROUPS.flatMap((g) => g.defs)
const DEFAULT = ['freq', 'period', 'pp', 'max', 'min', 'avg', 'acRms', 'rise', 'duty', 'gain', 'phase']

const KEY = 'owon.meas'
function load(): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(KEY) ?? 'null')
    if (Array.isArray(v)) return v
  } catch {}
  return DEFAULT
}
const chosen = ref<string[]>(load())
watch(chosen, (v) => {
  try {
    localStorage.setItem(KEY, JSON.stringify(v))
  } catch {}
})

function toggle(id: string) {
  chosen.value = chosen.value.includes(id) ? chosen.value.filter((x) => x !== id) : [...chosen.value, id]
}

function dtOf(f: Frame, c: FrameChannel) {
  return (f.timeDiv * 12) / (c.raw?.length || 600)
}

// ---- statistics ----

const STATS_KEY = 'owon.measStats'
const statsOn = ref(localStorage.getItem(STATS_KEY) === '1')
watch(statsOn, (v) => {
  try {
    localStorage.setItem(STATS_KEY, v ? '1' : '0')
  } catch {}
  resetStats()
})

// Kept outside Vue's reactivity; tick tells the rows to read it again.
let stats = new Map<string, Running>()
let statsKey = ''
let statsSeq = -1
const statsFrames = ref(0)
const tick = ref(0)

function resetStats() {
  stats = new Map()
  statsKey = ''
  statsFrames.value = 0
  tick.value++
}

const MATH_DEFS: { id: string; label: string; get: (s: ReturnType<typeof mathStats>) => number }[] = [
  { id: 'pp', label: 'Размах', get: (s) => s.pp },
  { id: 'max', label: 'Максимум', get: (s) => s.max },
  { id: 'min', label: 'Минимум', get: (s) => s.min },
  { id: 'avg', label: 'Среднее', get: (s) => s.avg },
  { id: 'rms', label: 'СКЗ', get: (s) => s.rms },
]

// Every new frame adds its values; a change of what places the traces on
// the screen starts again, since the values before it are of another kind.
watch(() => props.frame, (f) => {
  if (!statsOn.value || !f || f.seq === statsSeq) return
  const key = [f.timeDiv, f.hOffset, f.acqMode, f.depMem, display.math, ...f.channels.map((c) => `${c.on} ${c.voltsPerDiv} ${c.offset} ${c.coupling}`)].join('|')
  if (key !== statsKey) {
    resetStats()
    statsKey = key
  }
  statsSeq = f.seq
  const add = (id: string, v: number | null) => {
    if (v != null && Number.isFinite(v)) stats.set(id, push(stats.get(id), v))
  }
  for (const c of f.channels) if (c.on && c.raw?.length) for (const d of ALL) add(`${c.name}.${d.id}`, d.get(c.meas))
  if (f.pair) for (const d of PAIR) add(`pair.${d.id}`, d.get(f.pair))
  const m = mathTrace.value
  if (m) {
    const s = mathStats(m.v)
    for (const d of MATH_DEFS) add(`M.${d.id}`, d.get(s))
  }
  statsFrames.value++
  tick.value++
})

// statOf gives the lines of statistics under a value.
function statOf(id: string, fmt: (v: number | null) => string, fmtSd = (v: number) => fmt(v)) {
  void tick.value
  const s = stats.get(id)
  if (!statsOn.value) return null
  if (!s) return ['—', '—', '—', '—']
  return [fmt(s.mean), fmt(s.min), fmt(s.max), fmtSd(sd(s))]
}

const rows = computed(() => {
  const f = props.frame
  if (!f) return []
  const defs = ALL.filter((d) => chosen.value.includes(d.id))
  return f.channels
    .filter((c) => c.on && c.raw?.length)
    .map((c) => ({
      name: c.name,
      clipped: c.meas.clipped,
      items: defs.map((d) => ({
        ...d,
        value: d.show ? d.show(c.meas, dtOf(f, c)) : d.fmt(d.get(c.meas)),
        stat: statOf(`${c.name}.${d.id}`, d.fmt),
      })),
    }))
})

// The math channel is measured here over the whole screen, with a fixed set.
const mathItems = computed(() => {
  const m = mathTrace.value
  if (!m) return null
  const s = mathStats(m.v), u = mathUnit(display.math)
  const fmt = (v: number | null) => eng(v, u, 3)
  return {
    name: mathName(display.math),
    clipped: m.clipped,
    items: MATH_DEFS.map((d) => ({ id: d.id, label: d.label, value: fmt(d.get(s)), stat: statOf(`M.${d.id}`, fmt) })),
  }
})

const pairItems = computed(() => {
  const p = props.frame?.pair
  if (!p) return []
  return PAIR.filter((d) => chosen.value.includes(d.id)).map((d) => ({ ...d, value: d.fmt(d.get(p)), stat: statOf(`pair.${d.id}`, d.fmt, d.fmtSd) }))
})

const STAT_LINES = ['ср.', 'мин', 'макс', 'σ']

// ---- menu ----

const open = ref(false)
const menu = ref<HTMLDivElement>()
const place = ref<Record<string, string>>({})

// The menu opens above its button, inside the window even when the strip
// is tall.
function toggleMenu(e: MouseEvent) {
  const b = (e.currentTarget as HTMLElement).getBoundingClientRect()
  place.value = {
    right: `${Math.max(8, window.innerWidth - b.right)}px`,
    bottom: `${window.innerHeight - b.top + 6}px`,
    maxHeight: `${Math.max(160, b.top - 14)}px`,
  }
  open.value = !open.value
}

// The menu button toggles the menu itself, so a press on it is not outside.
function outside(e: MouseEvent) {
  const t = e.target as Element
  if (!menu.value?.contains(t) && !t.closest?.('.cfg')) open.value = false
}
watch(open, (o) => (o ? window.addEventListener('mousedown', outside) : window.removeEventListener('mousedown', outside)))
onBeforeUnmount(() => window.removeEventListener('mousedown', outside))
</script>

<template>
  <div class="strip">
    <div v-for="(r, i) in rows" :key="r.name" class="row" :class="r.name.toLowerCase()">
      <span class="tag">{{ r.name }}</span>
      <div v-if="statsOn" class="legend"><span class="k">&nbsp;</span><span class="v">сейчас</span><span v-for="l in STAT_LINES" :key="l" class="s">{{ l }}</span></div>
      <div v-for="it in r.items" :key="it.id" class="item" :title="it.title">
        <span class="k">{{ it.label }} <span v-if="it.abbr" class="ab">{{ it.abbr }}</span></span>
        <span class="v mono">{{ it.value }}</span>
        <span v-for="(s, j) in it.stat ?? []" :key="j" class="s mono">{{ s }}</span>
      </div>
      <span v-if="!r.items.length" class="muted hint">Выберите измерения в меню справа</span>
      <span class="end">
        <span v-if="r.clipped" class="clip" title="Часть сигнала за краем экрана: размах и амплитуда занижены. Увеличьте В/дел.">за экраном</span>
        <template v-if="i === 0 && statsOn">
          <span class="mono frames" title="Сколько кадров вошло в статистику с последней смены настроек">{{ statsFrames }} кадр.</span>
          <button class="ghost cfg" title="Начать статистику заново" @click="resetStats">⟲</button>
        </template>
        <button v-if="i === 0" class="ghost cfg" :class="{ on: open }" title="Выбрать, какие измерения показывать" @click="toggleMenu">⚙</button>
      </span>
    </div>
    <div v-if="mathItems" class="row math">
      <span class="tag" :title="`Математика: ${mathItems.name}, по всему экрану`">M</span>
      <span class="mono fn">{{ mathItems.name }}</span>
      <div v-if="statsOn" class="legend"><span class="k">&nbsp;</span><span class="v">сейчас</span><span v-for="l in STAT_LINES" :key="l" class="s">{{ l }}</span></div>
      <div v-for="it in mathItems.items" :key="it.id" class="item">
        <span class="k">{{ it.label }}</span>
        <span class="v mono">{{ it.value }}</span>
        <span v-for="(s, j) in it.stat ?? []" :key="j" class="s mono">{{ s }}</span>
      </div>
      <span class="end">
        <span v-if="mathItems.clipped" class="clip" title="Канал за краем экрана: и результат неверен. Увеличьте В/дел.">за экраном</span>
      </span>
    </div>
    <div v-if="pairItems.length" class="row pair">
      <span class="tag" title="CH2 относительно CH1">1→2</span>
      <div v-if="statsOn" class="legend"><span class="k">&nbsp;</span><span class="v">сейчас</span><span v-for="l in STAT_LINES" :key="l" class="s">{{ l }}</span></div>
      <div v-for="it in pairItems" :key="it.id" class="item" :title="it.title">
        <span class="k">{{ it.label }} <span class="ab">{{ it.abbr }}</span></span>
        <span class="v mono">{{ it.value }}</span>
        <span v-for="(s, j) in it.stat ?? []" :key="j" class="s mono">{{ s }}</span>
      </div>
    </div>
    <div v-if="!rows.length" class="row empty muted">Измерения появятся, когда придут данные с прибора</div>

    <div v-if="open" ref="menu" class="menu" :style="place">
      <div class="cols">
        <div v-for="g in GROUPS" :key="g.name" class="col">
          <div class="gname">{{ g.name }}</div>
          <label v-for="d in g.defs" :key="d.id" :title="d.title">
            <input type="checkbox" :checked="chosen.includes(d.id)" @change="toggle(d.id)" />
            {{ d.label }} <span v-if="d.abbr" class="ab">{{ d.abbr }}</span>
          </label>
        </div>
        <div class="col">
          <div class="gname">CH2 относительно CH1</div>
          <label v-for="d in PAIR" :key="d.id" :title="d.title">
            <input type="checkbox" :checked="chosen.includes(d.id)" @change="toggle(d.id)" />
            {{ d.label }} <span class="ab">{{ d.abbr }}</span>
          </label>
          <p class="note muted">Строка «1→2» видна, когда включены оба канала.</p>
        </div>
      </div>
      <div class="actions">
        <label class="stats" title="Под каждым измерением: среднее, минимум, максимум и стандартное отклонение по кадрам с последней смены настроек">
          <input v-model="statsOn" type="checkbox" /> Статистика по кадрам
        </label>
        <button class="ghost" @click="chosen = [...DEFAULT]">По умолчанию</button>
        <button class="ghost" @click="chosen = [...ALL.map((d) => d.id), ...PAIR.map((d) => d.id)]">Все</button>
        <button class="ghost" @click="chosen = []">Ничего</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.strip { display: flex; flex-direction: column; gap: 4px; position: relative; }
.row {
  display: flex;
  align-items: center;
  gap: 4px 16px;
  flex-wrap: wrap;
  padding: 6px 10px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-left: 3px solid var(--c, var(--line));
  border-radius: 6px;
  min-height: 38px;
}
.row.ch1 { --c: var(--ch1); }
.row.ch2 { --c: var(--ch2); }
.row.pair { --c: var(--accent); }
.row.math { --c: var(--math); }
.fn { font-size: 12px; color: var(--muted); min-width: 74px; }
.tag { font-weight: 600; color: var(--c); width: 30px; white-space: nowrap; }
.item { display: flex; flex-direction: column; min-width: 74px; }
.k { font-size: 11px; line-height: 14px; color: var(--muted); white-space: nowrap; }
.ab { color: var(--dim); font-size: 10px; }
.v { font-size: 13px; line-height: 17px; white-space: nowrap; }
.s { font-size: 11px; line-height: 15px; color: var(--muted); white-space: nowrap; }
.legend { display: flex; flex-direction: column; min-width: 40px; margin-left: -8px; }
.legend .v, .legend .s { font-size: 11px; color: var(--dim); }
.frames { font-size: 11px; color: var(--muted); }
.hint { font-size: 12px; }
.end { margin-left: auto; display: flex; align-items: center; gap: 8px; }
.clip {
  font-size: 11px;
  color: var(--warn);
  background: var(--warn-bg);
  border-radius: 4px;
  padding: 2px 7px;
}
.cfg { font-size: 16px; line-height: 1; padding: 4px 6px; }
.cfg.on { background: var(--panel-3); color: var(--text); }
.empty { font-size: 12px; }
.menu {
  position: fixed;
  z-index: 20;
  overflow-y: auto;
  background: var(--panel-2);
  border: 1px solid var(--line-2);
  border-radius: 10px;
  box-shadow: 0 12px 32px rgba(0, 0, 0, .45);
  padding: 12px 14px 8px;
}
.cols { display: flex; gap: 22px; }
.col { display: flex; flex-direction: column; gap: 3px; min-width: 130px; }
.gname { font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); margin-bottom: 4px; }
.col label { display: flex; align-items: center; gap: 7px; font-size: 13px; cursor: pointer; padding: 2px 0; white-space: nowrap; }
.note { font-size: 11px; max-width: 170px; margin: 6px 0 0; line-height: 1.35; }
.stats { display: flex; align-items: center; gap: 6px; font-size: 12px; margin-right: auto; cursor: pointer; }
.actions { display: flex; gap: 4px; justify-content: flex-end; border-top: 1px solid var(--line); margin-top: 10px; padding-top: 6px; }
.actions button { font-size: 12px; }
</style>
