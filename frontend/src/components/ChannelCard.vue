<script setup lang="ts">
import { computed } from 'vue'
import { probeFactor, signed, vdivOptions } from '../format'
import { scope, set } from '../state'
import ListStepper from './ListStepper.vue'
import Seg from './Seg.vue'
import Toggle from './Toggle.vue'

const props = defineProps<{ ch: 'ch1' | 'ch2'; disabled: boolean }>()

const k = (s: string) => `${props.ch}.${s}`
const on = computed(() => scope[k('display')] === 'ON')
const options = computed(() => vdivOptions(probeFactor(scope[k('probe')])))
const offset = computed(() => parseFloat(scope[k('offset')]) || 0)
const color = computed(() => `var(--${props.ch})`)

const COUPLING = [
  { value: 'DC', label: 'DC', title: 'Открытый вход' },
  { value: 'AC', label: 'AC', title: 'Закрытый вход: без постоянной составляющей' },
  { value: 'GND', label: 'Земля', title: 'Вход замкнут на землю' },
]
const PROBES = ['1X', '10X', '100X', '1000X'].map((p) => ({ value: p, label: p.replace('X', '×') }))
</script>

<template>
  <div class="card" :class="{ off: !on }" :style="{ '--c': color }">
    <div class="title">
      <span class="name">{{ ch.toUpperCase() }}</span>
      <Toggle :model-value="on" :color="color" :disabled="disabled" @update:model-value="set(k('display'), $event ? 'ON' : 'OFF')" />
    </div>
    <div class="grid">
      <span class="lbl">Масштаб</span>
      <ListStepper :options="options" :model-value="scope[k('scale')]" unit="/дел" :disabled="disabled" @update:model-value="set(k('scale'), $event)" />
      <span class="lbl">Вход</span>
      <Seg :options="COUPLING" :model-value="scope[k('coupling')]" :disabled="disabled" @update:model-value="set(k('coupling'), $event)" />
      <span class="lbl">Щуп</span>
      <Seg :options="PROBES" :model-value="scope[k('probe')]" :disabled="disabled" @update:model-value="set(k('probe'), $event)" />
      <span class="lbl">Смещение</span>
      <div class="slider">
        <input
          type="range" min="-4" max="4" step="0.04"
          :style="{ '--c': color }"
          :value="offset"
          :disabled="disabled"
          @input="set(k('offset'), (+($event.target as HTMLInputElement).value).toFixed(2))"
        />
        <span class="mono val">{{ signed(offset) }}</span>
        <button class="ghost zero" :disabled="disabled || offset === 0" title="На центр" @click="set(k('offset'), '0')">0</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.card {
  border: 1px solid var(--line);
  border-left: 3px solid var(--c);
  border-radius: var(--radius);
  padding: 8px 10px 10px;
  background: var(--panel);
}
.card.off .grid { opacity: .45; }
.title { display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px; }
.name { font-weight: 600; color: var(--c); letter-spacing: .02em; }
.grid { display: grid; grid-template-columns: 70px 1fr; align-items: center; gap: 7px 8px; }
.lbl { color: var(--muted); font-size: 12px; }
.slider { display: flex; align-items: center; gap: 6px; }
.slider input { flex: 1; min-width: 0; }
.val { width: 44px; text-align: right; font-size: 12px; }
.zero { padding: 2px 6px; font-size: 12px; }
</style>
