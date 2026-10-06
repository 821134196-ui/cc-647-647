// 极简 API 封装：令牌存 localStorage，403 时抛出后端错误信息
const TOKEN_KEY = 'swim_token'
const USER_KEY = 'swim_user'

export function getToken() {
  return localStorage.getItem(TOKEN_KEY) || ''
}
export function getStoredUser() {
  const raw = localStorage.getItem(USER_KEY)
  return raw ? JSON.parse(raw) : null
}
export function setAuth(token, user) {
  localStorage.setItem(TOKEN_KEY, token)
  localStorage.setItem(USER_KEY, JSON.stringify(user))
}
export function clearAuth() {
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(USER_KEY)
}

export async function api(path, options = {}) {
  const headers = { 'Content-Type': 'application/json', ...(options.headers || {}) }
  const token = getToken()
  if (token) headers.Authorization = 'Bearer ' + token
  const res = await fetch('/api' + path, {
    method: options.method || 'GET',
    headers,
    body: options.body ? JSON.stringify(options.body) : undefined
  })
  const text = await res.text()
  let data = null
  try { data = text ? JSON.parse(text) : null } catch { data = text }
  if (!res.ok) {
    const msg = (data && data.error) || `请求失败(${res.status})`
    const err = new Error(msg)
    err.status = res.status
    throw err
  }
  return data
}

export const ROLE_NAMES = {
  chief: '总裁判',
  judge: '裁判',
  clerk: '录入员',
  device: '电子计时设备'
}

export const DECISION_NAMES = {
  electronic: '采用电子成绩',
  manual: '采用手动成绩',
  swimoff: '采用重赛结果',
  withdraw: '撤回成绩'
}

export const SOURCE_NAMES = { electronic: '电子', manual: '手动', swimoff: '重赛' }

// 秒.百分秒 → 毫秒（输入框允许 51.88 / 1:02.34）
export function parseTimeInput(str) {
  str = (str || '').trim()
  if (!str) return null
  let minutes = 0
  if (str.includes(':')) {
    const [m, rest] = str.split(':')
    minutes = parseInt(m, 10) || 0
    str = rest
  }
  if (!/^\d+(\.\d+)?$/.test(str)) return null
  return Math.round((minutes * 60 + parseFloat(str)) * 1000)
}
