import axios from 'axios'

export interface MatchedRule {
  id: number
  name: string
  severity: 'info' | 'warning' | 'critical'
}

export interface LogRecord {
  id: number
  timestamp: string
  host: string
  source: string
  file_path: string
  line: string
  level: string
  matched_rules?: MatchedRule[]
}

export interface LogSearchParams {
  host?: string
  source?: string
  level?: string
  query?: string
  rule_id?: number
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

// 시간대별 로그 분포 (검색 필터와 동일 파라미터 + 자동 버킷 크기)
export interface LogHistogramBucket {
  ts: string
  count: number
  error_count: number
}
export interface LogHistogramResult {
  buckets: LogHistogramBucket[]
  bucket_sec: number
}
export async function getLogHistogram(params: LogSearchParams): Promise<LogHistogramResult> {
  const res = await axios.get('/api/logs/histogram', { params })
  return res.data
}

// 로그 다운로드 (CSV/JSON) — axios로 JWT 자동 첨부, blob으로 브라우저 저장
export async function downloadLogs(params: LogSearchParams, format: 'csv' | 'json'): Promise<void> {
  const res = await axios.get('/api/logs/export', {
    params: { ...params, format },
    responseType: 'blob',
  })
  const cd = res.headers['content-disposition'] || ''
  const match = cd.match(/filename="?([^"]+)"?/)
  const filename = match ? match[1] : `owlmon-logs.${format}`
  const url = URL.createObjectURL(res.data)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

// 라벨링 (운영자가 부여한 원인/조치 메모) ----------------------------------

export type AnnotationCategory = 'root_cause' | 'action_taken' | 'false_positive'

export interface LogAnnotation {
  id: number
  log_id: number
  log_timestamp: string
  annotator: string
  category?: AnnotationCategory | ''
  problem?: string
  solution?: string
  alert_id?: number | null
  created_at: string
}

export interface AnnotateRequest {
  category?: AnnotationCategory | ''
  problem?: string
  solution?: string
  alert_id?: number | null
}

export interface ListAnnotationsResult {
  items: LogAnnotation[]
  total: number
}

// 단일 로그 상세 (라벨링 모달에서 원본 line 표시용)
export async function getLogById(id: number): Promise<LogRecord> {
  const res = await axios.get(`/api/logs/${id}`)
  return res.data
}

// 특정 로그에 라벨 부여
export async function annotateLog(logId: number, body: AnnotateRequest): Promise<LogAnnotation> {
  const res = await axios.post(`/api/logs/${logId}/annotate`, body)
  return res.data
}

// 특정 로그의 라벨 목록 (모달 내 기존 라벨 표시)
export async function getLogAnnotations(logId: number): Promise<LogAnnotation[]> {
  const res = await axios.get(`/api/logs/${logId}/annotations`)
  return res.data
}

// 전체 라벨 목록 (학습 데이터 추출 — 추후 별도 페이지에서 사용 예정)
export async function listAllAnnotations(params?: { limit?: number; offset?: number }): Promise<ListAnnotationsResult> {
  const res = await axios.get('/api/logs/annotations', { params })
  return res.data
}

// 라벨 삭제 (오타 수정 등)
export async function deleteAnnotation(id: number): Promise<void> {
  await axios.delete(`/api/logs/annotations/${id}`)
}
