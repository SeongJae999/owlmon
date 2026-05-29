import axios from 'axios'

export interface DPMInstance {
  id: number
  name: string
  db_type: string
  host: string
  port: number
  username: string
  database: string
  poll_interval_sec: number
  enabled: boolean
  password?: string
}

export interface DPMMetrics {
  instance_id: number
  connections_active: number
  connections_idle: number
  connections_max: number
  cache_hit_ratio: number
  db_size_bytes: number
  error?: string   // 치명적 (연결 실패 등) — 빨간 카드
  notice?: string  // 정보성 (확장 미설치 등) — 일반 카드 + 안내
  collected_at: string
}

export interface DPMQueryStat {
  instance_id: number
  query_id: string
  query_text: string
  calls: number
  total_time_ms: number
  mean_time_ms: number
  max_time_ms: number
  rows_returned: number
  collected_at: string
}

export interface DPMStatusItem {
  instance: DPMInstance
  metrics?: DPMMetrics
}

export async function listDPMInstances(): Promise<DPMInstance[]> {
  const res = await axios.get('/api/dpm/instances')
  return res.data
}

export async function getDPMStatus(): Promise<DPMStatusItem[]> {
  const res = await axios.get('/api/dpm/status')
  return res.data
}

export async function addDPMInstance(i: Partial<DPMInstance>): Promise<DPMInstance> {
  const res = await axios.post('/api/dpm/instances', i)
  return res.data
}

export async function deleteDPMInstance(id: number): Promise<void> {
  await axios.delete(`/api/dpm/instances/${id}`)
}

export async function getDPMQueries(id: number): Promise<DPMQueryStat[]> {
  const res = await axios.get(`/api/dpm/instances/${id}/queries`)
  return res.data
}

export async function getDPMMetricsHistory(id: number, hours = 1): Promise<DPMMetrics[]> {
  const res = await axios.get(`/api/dpm/instances/${id}/metrics?hours=${hours}`)
  return res.data
}

export async function triggerDPMCheck(id: number): Promise<DPMMetrics> {
  const res = await axios.post(`/api/dpm/instances/${id}/check`)
  return res.data
}
