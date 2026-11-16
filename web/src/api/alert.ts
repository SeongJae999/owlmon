import axios from 'axios'

export interface AlertConfig {
  enabled: boolean
  recipients: string[]
  cpu_threshold: number
  mem_threshold: number
  disk_warn: number
  disk_crit: number
  channels?: Channels // 조회 전용 계정에겐 서버가 비워 보냄
}

export type MinSeverity = '' | 'warning' | 'critical'
export interface Webhook { name: string; url: string; format: 'json' | 'slack'; min_severity: MinSeverity; recovery: boolean }
export interface SMSGateway {
  enabled: boolean; url: string; method: 'POST' | 'GET' | 'PUT'; content_type: string
  headers: Record<string, string>; body: string; phones: string[]; min_severity: MinSeverity; recovery: boolean
}
export interface Escalation { after_min: number; recipients: string[]; phones: string[] }
export interface Channels { webhooks: Webhook[]; sms: SMSGateway; escalation: Escalation }

// 서버(Go)는 빈 목록을 null로 보낼 수 있어 화면에서 쓰기 전에 채움
export function normalizeChannels(c?: Partial<Channels> | null): Channels {
  const sms = c?.sms
  return {
    webhooks: (c?.webhooks ?? []).map(w => ({ ...w, format: w.format || 'json', min_severity: w.min_severity ?? '' })),
    sms: {
      enabled: !!sms?.enabled, url: sms?.url ?? '', method: sms?.method || 'POST', content_type: sms?.content_type || 'application/json',
      headers: sms?.headers ?? {}, body: sms?.body ?? '{"to":"{{phone}}","text":"{{message}}"}', phones: sms?.phones ?? [],
      min_severity: sms?.min_severity ?? '', recovery: !!sms?.recovery,
    },
    escalation: { after_min: c?.escalation?.after_min ?? 0, recipients: c?.escalation?.recipients ?? [], phones: c?.escalation?.phones ?? [] },
  }
}

export async function testChannel(req: { channel: 'webhook'; webhook: Webhook } | { channel: 'sms'; sms: SMSGateway; phone: string }): Promise<void> {
  await axios.post('/api/alert/test', req)
}

// 화면 경고/위험 기준 — 홈·상세가 같은 값을 쓰도록 단일 출처 (서버 알림 기준 = 위험, 경고는 그보다 낮게)
export function thresholds(cfg?: AlertConfig) {
  const cpu = cfg?.cpu_threshold ?? 90
  const mem = cfg?.mem_threshold ?? 95
  return {
    cpuWarn: cpu * 0.8, cpuCrit: cpu,
    memWarn: mem * 0.85, memCrit: mem,
    diskWarn: cfg?.disk_warn ?? 85, diskCrit: cfg?.disk_crit ?? 90,
  }
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
  acked_by?: string
  acked_at?: string
  since?: string
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

export interface AckEntry {
  host: string
  category: string
  severity: string
  acked_by: string
  acked_at: string
}

export async function getAlertAcks(): Promise<AckEntry[]> {
  const res = await axios.get('/api/alert/acks')
  return res.data
}

// 등급은 옆 배지가 보여 줌 — 제목 앞의 등급 표기와 예전 버전이 붙이던 이모지(🔴·🚨 등)를 떼어 같은 말이 두 번 나오지 않게
export const alertTitle = (subject: string) => subject
  .replace(/^\[OWLmon\]\s*/, '')
  .replace(/^(?:(?:\[(?:위험|주의|심각|경고|정보)\]|\p{Extended_Pictographic}\uFE0F?)\s*)+/u, '')
