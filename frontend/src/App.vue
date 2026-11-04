<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime'
import {
  Providers, Sessions, History, NewSession, Send, Cancel,
  PermissionState, SetPermissionMode, ResolveApproval
} from '../wailsjs/go/desktop/App'

type ProviderInfo = { name: string; defaultModel: string; models: string[]; available: boolean }
type Msg = { role: string; content: string; reasoning?: string }
type PermState = { mode: string; warning: boolean; pending: number }
type Approval = { id: string; sessionId?: string; tool: string; args: string; timeoutSec?: number }

type StreamEvent = {
  type: 'reasoning' | 'content' | 'tool_call' | 'tool_start' | 'tool_done' | 'usage' | 'done' | 'error'
  text?: string
  toolCall?: { id: string; name: string; arguments: string }
  usage?: { prompt_tokens: number; completion_tokens: number; total_tokens: number }
}

const providers = ref<ProviderInfo[]>([])
const provider = ref('')
const model = ref('')
const effort = ref('')
const permission = ref<PermState>({ mode: 'ask', warning: false, pending: 0 })
const approvals = ref<Approval[]>([])
const approval = computed(() => approvals.value[0] ?? null)
const sessionId = ref('')
const sessions = ref<{ id: string; title: string; updatedAt: number }[]>([])
const messages = ref<Msg[]>([])
const input = ref('')
const busy = ref(false)
const status = ref('')

const EFFORTS = [
  { value: '', label: '思考：默认' },
  { value: 'off', label: '思考：关' },
  { value: 'low', label: '思考：低' },
  { value: 'medium', label: '思考：中' },
  { value: 'high', label: '思考：高' }
]

const activeProvider = computed(() => providers.value.find((p) => p.name === provider.value))
const models = computed(() => activeProvider.value?.models ?? [])

function onStream(ev: StreamEvent) {
  const last = messages.value[messages.value.length - 1]
  switch (ev.type) {
    case 'reasoning':
      if (last && last.role === 'assistant') last.reasoning = (last.reasoning ?? '') + (ev.text ?? '')
      break
    case 'content':
      if (last && last.role === 'assistant') last.content += ev.text ?? ''
      break
    case 'tool_start':
      status.value = `running ${ev.toolCall?.name}`
      break
    case 'tool_done':
      status.value = `${ev.toolCall?.name} done`
      break
    case 'usage':
      status.value = `${ev.usage?.total_tokens ?? 0} tokens`
      break
    case 'error':
      status.value = 'error'
      break
    case 'done':
      busy.value = false
      break
  }
}

function onApproval(payload: any) {
  if (payload?.closed) {
    approvals.value = approvals.value.filter((a) => a.id !== payload.id)
    return
  }
  if (!approvals.value.some((a) => a.id === payload.id)) {
    approvals.value.push(payload as Approval)
  }
  refreshPermission()
}

async function refreshPermission() {
  permission.value = (await PermissionState()) as PermState
}

async function changePermission(e: Event) {
  const mode = (e.target as HTMLSelectElement).value
  permission.value = (await SetPermissionMode(mode)) as PermState
}

async function answer(decision: 'allow_once' | 'allow_session' | 'deny') {
  const a = approval.value
  if (!a) return
  approvals.value = approvals.value.filter((x) => x.id !== a.id)
  try {
    await ResolveApproval(a.id, decision)
  } catch (err) {
    status.value = String(err)
  }
  await refreshPermission()
}

onMounted(async () => {
  EventsOn('chat:event', onStream)
  EventsOn('tool:approval', onApproval)
  await refreshPermission()
  providers.value = (await Providers()) ?? []
  const ready = providers.value.find((p) => p.available) ?? providers.value[0]
  if (ready) {
    provider.value = ready.name
    model.value = ready.defaultModel
  }
  await loadSessions()
})

onUnmounted(() => {
  EventsOff('chat:event')
  EventsOff('tool:approval')
})

async function loadSessions() {
  sessions.value = (await Sessions()) ?? []
}

async function newChat() {
  sessionId.value = await NewSession()
  messages.value = []
  await loadSessions()
}

