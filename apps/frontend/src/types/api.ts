export interface LoginPayload {
  email: string
  password: string
}

export interface LoginResponse {
  token: string
  user: {
    id: string
    email: string
    is_admin: boolean
  }
}

export interface SymbolRecord {
  symbol: string
  name: string
  market: string
  source: string
}

export interface OHLCRow {
  symbol: string
  market: string
  date: string
  open: number
  high: number
  low: number
  close: number
  volume: number
  amount?: number
  change_rate?: number
}

export interface CopilotQueryPayload {
  symbol: string
  start_date?: string
  end_date?: string
  provider?: string
  provider_api_key?: string
  question: string
  history: Array<{
    role: string
    content: string
  }>
}

export interface CopilotResponse {
  session_id?: string
  session_date?: string
  answer: string
  bias: 'bullish' | 'bearish' | 'neutral' | string
  key_points: string[]
  risk_points: string[]
  watch_items: string[]
  levels: {
    support: {
      value: number
      reason: string
    }
    pressure: {
      value: number
      reason: string
    }
    risk: {
      value: number
      reason: string
    }
  }
  news_context: {
    used: boolean
    count: number
    note?: string
    items: Array<{
      title: string
      source: string
      published_at: string
      url: string
      summary: string
      relevance_reason: string
    }>
  }
}

export interface ChatMessage {
  role: 'user' | 'assistant' | string
  content: string
}

export interface CopilotSessionSummary {
  id: string
  session_date: string
  symbol: string
  message_count: number
  updated_at: string
  title: string
  summary: string
  is_favorite: boolean
}

export interface CopilotSessionMessagesResponse {
  session: CopilotSessionSummary
  messages: ChatMessage[]
}

export interface AIProviderInfo {
  id: string
  name: string
  model: string
  enabled: boolean
  is_default: boolean
}
