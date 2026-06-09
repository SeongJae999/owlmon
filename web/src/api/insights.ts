import axios from 'axios'

export type Severity = 'critical' | 'high' | 'medium' | 'low'
export type Category =
  | 'auth'
  | 'network'
  | 'disk'
  | 'db'
  | 'app'
  | 'security'
  | 'other'

export interface Insight {
  id: number
  template_hash: string
  sample_log_ids: number[]
  host_name?: string
  severity: Severity
  category: Category
  summary_ko: string
  root_cause_ko?: string
  action_ko?: string
  needs_human: boolean
  model_name: string
  prompt_tokens?: number
  output_tokens?: number
  latency_ms?: number
  created_at: string
}

export interface InsightListResult {
  items: Insight[]
  total: number
}

export interface InsightStatus {
  enabled: boolean
}

export interface ListInsightsParams {
  host?: string
  severity?: Severity
  /** RFC3339 e.g. 2026-05-10T00:00:00Z */
  from?: string
  /** RFC3339 */
  to?: string
  /** 1..500 (서버에서 0/초과 시 50으로 클램프) */
  limit?: number
}

/** GET /api/insights/status — 워커 활성 여부 */
export async function getInsightStatus(): Promise<InsightStatus> {
  const res = await axios.get<InsightStatus>('/api/insights/status')
  return res.data
}

/** GET /api/insights/list — 필터 조건으로 인사이트 목록 (최신순) */
export async function listInsights(
  params: ListInsightsParams = {}
): Promise<InsightListResult> {
  const res = await axios.get<InsightListResult>('/api/insights/list', {
    params,
  })
  return res.data
}

/** GET /api/insights/by-template — 단일 템플릿 해시의 캐시된 인사이트 (24h) */
export async function getInsightByTemplate(hash: string): Promise<Insight> {
  const res = await axios.get<Insight>('/api/insights/by-template', {
    params: { hash },
  })
  return res.data
}
