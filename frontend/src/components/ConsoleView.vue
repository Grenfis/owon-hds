<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { call } from '../api'
import { logs, status } from '../state'
import type { ConsoleResult } from '../types'

interface Entry { id: number; cmd: string; reply?: string; kind?: string; bytes?: number; ms?: number; error?: string; time: number }

const entries = ref<Entry[]>([])
const cmd = ref('')
const busy = ref(false)
const showUi = ref(false)
const list = ref<HTMLDivElement>()
const history: string[] = JSON.parse(localStorage.getItem('owon.history') ?? '[]')
let hpos = -1
let nextId = 1

const QUICK = ['*IDN?', ':MODel?', ':TRIGger:STATus?', ':RUNning?', ':DATa:WAVe:SCReen:HEAD?', ':MEASurement:CH1:FREQuency?', ':FUNCtion:FREQuency?', ':DMM:MEAS?']

// Commands the UI sends are shown too when asked, merged by time.
const shown = computed<(Entry & { ui: boolean })[]>(() => {
  const own = entries.value.map((e) => ({ ...e, ui: false }))
  if (!showUi.value) return own
  const ui = logs.value.map((l, i) => ({ id: -i - 1, cmd: l.text, time: l.time, ui: true }))
  return [...own, ...ui].sort((a, b) => a.time - b.time)
})

async function send(text = cmd.value) {
  text = text.trim()
  if (!text || busy.value) return
  history.splice(0, history.length, text, ...history.filter((h) => h !== text).slice(0, 49))
  localStorage.setItem('owon.history', JSON.stringify(history))
  hpos = -1
  cmd.value = ''
  const e: Entry = { id: nextId++, cmd: text, time: Date.now() }
  entries.value.push(e)
  busy.value = true
  try {
    const r = await call<ConsoleResult>('Console', text)
    Object.assign(e, { reply: r.reply, kind: r.kind, bytes: r.bytes, ms: r.ms })
  } catch (err: any) {
    e.error = String(err.message ?? err)
  } finally {
    busy.value = false
    entries.value = [...entries.value]
  }
}

function key(ev: KeyboardEvent) {
  if (ev.key === 'ArrowUp' && hpos < history.length - 1) cmd.value = history[++hpos]
  else if (ev.key === 'ArrowDown') cmd.value = hpos > 0 ? history[--hpos] : ((hpos = -1), '')
  else return
  ev.preventDefault()
}

watch(shown, async () => {
  await nextTick()
  list.value?.scrollTo({ top: list.value.scrollHeight })
})
</script>

<template>
  <div class="console">
    <div class="bar">
      <div class="quick">
        <button v-for="q in QUICK" :key="q" class="ghost mono" :disabled="!status.connected || busy" @click="send(q)">{{ q }}</button>
      </div>
      <label class="muted"><input v-model="showUi" type="checkbox" /> Показывать команды интерфейса</label>
      <button class="ghost" @click="entries = []; logs = []">Очистить</button>
    </div>
    <div ref="list" class="log mono">
      <div v-if="!shown.length" class="empty muted">
        Здесь можно отправить любую команду SCPI. Запрос с «?» ждёт ответа.
        Заводские и калибровочные команды (:SERial, :Selfcorrec, :AUTocalibration, :PRACTISE, :DOWNload, :VIDEO, :VOLFULL) заблокированы.
      </div>
      <div v-for="e in shown" :key="e.id" class="entry" :class="{ ui: e.ui }">
        <div class="req"><span class="arrow">{{ e.ui ? '·' : '›' }}</span>{{ e.cmd }}</div>
        <div v-if="e.error" class="err">{{ e.error }}</div>
        <div v-else-if="e.kind === 'none'" class="meta">отправлено, ответа не ждём</div>
        <template v-else-if="e.reply !== undefined">
          <div class="reply">{{ e.reply || '(пустой ответ)' }}</div>
          <div class="meta">{{ e.kind === 'data' ? `данные, ${e.bytes} байт` : `${e.bytes} символов` }} · {{ e.ms }} мс</div>
        </template>
      </div>
    </div>
    <form class="input" @submit.prevent="send()">
      <span class="prompt mono">›</span>
      <input v-model="cmd" type="text" class="mono" placeholder=":CH1:SCALe?" spellcheck="false" :disabled="!status.connected" @keydown="key" />
      <button class="primary" type="submit" :disabled="!status.connected || busy || !cmd.trim()">{{ busy ? 'Жду ответ…' : 'Отправить' }}</button>
    </form>
  </div>
</template>

<style scoped>
.console { display: flex; flex-direction: column; height: 100%; padding: 12px 14px; gap: 8px; }
.bar { display: flex; align-items: center; gap: 12px; }
.quick { display: flex; flex-wrap: wrap; gap: 2px; flex: 1; }
.quick button { font-size: 12px; padding: 2px 7px; }
.bar label { display: flex; gap: 6px; align-items: center; font-size: 12px; white-space: nowrap; cursor: pointer; }
.log {
  flex: 1;
  overflow: auto;
  background: #090b0f;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  padding: 10px 12px;
  font-size: 12.5px;
  user-select: text;
}
.empty { font-family: var(--sans); font-size: 13px; max-width: 640px; line-height: 1.6; }
.entry { padding: 4px 0; border-bottom: 1px solid #161a22; }
.entry.ui { color: var(--dim); padding: 1px 0; border: 0; }
.req { color: var(--accent); }
.entry.ui .req { color: var(--dim); }
.arrow { display: inline-block; width: 16px; color: var(--dim); }
.reply { padding-left: 16px; white-space: pre-wrap; word-break: break-all; color: var(--text); }
.err { padding-left: 16px; color: var(--err); font-family: var(--sans); }
.meta { padding-left: 16px; color: var(--dim); font-size: 11px; }
.input { display: flex; gap: 8px; align-items: center; }
.prompt { color: var(--accent); font-size: 16px; }
.input input { flex: 1; padding: 7px 10px; font-size: 13px; }
</style>
