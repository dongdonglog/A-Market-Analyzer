import axios from 'axios'
import { getStoredToken } from './auth'
import { handleUnauthorized } from './http'
import type {
  LoginPayload,
  LoginResponse,
  OHLCRow,
  SymbolRecord,
} from '../types/api'

export const api = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080',
})

api.interceptors.request.use((config) => {
  const token = getStoredToken()
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

api.interceptors.response.use(
  (response) => response,
  async (error) => {
    if (error.response?.status === 401) {
      handleUnauthorized()
    }
    return Promise.reject(error)
  },
)

export async function login(payload: LoginPayload) {
  const response = await api.post<LoginResponse>('/auth/login', payload)
  return response.data
}

export async function register(payload: LoginPayload) {
  const response = await api.post<LoginResponse>('/auth/register', payload)
  return response.data
}

export async function logout() {
  const response = await api.post<{ ok: boolean }>('/auth/logout')
  return response.data
}

export async function fetchSymbols(refresh = false) {
  const response = await api.get<SymbolRecord[] | { symbols: SymbolRecord[]; warning?: string }>(
    refresh ? '/symbols?refresh=1' : '/symbols',
  )
  return Array.isArray(response.data) ? response.data : response.data.symbols
}

export async function fetchOHLC(symbol: string, startDate?: string, endDate?: string) {
  const params = new URLSearchParams()
  if (startDate) params.set('start_date', startDate)
  if (endDate) params.set('end_date', endDate)

  const suffix = params.toString() ? `?${params.toString()}` : ''
  const response = await api.get<OHLCRow[]>(
    `/symbols/${encodeURIComponent(symbol)}/ohlc${suffix}`,
  )
  return response.data
}

export async function addSymbol(symbol: string) {
  const response = await api.post<SymbolRecord>('/symbols', { symbol })
  return response.data
}

export async function deleteSymbol(symbol: string) {
  const response = await api.delete<{ ok: boolean }>(`/symbols/${encodeURIComponent(symbol)}`)
  return response.data
}
