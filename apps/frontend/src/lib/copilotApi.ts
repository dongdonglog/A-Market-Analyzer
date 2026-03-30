import axios from 'axios'
import { getStoredToken } from './auth'
import { handleUnauthorized, isUnauthorizedError } from './http'
import type {
  AIProviderInfo,
  CopilotQueryPayload,
  CopilotResponse,
  CopilotSessionMessagesResponse,
  CopilotSessionSummary,
} from '../types/api'

export const copilotApi = axios.create({
  baseURL: import.meta.env.VITE_COPILOT_API_BASE_URL
    ?? import.meta.env.VITE_API_BASE_URL
    ?? 'http://localhost:8080',
})

copilotApi.interceptors.request.use((config) => {
  const token = getStoredToken()
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

copilotApi.interceptors.response.use(
  (response) => response,
  async (error) => {
    if (error.response?.status === 401) {
      handleUnauthorized()
    }
    return Promise.reject(error)
  },
)

export async function queryCopilot(payload: CopilotQueryPayload) {
  return queryCopilotWithOptions(payload)
}

function resolveCopilotBaseURL() {
  return import.meta.env.VITE_COPILOT_API_BASE_URL
    ?? import.meta.env.VITE_API_BASE_URL
    ?? 'http://localhost:8080'
}

type StreamHandlers = {
  onStart?: (payload: { session_id?: string; session_date?: string }) => void
  onStage?: (payload: { stage: string; message: string }) => void
  onDelta?: (payload: { content: string }) => void
  onError?: (payload: { error?: string }) => void
  onResult?: (payload: CopilotResponse) => void
  onDone?: () => void
}

type RequestOptions = {
  signal?: AbortSignal
}

export async function queryCopilotWithOptions(payload: CopilotQueryPayload, options?: RequestOptions) {
  const response = await copilotApi.post<CopilotResponse>('/copilot/query', payload, {
    signal: options?.signal,
  })
  return response.data
}

export async function streamCopilot(payload: CopilotQueryPayload, handlers: StreamHandlers, options?: RequestOptions) {
  const token = getStoredToken()
  const response = await fetch(`${resolveCopilotBaseURL()}/copilot/stream`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    signal: options?.signal,
    body: JSON.stringify(payload),
  })

  if (!response.ok) {
    let message = 'AI 分析失败'
    try {
      const data = await response.json() as { error?: string }
      if (data.error) {
        message = data.error
      }
    } catch {
      // ignore
    }
    if (response.status === 401 || isUnauthorizedError(new Error(message))) {
      handleUnauthorized()
    }
    throw new Error(message)
  }

  if (!response.body) {
    throw new Error('stream response body is empty')
  }

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  function processBlock(block: string) {
    const lines = block.split('\n')
    const event = lines.find((line) => line.startsWith('event:'))?.slice(6).trim()
    const data = lines
      .filter((line) => line.startsWith('data:'))
      .map((line) => line.slice(5).trim())
      .join('\n')
    if (!event || !data) {
      return
    }

    const payload = JSON.parse(data)
    switch (event) {
      case 'start':
        handlers.onStart?.(payload)
        break
      case 'delta':
        handlers.onDelta?.(payload)
        break
      case 'stage':
        handlers.onStage?.(payload)
        break
      case 'result':
        handlers.onResult?.(payload)
        break
      case 'error':
        handlers.onError?.(payload)
        break
      case 'done':
        handlers.onDone?.()
        break
      default:
        break
    }
  }

  while (true) {
    const { done, value } = await reader.read()
    buffer += decoder.decode(value ?? new Uint8Array(), { stream: !done })

    let separatorIndex = buffer.indexOf('\n\n')
    while (separatorIndex >= 0) {
      const block = buffer.slice(0, separatorIndex).trim()
      buffer = buffer.slice(separatorIndex + 2)
      if (block) {
        processBlock(block)
      }
      separatorIndex = buffer.indexOf('\n\n')
    }

    if (done) {
      break
    }
  }
}

export async function fetchAIProviders() {
  const response = await copilotApi.get<AIProviderInfo[]>('/ai/providers')
  return response.data
}

export async function fetchCopilotSessions(symbol: string) {
  const response = await copilotApi.get<CopilotSessionSummary[]>(
    `/copilot/sessions?symbol=${encodeURIComponent(symbol)}`,
  )
  return response.data
}

export async function fetchCopilotSessionMessages(sessionId: string) {
  const response = await copilotApi.get<CopilotSessionMessagesResponse>(
    `/copilot/sessions/${encodeURIComponent(sessionId)}/messages`,
  )
  return response.data
}

export async function toggleCopilotSessionFavorite(
  sessionId: string,
  isFavorite: boolean,
) {
  const response = await copilotApi.post<{ ok: boolean }>(
    `/copilot/sessions/${encodeURIComponent(sessionId)}/favorite`,
    { is_favorite: isFavorite },
  )
  return response.data
}
