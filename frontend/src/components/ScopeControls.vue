<script setup lang="ts">
import { computed, ref } from 'vue'
import { TIMEBASES, eng, signed } from '../format'
import { hasV } from '../cursors'
import { MATH_OPS, MATH_SCALES, mathUnit, type MathOp } from '../math'
import { PERSIST_MODES, type PersistMode } from '../persist'
import { refFromCSV } from '../ref'
import { cursors, display, fitMathToScreen, frame, mathTrace, persistEpoch, reference, resetCursors, saveReference, scope, set, setTrigLevel } from '../state'
import ChannelCard from './ChannelCard.vue'
import ListStepper from './ListStepper.vue'
import Seg from './Seg.vue'
import Toggle from './Toggle.vue'

defineProps<{ disabled: boolean }>()

const src = computed(() => (scope['trig.source'] || 'CH1').toLowerCase())
const srcCh = computed(() => frame.value?.channels.find((c) => c.name.toLowerCase() === src.value))
const vdiv = computed(() => srcCh.value?.voltsPerDiv || 1)
const srcOffset = computed(() => parseFloat(scope[`${src.value}.offset`]) || 0)
const level = computed(() => parseFloat(scope['trig.level']) || 0)
// The level slider covers the screen height of the source channel.
const levelMin = computed(() => (-4 - srcOffset.value) * vdiv.value)
const levelMax = computed(() => (4 - srcOffset.value) * vdiv.value)
// The device offset is positive when the trigger point is left of the
// centre; the slider shows where the trigger point is, so it runs the other way.
const hoff = computed(() => parseFloat(scope['hor.offset']) || 0)
const delay = computed(() => hoff.value * (frame.value?.timeDiv ?? 0))

const setLevel = setTrigLevel

function levelToMiddle() {
  const m = srcCh.value?.meas
  if (m?.max != null && m.min != null) setLevel((m.max + m.min) / 2)
}

const SOURCES = [{ value: 'CH1', label: 'CH1' }, { value: 'CH2', label: 'CH2' }]
const EDGES = [{ value: 'RISE', label: '↗ Фронт' }, { value: 'FALL', label: '↘ Спад' }]
const SWEEPS = [
  { value: 'AUTO', label: 'Авто', title: 'Развёртка идёт и без синхронизации' },
  { value: 'NORMal', label: 'Ждущий', title: 'Кадр обновляется только по синхронизации' },
  { value: 'SINGle', label: 'Однократ.', title: 'Один кадр по первой синхронизации, затем стоп' },
]
const COUPLING = [{ value: 'DC', label: 'DC' }, { value: 'AC', label: 'AC' }]
const MODES = [{ value: 'SAMPle', label: 'Выборка' }, { value: 'PEAK', label: 'Пиковый' }]
const DEPTHS = [{ value: '4K', label: '4K' }, { value: '8K', label: '8K' }]
const AVERAGES = [
  { value: '1', label: 'Выкл', title: 'Каждый кадр как есть' },
  { value: '4', label: '4', title: 'Среднее примерно четырёх последних кадров' },
  { value: '16', label: '16', title: 'Среднее примерно шестнадцати последних кадров' },
  { value: '64', label: '64', title: 'Среднее примерно 64 последних кадров: шума меньше всего, но изменения видны через несколько секунд' },
]
const CURSOR_MODES = [
  { value: 'off', label: 'Выкл' },
  { value: 't', label: 'Время', title: 'Две вертикальные линии: интервал, частота и напряжения каналов в этих точках' },
  { value: 'v', label: 'Напряж.', title: 'Две горизонтальные линии: уровни выбранного канала и разница между ними' },
  { value: 'tv', label: 'Оба', title: 'Обе пары линий' },
]
const CURSOR_SOURCES = [{ value: 'ch1', label: 'CH1' }, { value: 'ch2', label: 'CH2' }]
const SCREENS = [
  { value: 'yt', label: 'Y‑T', title: 'Напряжение от времени' },
  { value: 'xy', label: 'X‑Y', title: 'CH1 по горизонтали, CH2 по вертикали: фигуры Лиссажу, петли' },
]

const mathOptions = computed(() => MATH_SCALES.map((v) => ({ value: v, cmd: String(v), label: eng(v, mathUnit(display.math), 3, true) })))
const bothOn = computed(() => scope['ch1.display'] === 'ON' && scope['ch2.display'] === 'ON')

