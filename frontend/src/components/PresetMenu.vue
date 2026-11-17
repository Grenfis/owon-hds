<script setup lang="ts">
// The presets menu: apply a preset, keep the settings there are now under a
// name, remove a preset, and move presets to and from a file.
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { describe, merge, parsePresets, presetsJSON } from '../presets'
import { applyPreset, presets, savePreset } from '../state'

defineProps<{ online: boolean }>()
const emit = defineEmits<{ save: [name: string, blob: Blob] }>()

const open = ref(false)
const menu = ref<HTMLDivElement>()
const place = ref<Record<string, string>>({})

// The menu opens below its button, inside the window.
function toggle(e: MouseEvent) {
  const b = (e.currentTarget as HTMLElement).getBoundingClientRect()
  place.value = {
    left: `${Math.max(8, Math.min(b.left, window.innerWidth - 400))}px`,
    top: `${b.bottom + 6}px`,
    maxHeight: `${Math.max(200, window.innerHeight - b.bottom - 20)}px`,
  }
  open.value = !open.value
}

// The menu button toggles the menu itself, so a press on it is not outside.
function outside(e: MouseEvent) {
  const t = e.target as Element
  if (!menu.value?.contains(t) && !t.closest?.('.presets-btn')) open.value = false
}
watch(open, (o) => {
  if (o) window.addEventListener('mousedown', outside)
  else {
    window.removeEventListener('mousedown', outside)
    removing.value = ''
    fileError.value = ''
  }
})
onBeforeUnmount(() => window.removeEventListener('mousedown', outside))

const GEN_KEY = 'owon.presetGen'
const name = ref('')
const withGen = ref(localStorage.getItem(GEN_KEY) !== '0')
watch(withGen, (v) => localStorage.setItem(GEN_KEY, v ? '1' : '0'))
const exists = computed(() => presets.value.some((p) => p.name === name.value.trim()))

function save() {
  const n = name.value.trim()
  if (!n) return
  savePreset(n, withGen.value)
  name.value = ''
}

function apply(i: number) {
  applyPreset(presets.value[i])
  open.value = false
}

// A preset is removed on a second press, the first one asks.
const removing = ref('')
function remove(n: string) {
  if (removing.value !== n) {
    removing.value = n
    return
  }
  presets.value = presets.value.filter((p) => p.name !== n)
  removing.value = ''
}

const date = (t: number) => new Date(t).toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' })

function toFile() {
  emit('save', 'hds272s_presets.json', new Blob([presetsJSON(presets.value)], { type: 'application/json' }))
}

const fileInput = ref<HTMLInputElement>()
const fileError = ref('')
async function fromFile(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  const r = parsePresets(await file.text())
  if (typeof r === 'string') {
    fileError.value = r
    return
  }
  fileError.value = ''
  presets.value = merge(presets.value, r)
}
</script>

<template>
  <button class="presets-btn" :class="{ primary: open }" title="Наборы настроек: каналы, развёртка, синхронизация, генератор, режимы экрана" @click="toggle">Пресеты ▾</button>
  <div v-if="open" ref="menu" class="pmenu" :style="place">
    <p v-if="!presets.length" class="empty">Пресетов нет. Настройте прибор и сохраните настройки под именем.</p>
    <div v-for="(p, i) in presets" :key="p.name" class="item">
      <button class="apply" :disabled="!online" :title="online ? 'Применить' : 'Нет связи с прибором'" @click="apply(i)">
        <span class="name">{{ p.name }}</span>
        <span class="what">{{ describe(p) }}</span>
      </button>
      <span class="date">{{ date(p.at) }}</span>
      <button class="ghost del" :class="{ ask: removing === p.name }" :title="removing === p.name ? 'Нажмите ещё раз, чтобы удалить' : 'Удалить пресет'" @click="remove(p.name)">
        {{ removing === p.name ? 'Удалить?' : '✕' }}
      </button>
    </div>
    <div class="new">
      <input v-model="name" placeholder="Название" maxlength="40" @keydown.enter="save" />
      <label title="Сохранить и настройки генератора: форму, частоту, размах, смещение. Выход в пресет не входит"><input v-model="withGen" type="checkbox" /> генератор</label>
      <button :disabled="!name.trim()" @click="save">{{ exists ? 'Заменить' : 'Сохранить текущие' }}</button>
    </div>
    <div class="files">
      <button class="ghost" :disabled="!presets.length" title="Сохранить все пресеты в файл JSON" @click="toFile">В файл…</button>
      <button class="ghost" title="Добавить пресеты из файла JSON; пресет с тем же именем заменяется" @click="fileInput?.click()">Из файла…</button>
      <span v-if="fileError" class="err">{{ fileError }}</span>
      <input ref="fileInput" type="file" accept=".json,application/json" hidden @change="fromFile" />
    </div>
  </div>
</template>

<style scoped>
.pmenu {
  position: fixed;
  z-index: 20;
  width: 390px;
  overflow-y: auto;
  background: var(--panel-2);
  border: 1px solid var(--line-2);
  border-radius: 10px;
  box-shadow: 0 12px 32px rgba(0, 0, 0, .45);
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 12px;
}
.empty { margin: 4px 6px 8px; color: var(--muted); }
.item { display: flex; align-items: center; gap: 6px; }
.apply { flex: 1; min-width: 0; display: flex; flex-direction: column; align-items: flex-start; gap: 1px; padding: 5px 8px; text-align: left; }
.name { font-weight: 600; color: var(--text); }
.what { color: var(--muted); font-size: 11px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 100%; }
.date { color: var(--dim); font-size: 11px; white-space: nowrap; }
.del { padding: 3px 7px; }
.del.ask { color: var(--err); border-color: #6b2a2f; }
.new { display: flex; align-items: center; gap: 6px; border-top: 1px solid var(--line); padding-top: 8px; margin-top: 4px; }
.new input:not([type]) { flex: 1; min-width: 0; font-size: 12px; padding: 4px 6px; }
.new label { display: flex; align-items: center; gap: 4px; color: var(--muted); white-space: nowrap; cursor: pointer; }
.files { display: flex; align-items: center; gap: 6px; }
.err { color: var(--err); }
</style>
