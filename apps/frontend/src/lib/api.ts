import axios from 'axios'
import { getStoredToken } from './auth'
import { handleUnauthorized } from './http'
import type {
  AdminBatchCreateRedeemCodePayload,
  AdminActionLog,
  AdminCreateRedeemCodePayload,
  AdminGrantCreditsPayload,
  AdminGrantMembershipPayload,
  AdminRedeemCodeClaim,
  AdminRedeemCode,
  AdminUserDetail,
  AdminUserSummary,
  BillingSummary,
  LoginPayload,
  LoginResponse,
  OHLCRow,
  RedeemCodePayload,
  RedeemCodeResponse,
  RechargeOrderPayload,
  RechargeOrderResponse,
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
  const response = await api.get<SymbolRecord[]>(
    refresh ? '/symbols?refresh=1' : '/symbols',
  )
  return response.data
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

export async function deleteSymbol(symbol: string) {
  const response = await api.delete<{ ok: boolean }>(`/symbols/${encodeURIComponent(symbol)}`)
  return response.data
}

export async function fetchBillingSummary() {
  const response = await api.get<BillingSummary>('/billing/summary')
  return response.data
}

export async function createRechargeOrder(payload: RechargeOrderPayload) {
  const response = await api.post<RechargeOrderResponse>('/billing/recharge-orders', payload)
  return response.data
}

export async function mockPayRechargeOrder(orderID: string) {
  const response = await api.post<RechargeOrderResponse>(`/billing/recharge-orders/${encodeURIComponent(orderID)}/mock-pay`)
  return response.data
}

export async function redeemCode(payload: RedeemCodePayload) {
  const response = await api.post<RedeemCodeResponse>('/billing/redeem-codes/redeem', payload)
  return response.data
}

export async function fetchAdminRedeemCodes(params?: { search?: string; reward_type?: string; status?: string }) {
  const response = await api.get<AdminRedeemCode[]>('/admin/redeem-codes', { params })
  return response.data
}

export async function createAdminRedeemCode(payload: AdminCreateRedeemCodePayload) {
  const response = await api.post<AdminRedeemCode>('/admin/redeem-codes', payload)
  return response.data
}

export async function batchCreateAdminRedeemCodes(payload: AdminBatchCreateRedeemCodePayload) {
  const response = await api.post<AdminRedeemCode[]>('/admin/redeem-codes/batch', payload)
  return response.data
}

export async function disableAdminRedeemCode(code: string) {
  const response = await api.post<{ ok: boolean; code: string }>(`/admin/redeem-codes/${encodeURIComponent(code)}/disable`)
  return response.data
}

export async function fetchAdminRedeemCodeClaims(params?: { search?: string }) {
  const response = await api.get<AdminRedeemCodeClaim[]>('/admin/redeem-code-claims', { params })
  return response.data
}

export async function fetchAdminActionLogs(params?: { search?: string }) {
  const response = await api.get<AdminActionLog[]>('/admin/action-logs', { params })
  return response.data
}

export async function fetchAdminUsers(params?: { search?: string; membership_status?: string }) {
  const response = await api.get<AdminUserSummary[]>('/admin/users', { params })
  return response.data
}

export async function fetchAdminUserDetail(userID: string) {
  const response = await api.get<AdminUserDetail>(`/admin/users/${encodeURIComponent(userID)}`)
  return response.data
}

export async function grantAdminUserBonusCredits(userID: string, payload: AdminGrantCreditsPayload) {
  const response = await api.post<{ ok: boolean; credit_balance: number }>(`/admin/users/${encodeURIComponent(userID)}/bonus-credits`, payload)
  return response.data
}

export async function grantAdminUserMembership(userID: string, payload: AdminGrantMembershipPayload) {
  const response = await api.post(`/admin/users/${encodeURIComponent(userID)}/memberships`, payload)
  return response.data
}
