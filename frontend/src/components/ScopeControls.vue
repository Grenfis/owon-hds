<script setup lang="ts">
import { computed } from 'vue'
import { TIMEBASES, eng } from '../format'
import { frame, scope, set, setTrigLevel } from '../state'
import ChannelCard from './ChannelCard.vue'
import ListStepper from './ListStepper.vue'
import Seg from './Seg.vue'

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
</script>

<template>
  <div class="controls">
    <ChannelCard ch="ch1" :disabled="disabled" />
    <ChannelCard ch="ch2" :disabled="disabled" />

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
h3 { margin: 0 0 8px; font-size: 12px; font-weight: 600; color: var(--muted); text-transform: uppercase; letter-spacing: .06em; }
.grid { display: grid; grid-template-columns: 70px 1fr; align-items: center; gap: 7px 8px; }
.lbl { color: var(--muted); font-size: 12px; }
.slider { display: flex; align-items: center; gap: 6px; }
.slider input { flex: 1; min-width: 0; }
.val { width: 44px; text-align: right; font-size: 12px; }
.val.wide { width: 76px; }
.zero { padding: 2px 6px; font-size: 12px; }
.mid { font-size: 12px; justify-self: start; }
</style>