// Turning the math on fits it to the screen once; the scale then stays.
function setMath(op: string) {
  const was = display.math
  display.math = op as MathOp
  if (was === 'off' && op !== 'off') fitMathToScreen()
}

const canSave = computed(() => !!frame.value?.channels.some((c) => c.on && c.raw?.length))
const refSummary = computed(() => {
  const r = reference.value
  if (!r) return ''
  const what = r.timeDiv ? `${eng(r.timeDiv, 'с', 3, true)}/дел` : `${eng(r.t[r.t.length - 1] - r.t[0], 'с', 3)}, точек: ${r.t.length}`
  return `${r.traces.map((c) => c.name).join(', ')} · ${r.source} · ${what}`
})
const refError = ref('')
const fileInput = ref<HTMLInputElement>()

async function loadCSV(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  const r = refFromCSV(await file.text(), file.name)
  if (typeof r === 'string') {
    refError.value = r
    return
  }
  refError.value = ''
  reference.value = r
  display.ref = true
}
</script>

<template>
  <div class="controls">
    <ChannelCard ch="ch1" :disabled="disabled" />
    <ChannelCard ch="ch2" :disabled="disabled" />

    <section class="math" :class="{ off: display.math === 'off' }">
      <h3>Математика</h3>
      <div class="grid">
        <span class="lbl">Функция</span>
        <Seg :options="MATH_OPS" :model-value="display.math" @update:model-value="setMath" />
        <template v-if="display.math !== 'off'">
          <span class="lbl">Масштаб</span>
          <ListStepper :options="mathOptions" :model-value="String(display.mathScale)" unit="/дел" @update:model-value="display.mathScale = +$event" />
          <span class="lbl">Смещение</span>
          <div class="slider">
            <input
              type="range" min="-4" max="4" step="0.04"
              style="--c: var(--math)"
              :value="display.mathOffset"
              @input="display.mathOffset = +($event.target as HTMLInputElement).value"
            />
            <span class="mono val">{{ signed(display.mathOffset) }}</span>
            <button class="ghost zero" :disabled="display.mathOffset === 0" title="На центр" @click="display.mathOffset = 0">0</button>
          </div>
          <span />
          <button v-if="bothOn" class="mid" :disabled="!mathTrace" title="Подобрать масштаб и смещение под сигнал" @click="fitMathToScreen">Подогнать</button>
          <span v-else class="lbl">Включите оба канала</span>
        </template>
      </div>
    </section>

    <section class="ref">
      <div class="head">
        <h3>Опорная</h3>
        <Toggle v-if="reference" v-model="display.ref" color="var(--text)" title="Показать на экране" />
      </div>
      <p v-if="reference" class="src" :title="refSummary">{{ refSummary }}</p>
      <p v-if="refError" class="err">{{ refError }}</p>
      <div class="row">
        <button class="mid" :disabled="!canSave" title="Запомнить каналы и математику с экрана" @click="saveReference(); refError = ''">Запомнить</button>
        <button class="mid" title="Загрузить из файла CSV: время в секундах в первом столбце, вольты в остальных" @click="fileInput?.click()">Из CSV…</button>
        <button class="mid ghost" :disabled="!reference" @click="reference = null; refError = ''">Убрать</button>
      </div>
      <input ref="fileInput" type="file" accept=".csv,.txt,text/csv,text/plain" hidden @change="loadCSV" />
    </section>

    <section>
      <h3>Развёртка</h3>
      <div class="grid">
        <span class="lbl">Время</span>
        <ListStepper :options="TIMEBASES" :model-value="scope['hor.scale']" unit="/дел" :disabled="disabled" @update:model-value="set('hor.scale', $event)" />
        <span class="lbl">Сдвиг</span>
        <div class="slider" title="Положение точки синхронизации (▼ над экраном). Справа показана задержка центра экрана от неё">
          <input type="range" min="-6" max="6" step="0.04" :value="-hoff" :disabled="disabled" @input="set('hor.offset', (-($event.target as HTMLInputElement).value).toFixed(2))" />
          <span class="mono val wide">{{ hoff ? eng(delay, 'с', 3) : '0' }}</span>
          <button class="ghost zero" :disabled="disabled || hoff === 0" title="На центр" @click="set('hor.offset', '0')">0</button>
        </div>
        <span class="lbl">Сбор</span>
        <Seg :options="MODES" :model-value="scope['acq.mode']" :disabled="disabled" @update:model-value="set('acq.mode', $event)" />
        <span class="lbl">Память</span>
        <Seg :options="DEPTHS" :model-value="scope['acq.depmem']" :disabled="disabled" @update:model-value="set('acq.depmem', $event)" />
        <span class="lbl" title="Усреднение кадров: программа складывает кадры и убирает случайный шум">Усреднение</span>
        <Seg :options="AVERAGES" :model-value="String(display.avg)" @update:model-value="display.avg = +$event" />
        <span class="lbl">Экран</span>
        <Seg :options="SCREENS" :model-value="display.xy ? 'xy' : 'yt'" @update:model-value="display.xy = $event === 'xy'" />
        <span class="lbl" title="Послесвечение: следы прошлых кадров остаются на экране, яркость показывает, как часто сигнал там бывает">Послесв.</span>
        <div class="persist">
          <Seg :options="PERSIST_MODES" :model-value="display.persist" @update:model-value="display.persist = $event as PersistMode" />
          <button v-if="display.persist !== 'off'" class="ghost zero" title="Стереть следы" @click="persistEpoch++">⟲</button>
        </div>
      </div>
    </section>

    <section>
      <h3>Синхронизация</h3>
      <div class="grid">
        <span class="lbl">Источник</span>
        <Seg :options="SOURCES" :model-value="scope['trig.source']" :disabled="disabled" @update:model-value="set('trig.source', $event)" />
        <span class="lbl">Перепад</span>
        <Seg :options="EDGES" :model-value="scope['trig.edge']" :disabled="disabled" @update:model-value="set('trig.edge', $event)" />
        <span class="lbl">Режим</span>
        <Seg :options="SWEEPS" :model-value="scope['trig.sweep']" :disabled="disabled" @update:model-value="set('trig.sweep', $event)" />
        <span class="lbl">Связь</span>
        <Seg :options="COUPLING" :model-value="scope['trig.coupling']" :disabled="disabled" @update:model-value="set('trig.coupling', $event)" />
        <span class="lbl">Уровень</span>
        <div class="slider">
          <input
            type="range"
            :min="levelMin" :max="levelMax" :step="vdiv / 25"
            :style="{ '--c': `var(--${src})` }"
            :value="level"
            :disabled="disabled"
            @input="setLevel(+($event.target as HTMLInputElement).value)"
          />
          <span class="mono val wide">{{ eng(level, 'В', 3) }}</span>
        </div>
        <span />
        <button class="mid" :disabled="disabled || !srcCh?.meas?.pp" @click="levelToMiddle">Уровень на середину сигнала</button>
      </div>
    </section>

    <section>
      <h3>Курсоры</h3>
      <div class="grid">
        <span class="lbl">Показать</span>
        <Seg v-model="cursors.mode" :options="CURSOR_MODES" />
        <template v-if="hasV(cursors.mode)">
          <span class="lbl">Канал</span>
          <Seg v-model="cursors.src" :options="CURSOR_SOURCES" />
        </template>
        <template v-if="cursors.mode !== 'off'">
          <span />
          <button class="mid" title="Линии тянутся мышью прямо на экране. Эта кнопка возвращает их на исходные места" @click="resetCursors">Вернуть на место</button>
        </template>
      </div>
    </section>
  </div>
