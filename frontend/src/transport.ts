// One frontend, two shells.
//
// On the desktop the Wails bindings exist on window.go; in a browser they do
// not, so the same calls go over HTTP. Nothing in App.vue should know which.

type Handler = (payload: any) => void

const wailsApp = (globalThis as any).window?.go?.desktop?.App
export const isDesktopShell = typeof wailsApp?.Providers === 'function'
const isDesktop = isDesktopShell

const handlers: Record<string, Handler[]> = {}

export function EventsOn(name: string, cb: Handler) {
  if (isDesktop) {
    const rt = (globalThis as any).window?.runtime
    if (rt?.EventsOn) rt.EventsOn(name, cb)
    return
  }
  ;(handlers[name] ||= []).push(cb)
}

export function EventsOff(name: string) {
  if (isDesktop) {
    const rt = (globalThis as any).window?.runtime
    if (rt?.EventsOff) rt.EventsOff(name)
    return
  }
  delete handlers[name]
}

function dispatch(name: string, payload: any) {
  for (const cb of handlers[name] ?? []) cb(payload)
}

async function getJSON(path: string) {
  const r = await fetch(path)
  if (!r.ok) throw new Error(`${path}: ${r.status}`)
  return r.json()
}

async function sendJSON(method: string, path: string, body?: unknown) {
  const r = await fetch(path, {
    method,
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body)
  })
  if (r.status === 204) return undefined
  const text = await r.text()
  if (!r.ok) throw new Error(text || `${path}: ${r.status}`)
  return text ? JSON.parse(text) : undefined
}

export const Providers = () => (isDesktop ? wailsApp.Providers() : getJSON('/api/providers'))

export const Sessions = () => (isDesktop ? wailsApp.Sessions() : getJSON('/api/sessions'))

export const SearchSessions = (q: string) =>
  isDesktop ? wailsApp.SearchSessions(q) : getJSON(`/api/sessions?q=${encodeURIComponent(q)}`)

export const History = (id: string) =>
  isDesktop ? wailsApp.History(id) : getJSON(`/api/sessions/${id}/messages`)

export const NewSession = () =>
  isDesktop ? wailsApp.NewSession() : sendJSON('POST', '/api/sessions').then((r: any) => r.id)

export const RenameSession = (id: string, title: string) =>
  isDesktop
    ? wailsApp.RenameSession(id, title)
    : sendJSON('PATCH', `/api/sessions/${id}`, { title })

export const DeleteSession = (id: string) =>
  isDesktop ? wailsApp.DeleteSession(id) : sendJSON('DELETE', `/api/sessions/${id}`)

export const ExportSessionToFile = (id: string) =>
  isDesktop ? wailsApp.ExportSessionToFile(id) : Promise.resolve('')

export const ExportSession = (id: string) =>
  isDesktop
    ? wailsApp.ExportSession(id)
    : fetch(`/api/sessions/${id}/export`).then((r) => r.text())

export const PermissionState = () =>
  isDesktop ? wailsApp.PermissionState() : getJSON('/api/permission')

export const SetPermissionMode = (mode: string) =>
  isDesktop ? wailsApp.SetPermissionMode(mode) : sendJSON('POST', '/api/permission', { mode })

export const ResolveApproval = (id: string, decision: string) =>
  isDesktop
    ? wailsApp.ResolveApproval(id, decision)
    : sendJSON('POST', `/api/approvals/${id}`, { decision })

export const Settings = () => (isDesktop ? wailsApp.Settings() : getJSON('/api/settings'))

export const SaveSettings = (s: unknown) =>
  isDesktop ? wailsApp.SaveSettings(s) : sendJSON('POST', '/api/settings', s)

export const WorkspaceRoot = () =>
  isDesktop ? wailsApp.WorkspaceRoot() : Promise.resolve('(server)')

export const Cancel = (id: string) => (isDesktop ? wailsApp.Cancel(id) : Promise.resolve())

export const Retry = (
  sessionId: string,
  provider: string,
  model: string,
  effort: string
) =>
  isDesktop
    ? wailsApp.Retry(sessionId, provider, model, effort)
    : streamChat({ sessionId, provider, model, effort, retry: true })

export const Send = (
  sessionId: string,
  provider: string,
  model: string,
  effort: string,
  text: string
) =>
  isDesktop
    ? wailsApp.Send(sessionId, provider, model, effort, text)
    : streamChat({ sessionId, provider, model, effort, text })

// streamChat reads the SSE stream and re-emits the same event names the Wails
// shell uses, so App.vue has one code path.
async function streamChat(body: Record<string, unknown>): Promise<string> {
  const resp = await fetch('/api/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  })
  if (!resp.ok || !resp.body) throw new Error(`chat: ${resp.status}`)

  const reader = resp.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let answer = ''

  for (;;) {
    const { value, done } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    let idx: number
    while ((idx = buffer.indexOf('\n\n')) >= 0) {
      const chunk = buffer.slice(0, idx).trim()
      buffer = buffer.slice(idx + 2)
      if (!chunk.startsWith('data: ')) continue
      try {
        const { event, data } = JSON.parse(chunk.slice(6))
        if (event === 'chat:final') answer = data?.answer ?? ''
        else dispatch(event, data)
      } catch {
        // ignore malformed frames
      }
    }
  }
  return answer
}

export const Tickets = () => (isDesktop ? wailsApp.Tickets() : getJSON('/api/tickets'))

export const PanelSnapshot = (ticket?: string) =>
  isDesktop
    ? wailsApp.PanelSnapshot(ticket ?? '')
    : getJSON(`/api/panel${ticket ? `?ticket=${encodeURIComponent(ticket)}` : ''}`)

export const ApplyTicketAction = (id: string, action: string) =>
  isDesktop
    ? wailsApp.ApplyTicketAction(id, action)
    : sendJSON('POST', `/api/tickets/${id}/actions`, { action })
