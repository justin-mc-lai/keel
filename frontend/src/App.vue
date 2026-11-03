<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime'
import { Providers, Sessions, History, NewSession, Send, Cancel } from '../wailsjs/go/desktop/App'

type ProviderInfo = { name: string; defaultModel: string; models: string[]; available: boolean }
type Msg = { role: string; content: string; reasoning?: string }

type StreamEvent = {
  type: 'reasoning' | 'content' | 'tool_call' | 'tool_start' | 'tool_done' | 'usage' | 'done' | 'error'
  text?: string
  toolCall?: { id: string; name: string; arguments: string }
  usage?: { prompt_tokens: number; completion_tokens: number; total_tokens: number }
}

const providers = ref<ProviderInfo[]>([])
const provider = ref('')
const model = ref('')
const sessionId = ref('')
const sessions = ref<{ id: string; title: string; updatedAt: number }[]>([])
const messages = ref<Msg[]>([])
const input = ref('')
const busy = ref(false)
const status = ref('')

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

onMounted(async () => {
  EventsOn('chat:event', onStream)
  providers.value = (await Providers()) ?? []
  const ready = providers.value.find((p) => p.available) ?? providers.value[0]
  if (ready) {
    provider.value = ready.name
    model.value = ready.defaultModel
  }
  await loadSessions()
})

onUnmounted(() => EventsOff('chat:event'))

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
    await Send(sessionId.value, provider.value, model.value, text)
  } catch (err) {
    status.value = String(err)
    busy.value = false
    if (messages.value[messages.value.length - 1]?.role === 'assistant') {
      messages.value[messages.value.length - 1].content += `\n[error] ${err}`
    }
  } finally {
    busy.value = false
    await loadSessions()
  }
}

function stop() {
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
