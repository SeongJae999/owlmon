import axios from 'axios'

export interface LogRule {
  id: number
  name: string
  pattern: string
  severity: 'info' | 'warning' | 'critical'
  threshold_count: number | null
  threshold_window: number | null
  cooldown_seconds: number
  enabled: boolean
  description: string | null
  category: string | null
}

export type LogRuleInput = Omit<LogRule, 'id'>

export async function listRules(): Promise<LogRule[]> {
  const res = await axios.get('/api/log-rules')
  return res.data ?? []
}

export async function getRuleStats(): Promise<Record<number, number>> {
  const res = await axios.get('/api/log-rules/stats')
  return res.data ?? {}
}

export async function createRule(input: LogRuleInput): Promise<LogRule> {
  const res = await axios.post('/api/log-rules', input)
  return res.data
}

export async function updateRule(id: number, input: LogRuleInput): Promise<void> {
  await axios.put(`/api/log-rules/${id}`, input)
}

export async function toggleRule(id: number): Promise<void> {
  await axios.post(`/api/log-rules/${id}/toggle`)
}

export async function deleteRule(id: number): Promise<void> {
  await axios.delete(`/api/log-rules/${id}`)
}

// ─── 표시용 헬퍼 ─────────────────────────────────────────

export function severityLabel(s: string): string {
  switch (s) {
    case 'critical': return '심각'
    case 'warning':  return '주의'
    case 'info':     return '정보'
    default:         return s
  }
}

export function categoryLabel(c: string | null): string {
  switch (c) {
    case 'security': return '보안'
    case 'system':   return '시스템'
    case 'network':  return '네트워크'
    case 'app':      return '애플리케이션'
    default:         return c ?? '기타'
  }
}
