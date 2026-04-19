import { getToken, clearToken } from './auth'

class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const token = getToken()
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(init?.headers as Record<string, string>),
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  const res = await fetch(path, { ...init, headers })

  if (res.status === 401) {
    clearToken()
    window.location.href = '/'
    throw new ApiError(401, 'unauthorized')
  }

  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: 'unknown error' }))
    throw new ApiError(res.status, body.error || `HTTP ${res.status}`)
  }

  return res.json()
}

export { apiFetch, ApiError }
