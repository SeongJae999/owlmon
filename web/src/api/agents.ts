import axios from 'axios'

export interface Agent {
  id: number
  name: string
  status: 'active' | 'blocked' | 'pending'
  created_at: string
  last_seen: string | null
}

export interface AgentsInfo {
  agents: Agent[]
  legacy_shared_key: boolean // 공용 키 전환 설정이 켜져 있음 (그 키로 온 데이터는 장비 이름 확인 불가)
  cert: { spki_pin: string; sha256: string; self_signed: boolean } | null // 웹 인증서 지문 (개발 환경이면 null)
}

export async function getAgents(): Promise<AgentsInfo> {
  const res = await axios.get('/api/agents')
  return res.data
}

export async function issueEnrollToken(name: string): Promise<{ token: string; name: string; expires_at: string }> {
  const res = await axios.post('/api/agent/enroll-tokens', { name })
  return res.data
}

export async function revokeAgent(id: number): Promise<void> {
  await axios.post(`/api/agents/${id}/status`, { status: 'blocked' })
}