</template>

<style scoped>
.controls { display: flex; flex-direction: column; gap: 10px; }
section {
  border: 1px solid var(--line);
  border-radius: var(--radius);
  padding: 8px 10px 10px;
  background: var(--panel);
}
section.math { border-left: 3px solid var(--math); }
section.math.off h3 { margin-bottom: 8px; }
h3 { margin: 0 0 8px; font-size: 12px; font-weight: 600; color: var(--muted); text-transform: uppercase; letter-spacing: .06em; }
.grid { display: grid; grid-template-columns: 70px 1fr; align-items: center; gap: 7px 8px; }
.lbl { color: var(--muted); font-size: 12px; }
.slider { display: flex; align-items: center; gap: 6px; }
.slider input { flex: 1; min-width: 0; }
.val { width: 44px; text-align: right; font-size: 12px; }
.val.wide { width: 76px; }
.zero { padding: 2px 6px; font-size: 12px; }
.mid { font-size: 12px; justify-self: start; }
.persist { display: flex; align-items: center; gap: 6px; min-width: 0; }
.head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px; }
.head h3 { margin: 0; }
.row { display: flex; gap: 6px; flex-wrap: wrap; }
.src { margin: 0 0 8px; font-size: 12px; color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.err { margin: 0 0 8px; font-size: 12px; color: var(--err); }
</style>
