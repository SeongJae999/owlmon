import axios from 'axios'

export interface LLMStatus {
  enabled: boolean
  provider?: string
}

export interface ExplainResult {
  explanation: string
  cached: boolean
  masked: boolean
}

export interface SummaryResult {
  summary: string
  total: number
  cached: boolean
}

export async function getLLMStatus(): Promise<LLMStatus> {
  const res = await axios.get('/api/llm/status')
  return res.data
}

export async function explainLog(line: string): Promise<ExplainResult> {
  const res = await axios.post('/api/llm/explain', { line })
  return res.data
}

export async function summarizeAlerts(hours = 24): Promise<SummaryResult> {
  const res = await axios.post('/api/llm/summary', { hours })
  return res.data
}
