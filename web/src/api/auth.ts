import axios from 'axios'

const TOKEN_KEY = 'owlmon_token'

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token)
}

export function removeToken() {
  localStorage.removeItem(TOKEN_KEY)
}

export function isLoggedIn(): boolean {
  return !!getToken()
}

export async function login(username: string, password: string): Promise<void> {
  const res = await axios.post('/api/auth/login', { username, password })
  setToken(res.data.token)
}

export interface Me {
  username: string
  role: 'admin' | 'viewer'
  multi_user: boolean
}

export async function getMe(): Promise<Me> {
  const res = await axios.get('/api/auth/me')
  return res.data
}

export async function changePassword(current: string, next: string): Promise<void> {
  await axios.post('/api/auth/password', { current, new: next })
}

export function logout() {
  removeToken()
  window.location.href = '/'
}

// axios 기본 헤더에 JWT 자동 추가
axios.interceptors.request.use((config) => {
  const token = getToken()
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// 401 → 자동 로그아웃 (로그인 요청 자체의 401은 제외 — 새로고침되면 실패 안내가 사라짐)
// 403 → 조회 전용 계정의 변경 시도. 버튼마다 권한 분기를 두지 않고 여기서 한 번에 안내
export const FORBIDDEN_EVENT = 'owlmon:forbidden'
axios.interceptors.response.use(
  (res) => res,
  (err) => {
    const status = err.response?.status
    if (status === 401 && !err.config?.url?.endsWith('/api/auth/login')) {
      removeToken()
      window.location.href = '/'
    }
    if (status === 403) {
      window.dispatchEvent(new CustomEvent(FORBIDDEN_EVENT, {
        detail: typeof err.response.data === 'string' ? err.response.data.trim() : '권한이 없습니다',
      }))
    }
    return Promise.reject(err)
  }
)
