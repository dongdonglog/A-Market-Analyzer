import type { LoginResponse } from '../types/api'

const TOKEN_KEY = 'market-copilot-token'
const USER_KEY = 'market-copilot-user'

export function getStoredToken() {
  return window.localStorage.getItem(TOKEN_KEY)
}

export function getStoredUser() {
  const raw = window.localStorage.getItem(USER_KEY)
  if (!raw) {
    return null
  }

  try {
    return JSON.parse(raw) as LoginResponse['user']
  } catch {
    return null
  }
}

export function storeAuth(payload: LoginResponse) {
  window.localStorage.setItem(TOKEN_KEY, payload.token)
  window.localStorage.setItem(USER_KEY, JSON.stringify(payload.user))
}

export function clearAuth() {
  window.localStorage.removeItem(TOKEN_KEY)
  window.localStorage.removeItem(USER_KEY)
}
