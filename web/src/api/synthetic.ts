import axios from 'axios'

export interface SyntheticMonitor {
  id: number
  name: string
  url: string
  method: string
  expected_status: number
  expected_keyword: string
  interval_seconds: number
  timeout_seconds: number
  enabled: boolean
}

export interface SyntheticResult {
  monitor_id: number
  success: boolean
  status_code: number
  response_time_ms: number
  error?: string
  checked_at: string
}

export interface SyntheticStats {
  monitor_id: number
  uptime_pct: number
  avg_latency_ms: number
  p95_latency_ms: number
  total_checks: number
  failed_checks: number
}

export interface SyntheticStatusItem {
  monitor: SyntheticMonitor
  latest?: SyntheticResult
  stats: SyntheticStats
}

export async function listSyntheticMonitors(): Promise<SyntheticMonitor[]> {
  const res = await axios.get('/api/synthetic/monitors')
  return res.data
}

export async function getSyntheticStatus(): Promise<SyntheticStatusItem[]> {
  const res = await axios.get('/api/synthetic/status')
  return res.data
}

export async function addSyntheticMonitor(m: Partial<SyntheticMonitor>): Promise<SyntheticMonitor> {
  const res = await axios.post('/api/synthetic/monitors', m)
  return res.data
}

export async function updateSyntheticMonitor(id: number, m: Partial<SyntheticMonitor>): Promise<void> {
  await axios.put(`/api/synthetic/monitors/${id}`, m)
}

export async function deleteSyntheticMonitor(id: number): Promise<void> {
  await axios.delete(`/api/synthetic/monitors/${id}`)
}

export async function getSyntheticHistory(id: number, limit = 100): Promise<SyntheticResult[]> {
  const res = await axios.get(`/api/synthetic/monitors/${id}/history?limit=${limit}`)
  return res.data
}

export async function triggerSyntheticCheck(id: number): Promise<SyntheticResult> {
  const res = await axios.post(`/api/synthetic/monitors/${id}/check`)
  return res.data
}
