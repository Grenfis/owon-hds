<script setup lang="ts">
import { computed } from 'vue'
import { eng } from '../format'
import type { Frame } from '../types'

const props = defineProps<{ frame: Frame | null }>()

const rows = computed(() =>
  (props.frame?.channels ?? [])
    .filter((c) => c.on && c.raw?.length)
    .map((c) => {
      const m = c.meas
      return {
        name: c.name,
        clipped: m.clipped,
        items: [
          ['Частота', eng(m.freq, 'Гц', 5)],
          ['Период', eng(m.period, 'с', 4)],
          ['Размах', eng(m.pp, 'В', 3)],
          ['Макс', eng(m.max, 'В', 3)],
          ['Мин', eng(m.min, 'В', 3)],
          ['Среднее', eng(m.avg, 'В', 3)],
          ['СКЗ', eng(m.rms, 'В', 3)],
          ['Заполн.', m.duty == null ? '—' : `${m.duty.toFixed(1)} %`],
        ],
      }
    }),
)
</script>

<template>
  <div class="strip">
    <div v-for="r in rows" :key="r.name" class="row" :class="r.name.toLowerCase()">
      <span class="tag">{{ r.name }}</span>
      <div v-for="[k, v] in r.items" :key="k" class="item">
        <span class="k">{{ k }}</span>
        <span class="v mono">{{ v }}</span>
      </div>
      <span v-if="r.clipped" class="clip" title="Часть сигнала за краем экрана: размах и амплитуда занижены. Увеличьте В/дел.">за экраном</span>
    </div>
    <div v-if="!rows.length" class="row empty muted">Измерения появятся, когда придут данные с прибора</div>
  </div>
</template>

<style scoped>
.strip { display: flex; flex-direction: column; gap: 4px; }
.row {
  display: flex;
  align-items: center;
  gap: 6px 18px;
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
.tag { font-weight: 600; color: var(--c); width: 30px; }
.item { display: flex; flex-direction: column; min-width: 78px; }
.k { font-size: 11px; color: var(--muted); }
.v { font-size: 13px; }
.clip {
  margin-left: auto;
  font-size: 11px;
  color: var(--warn);
  background: var(--warn-bg);
  border-radius: 4px;
  padding: 2px 7px;
}
.empty { font-size: 12px; }
</style>
