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

// 401 응답 시 자동 로그아웃
axios.interceptors.response.use(
  (res) => res,
  (err) => {
    if (err.response?.status === 401) {
      removeToken()
      window.location.href = '/'
    }
    return Promise.reject(err)
  }
)
