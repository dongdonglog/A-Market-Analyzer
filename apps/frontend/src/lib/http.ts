import axios from 'axios'
import { clearAuth } from './auth'

type AlertContent = {
  title: string
  description?: string
}

export function handleUnauthorized() {
  clearAuth()
  if (window.location.pathname !== '/login') {
    window.location.href = '/login'
  }
}

export function isUnauthorizedError(error: unknown) {
  if (axios.isAxiosError(error)) {
    return error.response?.status === 401
  }
  if (error instanceof Error) {
    return /401|unauthorized|missing bearer token|token revoked/i.test(error.message)
  }
  return false
}

export function buildQueryErrorAlert(
  error: unknown,
  options?: { dataSourceName?: string },
): AlertContent {
  if (isUnauthorizedError(error)) {
    return {
      title: '登录已失效',
      description: '当前登录状态不可用，正在跳回登录页。请重新登录后再试。',
    }
  }

  if (axios.isAxiosError(error)) {
    const status = error.response?.status
    const backendMessage = typeof error.response?.data?.error === 'string'
      ? error.response.data.error
      : ''

    if (!error.response) {
      return {
        title: '后端服务不可用',
        description: '请确认 api-go 与 ai-go 已经启动，并且本地 8080/8081 端口可访问。',
      }
    }

    if (backendMessage.toLowerCase().includes('eastmoney')) {
      return {
        title: '行情数据暂时不可用',
        description: '外部行情接口当前没有返回可用数据，已优先使用本地缓存数据。',
      }
    }

    if (status && status >= 500) {
      return {
        title: '后端服务异常',
        description: backendMessage || '服务端处理请求失败，请查看后端日志。',
      }
    }

    if (status && status >= 400) {
      return {
        title: '请求失败',
        description: backendMessage || '请求参数或当前状态不满足接口要求。',
      }
    }
  }

  return {
    title: options?.dataSourceName ? `${options.dataSourceName}加载失败` : '数据加载失败',
    description: '请稍后重试，若持续失败请查看前端控制台与后端日志。',
  }
}
