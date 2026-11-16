<script setup lang="ts">
withDefaults(defineProps<{ modelValue: boolean; color?: string; disabled?: boolean }>(), { color: 'var(--accent)' })
defineEmits<{ 'update:modelValue': [value: boolean] }>()
</script>

<template>
  <button
    class="toggle"
    role="switch"
    :aria-checked="modelValue"
    :class="{ on: modelValue }"
    :style="{ '--c': color }"
    :disabled="disabled"
    @click="$emit('update:modelValue', !modelValue)"
  >
    <span class="knob" />
  </button>
</template>

<style scoped>
.toggle {
  width: 34px;
  height: 18px;
  padding: 0;
  border-radius: 9px;
  border: 1px solid var(--line-2);
  background: var(--bg);
  position: relative;
  flex: none;
}
.toggle:hover:not(:disabled) { background: var(--bg); }
.knob {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: var(--dim);
  transition: transform .15s, background .15s;
}
.toggle.on { border-color: var(--c); }
.toggle.on .knob { transform: translateX(16px); background: var(--c); }
</style>
