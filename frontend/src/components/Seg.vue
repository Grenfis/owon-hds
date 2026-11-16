<script setup lang="ts">
// Segmented control: a row of mutually exclusive buttons.
defineProps<{
  modelValue: string
  options: { value: string; label: string; title?: string }[]
  disabled?: boolean
}>()
defineEmits<{ 'update:modelValue': [value: string] }>()
</script>

<template>
  <div class="seg" :class="{ disabled }">
    <button
      v-for="o in options"
      :key="o.value"
      :class="{ on: o.value.toUpperCase() === modelValue.toUpperCase() }"
      :title="o.title"
      :disabled="disabled"
      @click="$emit('update:modelValue', o.value)"
    >
      {{ o.label }}
    </button>
  </div>
</template>

<style scoped>
.seg {
  display: inline-flex;
  background: var(--bg);
  border: 1px solid var(--line-2);
  border-radius: 6px;
  padding: 2px;
  gap: 2px;
}
.seg button {
  border: 0;
  background: transparent;
  padding: 3px 9px;
  border-radius: 4px;
  color: var(--muted);
  white-space: nowrap;
}
.seg button:hover:not(:disabled) { background: var(--panel-2); color: var(--text); }
.seg button.on { background: var(--panel-3); color: var(--text); box-shadow: inset 0 0 0 1px #3e4657; }
</style>
