import axios from 'axios'

export interface LogRecord {
  id: number
  timestamp: string
  host: string
  source: string
  file_path: string
  line: string
  level: string
}

export interface LogSearchParams {
  host?: string
  source?: string
  level?: string
  query?: string
  from?: string
  to?: string
  limit?: number
  offset?: number
}

export interface LogSearchResult {
  records: LogRecord[]
  total: number
}

export interface LogSource {
  host: string
  source: string
}

export async function searchLogs(params: LogSearchParams): Promise<LogSearchResult> {
  const res = await axios.get('/api/logs', { params })
  return res.data
}

export async function getLogSources(): Promise<LogSource[]> {
  const res = await axios.get('/api/logs/sources')
  return res.data
}
