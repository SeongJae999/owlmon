import axios from 'axios'

export interface AuditEntry {
  id: number
  ts: string
  actor: string
  ip: string
  action: string
  target_type: string
  target_id: string
  details?: any
  result: 'success' | 'failure' | 'unauthorized'
  user_agent?: string
}

export interface AuditFilter {
  actor?: string
  action?: string
  target_type?: string
  from?: string
  to?: string
  limit?: number
}

export async function searchAudit(filter: AuditFilter = {}): Promise<AuditEntry[]> {
  const params: Record<string, any> = {}
  if (filter.actor) params.actor = filter.actor
  if (filter.action) params.action = filter.action
  if (filter.target_type) params.target_type = filter.target_type
  if (filter.from) params.from = filter.from
  if (filter.to) params.to = filter.to
  if (filter.limit) params.limit = filter.limit
  const res = await axios.get('/api/audit', { params })
  return res.data
}

export async function downloadAudit(filter: AuditFilter, format: 'csv' | 'json'): Promise<void> {
  const params: Record<string, any> = { format }
  if (filter.actor) params.actor = filter.actor
  if (filter.action) params.action = filter.action
  if (filter.target_type) params.target_type = filter.target_type
  if (filter.from) params.from = filter.from
  if (filter.to) params.to = filter.to
  const res = await axios.get('/api/audit/export', { params, responseType: 'blob' })
  const cd = res.headers['content-disposition'] || ''
  const match = cd.match(/filename="?([^"]+)"?/)
  const filename = match ? match[1] : `owlmon-audit.${format}`
  const url = URL.createObjectURL(res.data)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}