async function openSession(id: string) {
  sessionId.value = id
  const history = (await History(id)) ?? []
  messages.value = history
    .filter((m) => m.role === 'user' || m.role === 'assistant')
    .filter((m) => (m.content ?? '') !== '')
    .map((m) => ({ role: m.role, content: m.content ?? '', reasoning: m.reasoning ?? '' }))
}

async function send() {
  const text = input.value.trim()
  if (!text || busy.value) return
  if (!sessionId.value) await newChat()

  messages.value.push({ role: 'user', content: text })
  messages.value.push({ role: 'assistant', content: '', reasoning: '' })
  input.value = ''
  busy.value = true
  status.value = 'streaming'

  try {
    await Send(sessionId.value, provider.value, model.value, effort.value, text)
  } catch (err) {
    status.value = String(err)
    busy.value = false
    if (messages.value[messages.value.length - 1]?.role === 'assistant') {
      messages.value[messages.value.length - 1].content += `\n[error] ${err}`
    }
  } finally {
    busy.value = false
    approvals.value = approvals.value.filter((a) => a.sessionId !== sessionId.value)
    await refreshPermission()
    await loadSessions()
  }
}

async function stop() {
  while (approvals.value.length > 0) await answer('deny')
  if (sessionId.value) Cancel(sessionId.value)
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    send()
  }
}
</script>

<template>
  <div class="shell">
    <aside class="sidebar">
      <div class="brand">AgentDesk</div>
      <button class="new" @click="newChat">+ New chat</button>
      <ul class="sessions">
        <li
          v-for="s in sessions"
          :key="s.id"
          :class="{ active: s.id === sessionId }"
          @click="openSession(s.id)"
        >
          {{ s.title || 'Untitled' }}
        </li>
      </ul>
    </aside>

    <main class="main">
      <header class="toolbar">
        <select v-model="provider">
          <option v-for="p in providers" :key="p.name" :value="p.name">
            {{ p.name }}{{ p.available ? '' : ' (no key)' }}
          </option>
        </select>
        <select v-model="model">
          <option v-for="m in models" :key="m" :value="m">{{ m }}</option>
        </select>
        <select
          class="perm"
          :class="{ danger: permission.warning }"
          :value="permission.mode"
          @change="changePermission"
        >
          <option value="ask">权限：每次都问</option>
          <option value="allow">权限：本会话放行</option>
          <option value="bypass">权限：全部放行</option>
        </select>
        <select v-model="effort">
          <option v-for="e in EFFORTS" :key="e.value" :value="e.value">{{ e.label }}</option>
        </select>
        <span v-if="permission.warning" class="warn">BYPASS · 工具不再询问</span>
        <span class="status">{{ status }}</span>
      </header>

      <div class="messages">
        <div v-if="messages.length === 0" class="empty">
          <h1>Local, provider-agnostic, single binary.</h1>
          <p>Pick a provider, type a message, watch tokens stream in-process.</p>
        </div>
        <div v-for="(m, i) in messages" :key="i" class="msg" :class="m.role">
          <details v-if="m.reasoning" class="reasoning">
            <summary>thinking</summary>
            <pre>{{ m.reasoning }}</pre>
          </details>
          <div class="bubble">{{ m.content }}</div>
        </div>
      </div>

      <div v-if="approval" class="approval">
        <div class="approval-head">
          <strong>工具请求执行</strong>
          <code>{{ approval.tool }}</code>
          <span v-if="approvals.length > 1" class="queued">还有 {{ approvals.length - 1 }} 个待处理</span>
        </div>
        <pre class="approval-args">{{ approval.args }}</pre>
        <div class="approval-actions">
          <button class="allow" @click="answer('allow_once')">允许一次</button>
          <button class="allow-session" @click="answer('allow_session')">本会话允许此工具</button>
          <button class="deny" @click="answer('deny')">拒绝</button>
        </div>
        <p class="approval-hint">等待中，{{ approval.timeoutSec ?? 120 }} 秒后自动拒绝</p>
      </div>

      <footer class="composer">
        <textarea
          v-model="input"
          placeholder="Message… (Enter to send, Shift+Enter for newline)"
          @keydown="onKeydown"
        />
        <button v-if="!busy" @click="send">Send</button>
        <button v-else class="stop" @click="stop">Stop</button>
      </footer>
    </main>
  </div>
</template>
