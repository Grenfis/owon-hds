<script setup lang="ts">
// Picks a value from a fixed list, like a detented knob: step buttons, the
// mouse wheel over the control, or the drop-down list.
import { computed } from 'vue'
import type { Option } from '../format'

const props = defineProps<{ options: Option[]; modelValue: string; unit?: string; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [cmd: string] }>()

const index = computed(() => {
  const i = props.options.findIndex((o) => o.cmd.toLowerCase() === props.modelValue.toLowerCase())
  return i < 0 ? 0 : i
})

function step(d: number) {
  const i = Math.max(0, Math.min(props.options.length - 1, index.value + d))
  if (i !== index.value) emit('update:modelValue', props.options[i].cmd)
}

let wheelAcc = 0
function wheel(e: WheelEvent) {
  if (props.disabled) return
  wheelAcc -= e.deltaMode === 1 ? e.deltaY * 33 : e.deltaY
  const n = Math.trunc(wheelAcc / 100)
  wheelAcc -= n * 100
  if (n) step(n)
}
</script>

<template>
  <div class="stepper" :class="{ disabled }" @wheel.prevent="wheel" title="Колесо мыши меняет значение">
    <button class="ghost" :disabled="disabled || index === 0" @click="step(-1)" aria-label="Меньше">−</button>
    <select
      class="mono"
      :value="options[index]?.cmd"
      :disabled="disabled"
      @change="emit('update:modelValue', ($event.target as HTMLSelectElement).value)"
    >
      <option v-for="o in options" :key="o.cmd" :value="o.cmd">{{ o.label }}{{ unit }}</option>
    </select>
    <button class="ghost" :disabled="disabled || index === options.length - 1" @click="step(1)" aria-label="Больше">+</button>
  </div>
</template>

<style scoped>
.stepper {
  display: inline-flex;
  align-items: center;
  border: 1px solid var(--line-2);
  border-radius: 6px;
  background: var(--bg);
}
.stepper button { padding: 2px 9px; font-size: 15px; line-height: 1; }
.stepper select {
  border: 0;
  background: transparent;
  text-align: center;
  text-align-last: center;
  min-width: 96px;
  padding: 4px 2px;
  font-size: 13px;
  appearance: none;
  cursor: pointer;
}
.stepper select option { background: var(--panel-2); }
</style>
