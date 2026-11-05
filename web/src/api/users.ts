import axios from 'axios'

export interface User {
  id: number
  username: string
  role: 'admin' | 'viewer'
  disabled: boolean
  created_at: string
}

export async function listUsers(): Promise<User[]> {
  const res = await axios.get('/api/users')
  return res.data
}

export async function createUser(u: { username: string; password: string; role: User['role'] }): Promise<User> {
  const res = await axios.post('/api/users', u)
  return res.data
}

export async function updateUser(id: number, u: { role: User['role']; disabled: boolean; password?: string }): Promise<void> {
  await axios.patch(`/api/users/${id}`, u)
}

// 서버 검증 메시지(text/plain)를 화면에 그대로 보여주기 위한 헬퍼
export function errorText(e: any, fallback: string): string {
  const d = e?.response?.data
  return typeof d === 'string' && d.trim() ? d.trim() : fallback
}
