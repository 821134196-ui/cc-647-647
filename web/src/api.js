const TOKEN_KEY = 'tr_token'
const USER_KEY = 'tr_user'

export function getToken() {
  return localStorage.getItem(TOKEN_KEY) || ''
}
export function getUser() {
  try {
    return JSON.parse(localStorage.getItem(USER_KEY) || 'null')
  } catch {
    return null
  }
}
export function setSession(token, user) {
  localStorage.setItem(TOKEN_KEY, token)
  localStorage.setItem(USER_KEY, JSON.stringify(user))
}
export function clearSession() {
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(USER_KEY)
}

async function request(method, url, body) {
  const headers = { 'Content-Type': 'application/json' }
  const token = getToken()
  if (token) headers.Authorization = 'Bearer ' + token
  const resp = await fetch(url, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined
  })
  let data = null
  const text = await resp.text()
  if (text) {
    try { data = JSON.parse(text) } catch { data = { raw: text } }
  }
  if (!resp.ok) {
    const err = new Error((data && data.error) || `请求失败 (${resp.status})`)
    err.status = resp.status
    err.data = data
    throw err
  }
  return data
}

export const api = {
  get: (u) => request('GET', u),
  post: (u, b) => request('POST', u, b || {})
}

export const ROLE_LABEL = {
  clerk: '录入员',
  judge: '裁判',
  chief: '总裁判'
}
