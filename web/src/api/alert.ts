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

export interface AlertRecord {
  id: number
  sent_at: string
  host: string
  category: string
  severity: string
  subject: string
  body: string
}

export async function getAlertHistory(limit = 100): Promise<AlertRecord[]> {
  const res = await axios.get('/api/alert/history', { params: { limit } })
  return res.data
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
