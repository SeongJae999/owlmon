import axios from 'axios'

export interface AlertConfig {
  enabled: boolean
  recipients: string[]
  cpu_threshold: number
  mem_threshold: number
  disk_warn: number
  disk_crit: number
}

export async function getAlertConfig(): Promise<AlertConfig> {
  const res = await axios.get('/api/alert/config')
  return res.data
}

export async function setAlertConfig(cfg: AlertConfig): Promise<AlertConfig> {
  const res = await axios.post('/api/alert/config', cfg)
  return res.data
}

// 이메일 송신 상태 (SMTP 환경변수 + 수신자 등록)
export interface EmailStatus {
  smtp_configured: boolean
  recipients_count: number
  healthy: boolean
  issues: string[]
}

export async function getEmailStatus(): Promise<EmailStatus> {
  const res = await axios.get('/api/alert/email-status')
  return res.data
}

export interface AlertRecord {
  id: number
  sent_at: string
  host: string
  category: string
  severity: string
  subject: string
  body: string
}

export interface AlertHistoryFilter {
  from?: string
  to?: string
  severity?: string
  host?: string
  limit?: number
}

export async function getAlertHistory(filter: AlertHistoryFilter = {}): Promise<AlertRecord[]> {
  const params: Record<string, any> = { limit: filter.limit ?? 500 }
  if (filter.from) params.from = filter.from
  if (filter.to) params.to = filter.to
  if (filter.severity) params.severity = filter.severity
  if (filter.host) params.host = filter.host
  const res = await axios.get('/api/alert/history', { params })
  return res.data
}

export async function downloadAlertHistory(filter: AlertHistoryFilter, format: 'csv' | 'json'): Promise<void> {
  const params: Record<string, any> = { format }
  if (filter.from) params.from = filter.from
  if (filter.to) params.to = filter.to
  if (filter.severity) params.severity = filter.severity
  if (filter.host) params.host = filter.host
  const res = await axios.get('/api/alert/history/export', { params, responseType: 'blob' })
  const cd = res.headers['content-disposition'] || ''
  const match = cd.match(/filename="?([^"]+)"?/)
  const filename = match ? match[1] : `owlmon-alerts.${format}`
  const url = URL.createObjectURL(res.data)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

export interface ActiveAlert {
  host: string
  category: string
  severity: string
  value: number
  message: string
  acked: boolean
  in_maintenance: boolean
}

export async function getAlertStatus(): Promise<ActiveAlert[]> {
  const res = await axios.get('/api/alert/status')
  return res.data
}

export async function ackAlert(host: string, category: string, severity: string): Promise<void> {
  await axios.post('/api/alert/ack', { host, category, severity })
}

export async function getMaintenanceHosts(): Promise<string[]> {
  const res = await axios.get<string[]>('/api/maintenance')
  return res.data
}

export async function setMaintenance(host: string, enabled: boolean): Promise<void> {
  await axios.post('/api/maintenance', { host, enabled })
}
