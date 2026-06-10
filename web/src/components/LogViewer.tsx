import { useState, useMemo } from 'react'
import {
  searchLogs, getLogSources, getLogHistogram, downloadLogs,
  type LogSearchParams, type LogRecord, type MatchedRule,
} from '../api/logs'
import { listRules, severityLabel } from '../api/rules'
import { useQuery } from '@tanstack/react-query'
import {
  Search, RefreshCcw, Server, ChevronLeft, ChevronRight, Info, AlertCircle,
  Tag, ListChecks, ChevronDown, ChevronRight as ChevronRightSmall, Layers,
  Clock, Download, Sparkles, X,
} from 'lucide-react'
import { getLLMStatus, explainLog, type ExplainResult } from '../api/llm'
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Cell } from 'recharts'
import { cn } from '../utils/cn'
import { TONES } from '../utils/severity'
import AnnotateModal from './AnnotateModal'

// 시간 범위 프리셋
const RANGE_PRESETS = [
  { label: '15분', minutes: 15 },
  { label: '1시간', minutes: 60 },
  { label: '24시간', minutes: 24 * 60 },
  { label: '7일', minutes: 7 * 24 * 60 },
  { label: '30일', minutes: 30 * 24 * 60 },
] as const

const LEVEL_CONFIG: Record<string, { bg: string, text: string, ring: string }> = {
  ERROR:   { bg: TONES.rose.bg,  text: TONES.rose.text,  ring: TONES.rose.ring },
  FATAL:   { bg: TONES.rose.bg,  text: TONES.rose.text,  ring: TONES.rose.ring },
  WARN:    { bg: TONES.amber.bg, text: TONES.amber.text, ring: TONES.amber.ring },
  WARNING: { bg: TONES.amber.bg, text: TONES.amber.text, ring: TONES.amber.ring },
  INFO:    { bg: TONES.blue.bg,  text: TONES.blue.text,  ring: TONES.blue.ring },
  DEBUG:   { bg: 'bg-slate-700', text: TONES.slate.text, ring: TONES.slate.ring },
}

interface LogGroup {
  key: string
  host: string
  source: string
  level: string
  line: string
  count: number
  firstTs: number
  lastTs: number
  records: LogRecord[]
  matched_rules?: MatchedRule[]
}

function groupRecords(records: LogRecord[]): LogGroup[] {
  const map = new Map<string, LogGroup>()
  for (const r of records) {
    const key = `${r.host}|${r.source}|${(r.level || '').toUpperCase()}|${r.line}`
    const ts = new Date(r.timestamp).getTime()
    const existing = map.get(key)
    if (existing) {
      existing.count++
      existing.firstTs = Math.min(existing.firstTs, ts)
      existing.lastTs = Math.max(existing.lastTs, ts)
      existing.records.push(r)
    } else {
      map.set(key, {
        key,
        host: r.host,
        source: r.source,
        level: (r.level || '').toUpperCase(),
        line: r.line,
        count: 1,
        firstTs: ts,
        lastTs: ts,
        records: [r],
        matched_rules: r.matched_rules,
      })
    }
  }
  return [...map.values()].sort((a, b) => b.lastTs - a.lastTs)
}

function formatTime(ts: number): string {
  return new Date(ts).toLocaleTimeString('ko-KR', {
    hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
  })
}

function formatTimeRange(first: number, last: number): string {
  if (first === last) return formatTime(last)
  return `${formatTime(first)} ~ ${formatTime(last)}`
}

export default function LogViewer() {
  const [autoRefresh, setAutoRefresh] = useState(false)
  const [refreshSec, setRefreshSec] = useState(30)
  const [host, setHost] = useState('')
  const [source, setSource] = useState('')
  const [level, setLevel] = useState('')
  const [ruleID, setRuleID] = useState<number | ''>('')
  const [queryText, setQueryText] = useState('')
  const [page, setPage] = useState(0)
  const [groupMode, setGroupMode] = useState(true)
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [selectedLog, setSelectedLog] = useState<LogRecord | null>(null)
  const [explainLine, setExplainLine] = useState<string | null>(null)
  const [explainResult, setExplainResult] = useState<ExplainResult | null>(null)
  const [explainLoading, setExplainLoading] = useState(false)
  const [explainError, setExplainError] = useState<string | null>(null)

  // LLM 활성 여부 — 비활성이면 AI 버튼 숨김
  const { data: llmStatus } = useQuery({
    queryKey: ['llmStatus'],
    queryFn: getLLMStatus,
    staleTime: 5 * 60_000,
  })

  const handleExplain = async (line: string) => {
    setExplainLine(line)
    setExplainResult(null)
    setExplainError(null)
    setExplainLoading(true)
    try {
      const result = await explainLog(line)
      setExplainResult(result)
    } catch (e: any) {
      setExplainError(e?.response?.data?.toString?.() || e?.message || 'AI 설명 실패')
    } finally {
      setExplainLoading(false)
    }
  }
  const [rangeMinutes, setRangeMinutes] = useState<number>(24 * 60) // 디폴트 24시간
  const [downloading, setDownloading] = useState(false)
  const limit = 50

  // 시간 범위 → from/to ISO 문자열은 매 쿼리 시점에 새로 계산 (자동새로고침에서도 최신 보장)
  const buildTimeWindow = () => {
    const now = new Date()
    const from = new Date(now.getTime() - rangeMinutes * 60_000)
    return { fromISO: from.toISOString(), toISO: now.toISOString() }
  }

  const { data: sources = [] } = useQuery({
    queryKey: ['logSources'],
    queryFn: getLogSources,
  })

  const { data: rules = [] } = useQuery({
    queryKey: ['log-rules'],
    queryFn: listRules,
    staleTime: 60_000,
  })

  const buildParams = (): LogSearchParams => {
    const { fromISO, toISO } = buildTimeWindow()
    const p: LogSearchParams = { from: fromISO, to: toISO }
    if (host) p.host = host
    if (source) p.source = source
    if (level) p.level = level
    if (queryText) p.query = queryText
    if (ruleID) p.rule_id = ruleID
    return p
  }

  // queryKey에는 rangeMinutes만 — 자동 새로고침은 refetchInterval로 갱신
  const { data: logData, isLoading, refetch } = useQuery({
    queryKey: ['logs', host, source, level, ruleID, queryText, page, rangeMinutes],
    queryFn: () => searchLogs({ ...buildParams(), limit, offset: page * limit }),
    refetchInterval: autoRefresh ? refreshSec * 1000 : false,
  })

  const { data: histogram } = useQuery({
    queryKey: ['logsHistogram', host, source, level, ruleID, queryText, rangeMinutes],
    queryFn: () => getLogHistogram(buildParams()),
    refetchInterval: autoRefresh ? refreshSec * 1000 : false,
    staleTime: 30_000,
  })

  const handleDownload = async (format: 'csv' | 'json') => {
    setDownloading(true)
    try {
      await downloadLogs(buildParams(), format)
    } catch (e) {
      console.error(e)
      alert('다운로드 실패')
    } finally {
      setDownloading(false)
    }
  }

  const records = logData?.records || []
  const total = logData?.total || 0
  const uniqueHosts = [...new Set(sources.map(s => s.host))]
  const filteredSources = host ? sources.filter(s => s.host === host) : sources
  const uniqueSources = [...new Set(filteredSources.map(s => s.source))]
  const totalPages = Math.ceil(total / limit)

  const groups = useMemo(() => groupRecords(records), [records])
  const duplicatesCollapsed = records.length - groups.length

  const handleSearch = () => {
    setPage(0)
    refetch()
  }

  const toggleExpand = (key: string) => {
    setExpanded(prev => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  // 활성 필터 칩 표시용
  const activeFilters: { label: string; onClear: () => void }[] = []
  if (host)   activeFilters.push({ label: `HOST: ${host}`,   onClear: () => { setHost(''); setPage(0) } })
  if (source) activeFilters.push({ label: `SOURCE: ${source}`, onClear: () => { setSource(''); setPage(0) } })
  if (level)  activeFilters.push({ label: `LEVEL: ${level}`,  onClear: () => { setLevel(''); setPage(0) } })
  if (ruleID) {
    const r = rules.find(x => x.id === ruleID)
    activeFilters.push({ label: `RULE: ${r?.name ?? ruleID}`, onClear: () => { setRuleID(''); setPage(0) } })
  }
  if (queryText) activeFilters.push({ label: `검색: ${queryText}`, onClear: () => { setQueryText(''); setPage(0); setTimeout(() => refetch(), 0) } })

  const selectCls = "bg-slate-800 border border-slate-700 hover:border-slate-600 rounded-lg pl-3 pr-7 py-2 text-sm text-slate-200 focus:outline-none focus:ring-2 focus:ring-indigo-500/30 focus:border-indigo-500 transition-all cursor-pointer appearance-none bg-no-repeat bg-[length:14px_14px] bg-[right_0.5rem_center]"
  const selectStyle = {
    backgroundImage: "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='14' height='14' viewBox='0 0 24 24' fill='none' stroke='%2394a3b8' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpolyline points='6 9 12 15 18 9'%3E%3C/polyline%3E%3C/svg%3E\")",
  }

  return (
    <div className="space-y-4">
      {/* ─── Search & Filter Bar ─────────────────────────── */}
      <div className="bg-slate-900 p-3 rounded-2xl border border-slate-800 shadow-sm space-y-3">
        {/* Row 1: 검색 input + 4개 필터 (lg에서 한 줄, 미만에서 줄 바꿈) */}
        <div className="flex flex-col lg:flex-row gap-2 lg:items-center">
          {/* 검색 input (가장 강조) */}
          <div className="relative flex-1 min-w-0 lg:min-w-[260px]">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-400 pointer-events-none" size={16} />
            <input
              className="w-full bg-slate-800 border border-slate-700 rounded-lg pl-9 pr-9 py-2 text-sm text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-indigo-500/30 focus:border-indigo-500 transition-all"
              placeholder="로그 본문 검색..."
              value={queryText}
              onChange={e => setQueryText(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter') handleSearch() }}
            />
            {queryText && (
              <button
                onClick={() => { setQueryText(''); setPage(0); setTimeout(() => refetch(), 0) }}
                className="absolute right-2 top-1/2 -translate-y-1/2 text-slate-500 hover:text-slate-200 text-xs px-1"
                title="검색어 지우기"
              >
                ✕
              </button>
            )}
          </div>

          {/* 필터 4개 */}
          <div className="grid grid-cols-2 lg:flex lg:flex-row gap-2">
            <select
              className={selectCls}
              style={selectStyle}
              value={host}
              onChange={e => { setHost(e.target.value); setSource(''); setPage(0) }}
              title="호스트 필터"
            >
              <option value="">전체 호스트</option>
              {uniqueHosts.map(h => <option key={h} value={h}>{h}</option>)}
            </select>
            <select
              className={selectCls}
              style={selectStyle}
              value={source}
              onChange={e => { setSource(e.target.value); setPage(0) }}
              title="소스 필터"
            >
              <option value="">전체 소스</option>
              {uniqueSources.map(s => <option key={s} value={s}>{s}</option>)}
            </select>
            <select
              className={selectCls}
              style={selectStyle}
              value={level}
              onChange={e => { setLevel(e.target.value); setPage(0) }}
              title="레벨 필터"
            >
              <option value="">전체 레벨</option>
              <option value="ERROR">ERROR</option>
              <option value="WARN">WARN</option>
              <option value="INFO">INFO</option>
              <option value="DEBUG">DEBUG</option>
            </select>
            <select
              className={selectCls}
              style={selectStyle}
              value={ruleID}
              onChange={e => { setRuleID(e.target.value ? Number(e.target.value) : ''); setPage(0) }}
              title="룰 필터"
            >
              <option value="">전체 룰</option>
              {rules.map(r => <option key={r.id} value={r.id}>{r.name}</option>)}
            </select>
          </div>

          <button
            className="bg-indigo-600 hover:bg-indigo-700 text-white px-4 py-2 rounded-lg text-sm font-semibold transition-colors shadow-sm shadow-indigo-500/20 shrink-0"
            onClick={handleSearch}
          >
            검색
          </button>
        </div>

        {/* 시간 범위 + 다운로드 */}
        <div className="flex flex-wrap items-center gap-2 pt-2 border-t border-slate-800">
          <span className="text-[11px] font-semibold text-slate-500 uppercase tracking-wider flex items-center gap-1">
            <Clock size={11} /> 시간 범위
          </span>
          <div className="flex gap-1 bg-slate-800 rounded-lg p-0.5">
            {RANGE_PRESETS.map(r => (
              <button
                key={r.label}
                onClick={() => { setRangeMinutes(r.minutes); setPage(0) }}
                className={cn(
                  "px-2.5 py-1 rounded text-[11px] font-bold transition-colors",
                  rangeMinutes === r.minutes ? "bg-indigo-600 text-white" : "text-slate-400 hover:text-slate-200"
                )}
              >
                {r.label}
              </button>
            ))}
          </div>

          <div className="ml-auto flex gap-1.5">
            <button
              onClick={() => handleDownload('csv')}
              disabled={downloading || total === 0}
              className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-[11px] font-bold bg-slate-800 text-slate-300 border border-slate-700 hover:bg-slate-700 transition-colors disabled:opacity-40"
              title="CSV 다운로드 (최대 1만건)"
            >
              {downloading ? <RefreshCcw size={11} className="animate-spin" /> : <Download size={11} />}
              CSV
            </button>
            <button
              onClick={() => handleDownload('json')}
              disabled={downloading || total === 0}
              className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-[11px] font-bold bg-slate-800 text-slate-300 border border-slate-700 hover:bg-slate-700 transition-colors disabled:opacity-40"
              title="JSON 다운로드 (최대 1만건)"
            >
              {downloading ? <RefreshCcw size={11} className="animate-spin" /> : <Download size={11} />}
              JSON
            </button>
          </div>
        </div>

        {/* Row 2: 활성 필터 칩 (있을 때만) */}
        {activeFilters.length > 0 && (
          <div className="flex flex-wrap gap-1.5 items-center">
            <span className="text-[11px] font-semibold text-slate-500 uppercase tracking-wider">활성 필터</span>
            {activeFilters.map((f, i) => (
              <button
                key={i}
                onClick={f.onClear}
                className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md bg-indigo-500/10 border border-indigo-500/30 text-[11px] font-semibold text-indigo-300 hover:bg-indigo-500/20 hover:text-indigo-200 transition-colors"
                title="필터 제거"
              >
                {f.label}
                <span className="text-slate-500">✕</span>
              </button>
            ))}
          </div>
        )}

        {/* Row 3: 결과 카운트 + 그룹뷰 토글 + 자동 새로고침 */}
        <div className="pt-2 border-t border-slate-800 flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-baseline gap-2">
            <span className="text-[11px] font-semibold text-slate-500 uppercase tracking-wider">결과</span>
            <span className="text-2xl font-bold text-slate-100 tabular-nums leading-none">{total.toLocaleString()}</span>
            <span className="text-xs text-slate-500">건</span>
            {groupMode && duplicatesCollapsed > 0 && (
              <span className="ml-2 inline-flex items-center gap-1 px-1.5 py-0.5 rounded-md bg-amber-500/10 border border-amber-500/30 text-[11px] font-bold text-amber-300">
                <Layers size={10} />
                페이지 내 중복 {duplicatesCollapsed}건 압축됨
              </span>
            )}
          </div>

          <div className="flex items-center gap-3">
            {/* 그룹뷰 토글 */}
            <label className="flex items-center gap-1.5 cursor-pointer group">
              <input
                type="checkbox"
                checked={groupMode}
                onChange={e => setGroupMode(e.target.checked)}
                className="sr-only peer"
              />
              <div className="relative w-9 h-5 bg-slate-700 rounded-full peer peer-checked:bg-indigo-600 after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-slate-900 after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:after:translate-x-full" />
              <span className="text-[11px] font-semibold text-slate-400 group-hover:text-slate-200 transition-colors flex items-center gap-1">
                <Layers size={11} /> 중복 묶기
              </span>
            </label>

            {/* 자동 새로고침 */}
            <label className="flex items-center gap-1.5 cursor-pointer group">
              <input
                type="checkbox"
                checked={autoRefresh}
                onChange={e => setAutoRefresh(e.target.checked)}
                className="sr-only peer"
              />
              <div className="relative w-9 h-5 bg-slate-700 rounded-full peer peer-checked:bg-emerald-600 after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-slate-900 after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:after:translate-x-full" />
              <span className="text-[11px] font-semibold text-slate-400 group-hover:text-slate-200 transition-colors">자동 새로고침</span>
            </label>
            <select
              value={refreshSec}
              onChange={e => setRefreshSec(Number(e.target.value))}
              disabled={!autoRefresh}
              className="text-[11px] bg-slate-800 border border-slate-700 rounded px-2 py-1 text-slate-300 disabled:opacity-40"
            >
              <option value={10}>10초</option>
              <option value={30}>30초</option>
              <option value={60}>1분</option>
              <option value={300}>5분</option>
            </select>
            {autoRefresh && <RefreshCcw size={12} className="text-emerald-400 animate-spin" />}
          </div>
        </div>
      </div>

      {/* ─── Histogram ─────────────────────────────── */}
      {histogram && histogram.buckets.length > 0 && (
        <div className="bg-slate-900 rounded-2xl border border-slate-800 shadow-sm p-3">
          <div className="flex items-center justify-between mb-2">
            <span className="text-[11px] font-bold text-slate-400 uppercase tracking-wider flex items-center gap-1.5">
              <Layers size={11} /> 시간대별 분포
            </span>
            <span className="text-[10px] text-slate-500 font-mono">
              {histogram.bucket_sec >= 86400 ? `${histogram.bucket_sec / 86400}일`
                : histogram.bucket_sec >= 3600 ? `${histogram.bucket_sec / 3600}시간`
                : `${histogram.bucket_sec / 60}분`} 단위
            </span>
          </div>
          <div className="h-24">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={histogram.buckets} margin={{ top: 2, right: 4, left: -28, bottom: 0 }}>
                <XAxis
                  dataKey="ts"
                  stroke="#64748b"
                  fontSize={9}
                  tickFormatter={(t) => {
                    const d = new Date(t)
                    if (histogram.bucket_sec >= 86400) return d.toLocaleDateString('ko-KR', { month: 'numeric', day: 'numeric' })
                    if (histogram.bucket_sec >= 3600) return d.toLocaleString('ko-KR', { month: 'numeric', day: 'numeric', hour: '2-digit', hour12: false })
                    return d.toLocaleTimeString('ko-KR', { hour: '2-digit', minute: '2-digit', hour12: false })
                  }}
                  interval="preserveStartEnd"
                  minTickGap={40}
                />
                <YAxis stroke="#64748b" fontSize={9} allowDecimals={false} />
                <Tooltip
                  contentStyle={{ background: '#0f172a', border: '1px solid #334155', borderRadius: '8px', fontSize: '11px' }}
                  labelFormatter={(t) => new Date(t).toLocaleString('ko-KR')}
                  formatter={(value: number, name: string) => [value, name === 'count' ? '전체' : 'ERROR/WARN']}
                />
                <Bar dataKey="count" fill="#6366f1">
                  {histogram.buckets.map((b, i) => (
                    <Cell key={i} fill={b.error_count > 0 ? '#f43f5e' : '#6366f1'} />
                  ))}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          </div>
        </div>
      )}

      {/* ─── Log Content ─────────────────────────────────── */}
      <div className="bg-slate-900 rounded-2xl border border-slate-800 shadow-sm overflow-hidden">
        {isLoading && records.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-32 text-slate-400 animate-pulse">
            <RefreshCcw size={48} className="mb-4 opacity-20 animate-spin" />
            <p className="font-medium">로그 데이터를 불러오는 중...</p>
          </div>
        ) : records.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-32 text-slate-400 text-center px-6">
            {sources.length === 0 ? (
              <>
                <Info size={48} className="mb-4 opacity-20" />
                <p className="font-medium max-w-xs leading-relaxed text-sm">
                  수집된 로그가 없습니다.<br/>
                  에이전트 <code className="bg-slate-800 px-1 rounded text-slate-400 font-bold">config.yaml</code>에 로그 수집 설정을 추가하세요.
                </p>
              </>
            ) : (
              <>
                <AlertCircle size={48} className="mb-4 opacity-20" />
                <p className="font-medium">검색 조건에 맞는 로그가 없습니다.</p>
              </>
            )}
          </div>
        ) : groupMode ? (
          /* ─── Grouped view ─────────────────────────────── */
          <div className="divide-y divide-slate-800">
            {groups.map(g => {
              const lvlCfg = LEVEL_CONFIG[g.level] ?? { bg: 'bg-slate-700', text: 'text-slate-400', ring: 'ring-slate-600/30' }
              const isExpanded = expanded.has(g.key)
              const hasMultiple = g.count > 1

              return (
                <div key={g.key} className="group/row hover:bg-slate-800/40 transition-colors">
                  {/* 그룹 헤더 행 */}
                  <div className="flex items-start gap-2 px-3 py-2">
                    {/* 펼치기 버튼 (count > 1만) */}
                    <button
                      onClick={() => hasMultiple && toggleExpand(g.key)}
                      disabled={!hasMultiple}
                      className={cn(
                        "shrink-0 mt-0.5 p-0.5 rounded text-slate-500 hover:text-slate-200 hover:bg-slate-700 transition-colors",
                        !hasMultiple && "invisible"
                      )}
                      title={isExpanded ? "접기" : `원본 ${g.count}건 펼치기`}
                    >
                      {isExpanded ? <ChevronDown size={14} /> : <ChevronRightSmall size={14} />}
                    </button>

                    {/* 시간 */}
                    <span
                      className="shrink-0 text-[11px] text-slate-300 font-mono whitespace-nowrap mt-1 w-[62px] tabular-nums"
                      title={hasMultiple
                        ? `${formatTimeRange(g.firstTs, g.lastTs)} (${g.count}회)`
                        : new Date(g.lastTs).toLocaleString('ko-KR')}
                    >
                      {formatTime(g.lastTs)}
                    </span>

                    {/* 레벨 뱃지 */}
                    <span className={cn(
                      "shrink-0 px-1.5 py-0.5 rounded text-[11px] font-bold uppercase tracking-tight mt-0.5 w-[52px] text-center",
                      lvlCfg.bg, lvlCfg.text
                    )}>
                      {g.level || '—'}
                    </span>

                    {/* count 뱃지 */}
                    {hasMultiple && (
                      <span className="shrink-0 mt-0.5 px-1.5 py-0.5 rounded text-[11px] font-bold bg-rose-500/15 text-rose-300 ring-1 ring-rose-500/30 tabular-nums">
                        ×{g.count}
                      </span>
                    )}

                    {/* host · source */}
                    <span className="shrink-0 hidden md:inline-flex items-center gap-1 mt-1 text-xs text-slate-300 max-w-[180px] truncate">
                      <Server size={10} className="opacity-50 shrink-0" />
                      <span className="font-semibold truncate">{g.host}</span>
                      <span className="opacity-40">·</span>
                      <span className="truncate text-slate-500">{g.source}</span>
                    </span>

                    {/* 메시지 (한 줄, truncate) */}
                    <code
                      className="flex-1 min-w-0 text-[12.5px] leading-snug text-slate-100 font-mono truncate mt-0.5"
                      title={g.line}
                    >
                      {g.line}
                    </code>

                    {/* 액션 버튼 그룹 — hover 시 표시 */}
                    <div className="shrink-0 opacity-0 group-hover/row:opacity-100 transition-opacity flex items-center gap-1 mt-0.5">
                      {llmStatus?.enabled && (
                        <button
                          onClick={() => handleExplain(g.line)}
                          className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[11px] font-bold text-violet-300 bg-violet-500/10 hover:bg-violet-500/20 border border-violet-500/30"
                          title="AI가 이 로그를 한국어로 설명"
                        >
                          <Sparkles size={10} />
                          AI 설명
                        </button>
                      )}
                      <button
                        onClick={() => setSelectedLog(g.records[g.records.length - 1])}
                        className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[11px] font-bold text-indigo-300 bg-indigo-500/10 hover:bg-indigo-500/20 border border-indigo-500/30"
                        title="이 로그에 원인/조치 메모 남기기"
                      >
                        <Tag size={10} />
                        메모
                      </button>
                    </div>
                  </div>

                  {/* 모바일 메타 (md 미만) */}
                  <div className="md:hidden px-3 pb-1.5 -mt-1 ml-[88px] text-[11px] text-slate-500 flex items-center gap-1.5">
                    <Server size={10} className="opacity-50 shrink-0" />
                    <span className="font-semibold text-slate-400 truncate">{g.host}</span>
                    <span className="opacity-40">·</span>
                    <span className="truncate">{g.source}</span>
                  </div>

                  {/* 룰 뱃지 */}
                  {g.matched_rules && g.matched_rules.length > 0 && (
                    <div className="px-3 pb-2 -mt-1 ml-[88px] flex flex-wrap gap-1">
                      {g.matched_rules.map(mr => {
                        const c = mr.severity === 'critical'
                          ? 'bg-rose-500/15 text-rose-300 border-rose-500/30'
                          : mr.severity === 'warning'
                            ? 'bg-amber-500/15 text-amber-300 border-amber-500/30'
                            : 'bg-slate-500/15 text-slate-300 border-slate-500/30'
                        return (
                          <button
                            key={mr.id}
                            onClick={() => { setRuleID(mr.id); setPage(0) }}
                            className={cn(
                              "inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[11px] font-semibold border hover:opacity-80",
                              c,
                            )}
                            title={`'${mr.name}' 룰만 필터`}
                          >
                            <ListChecks size={10} />
                            {mr.name}
                            <span className="opacity-60">·{severityLabel(mr.severity)}</span>
                          </button>
                        )
                      })}
                    </div>
                  )}

                  {/* 펼친 원본 */}
                  {isExpanded && hasMultiple && (
                    <div className="ml-[88px] mr-3 mb-2 border-l-2 border-slate-700 pl-3 space-y-1">
                      {g.records.slice(0, 20).map(r => (
                        <div key={r.id} className="flex items-baseline gap-2 text-[11.5px]">
                          <span className="text-slate-400 font-mono tabular-nums shrink-0 w-[62px]">
                            {new Date(r.timestamp).toLocaleTimeString('ko-KR', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })}
                          </span>
                          <button
                            onClick={() => setSelectedLog(r)}
                            className="text-indigo-400 hover:text-indigo-300 underline-offset-2 hover:underline shrink-0"
                            title="이 인스턴스에 메모 남기기"
                          >
                            #{r.id}
                          </button>
                          <code className="text-slate-200 font-mono truncate" title={r.line}>{r.line}</code>
                        </div>
                      ))}
                      {g.records.length > 20 && (
                        <div className="text-[11px] text-slate-500 italic pt-1">
                          ... 그리고 {g.records.length - 20}건 더 (페이지 내). 더 보려면 중복 묶기를 끄거나 검색을 좁히세요.
                        </div>
                      )}
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        ) : (
          /* ─── Flat view ────────────────────────────────── */
          <div className="divide-y divide-slate-800">
            {records.map(r => {
              const lvlCfg = LEVEL_CONFIG[(r.level || '').toUpperCase()] ?? { bg: 'bg-slate-700', text: 'text-slate-400', ring: 'ring-slate-600/30' }
              const ts = new Date(r.timestamp)
              return (
                <div key={r.id} className="group/row hover:bg-slate-800/40 transition-colors">
                  <div className="flex items-start gap-2 px-3 py-1.5">
                    <span
                      className="shrink-0 text-[11px] text-slate-300 font-mono whitespace-nowrap mt-1 w-[62px] tabular-nums"
                      title={ts.toLocaleString('ko-KR')}
                    >
                      {ts.toLocaleTimeString('ko-KR', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })}
                    </span>
                    <span className={cn(
                      "shrink-0 px-1.5 py-0.5 rounded text-[11px] font-bold uppercase tracking-tight mt-0.5 w-[52px] text-center",
                      lvlCfg.bg, lvlCfg.text,
                    )}>
                      {r.level || '—'}
                    </span>
                    <span className="shrink-0 hidden md:inline-flex items-center gap-1 mt-1 text-xs text-slate-300 max-w-[180px] truncate">
                      <Server size={10} className="opacity-50 shrink-0" />
                      <span className="font-semibold truncate">{r.host}</span>
                      <span className="opacity-40">·</span>
                      <span className="truncate text-slate-500">{r.source}</span>
                    </span>
                    <code
                      className="flex-1 min-w-0 text-[12.5px] leading-snug text-slate-100 font-mono truncate mt-0.5"
                      title={r.line}
                    >
                      {r.line}
                    </code>
                    <div className="shrink-0 opacity-0 group-hover/row:opacity-100 transition-opacity flex items-center gap-1 mt-0.5">
                      {llmStatus?.enabled && (
                        <button
                          onClick={() => handleExplain(r.line)}
                          className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[11px] font-bold text-violet-300 bg-violet-500/10 hover:bg-violet-500/20 border border-violet-500/30"
                          title="AI가 이 로그를 한국어로 설명"
                        >
                          <Sparkles size={10} />
                          AI 설명
                        </button>
                      )}
                      <button
                        onClick={() => setSelectedLog(r)}
                        className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[11px] font-bold text-indigo-300 bg-indigo-500/10 hover:bg-indigo-500/20 border border-indigo-500/30"
                        title="이 로그에 원인/조치 메모 남기기"
                      >
                        <Tag size={10} />
                        메모
                      </button>
                    </div>
                  </div>
                  <div className="md:hidden px-3 pb-1.5 -mt-1 ml-[68px] text-[11px] text-slate-500 flex items-center gap-1.5">
                    <Server size={10} className="opacity-50 shrink-0" />
                    <span className="font-semibold text-slate-400 truncate">{r.host}</span>
                    <span className="opacity-40">·</span>
                    <span className="truncate">{r.source}</span>
                  </div>
                  {r.matched_rules && r.matched_rules.length > 0 && (
                    <div className="px-3 pb-1.5 -mt-1 ml-[68px] flex flex-wrap gap-1">
                      {r.matched_rules.map(mr => {
                        const c = mr.severity === 'critical'
                          ? 'bg-rose-500/15 text-rose-300 border-rose-500/30'
                          : mr.severity === 'warning'
                            ? 'bg-amber-500/15 text-amber-300 border-amber-500/30'
                            : 'bg-slate-500/15 text-slate-300 border-slate-500/30'
                        return (
                          <button
                            key={mr.id}
                            onClick={() => { setRuleID(mr.id); setPage(0) }}
                            className={cn(
                              "inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[11px] font-semibold border hover:opacity-80",
                              c,
                            )}
                            title={`'${mr.name}' 룰만 필터`}
                          >
                            <ListChecks size={10} />
                            {mr.name}
                            <span className="opacity-60">·{severityLabel(mr.severity)}</span>
                          </button>
                        )
                      })}
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        )}

        {/* Annotate Modal */}
        {selectedLog && (
          <AnnotateModal
            log={selectedLog}
            onClose={() => setSelectedLog(null)}
          />
        )}

        {/* AI 설명 Modal */}
        {explainLine !== null && (
          <div
            className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4"
            onClick={() => setExplainLine(null)}
          >
            <div
              className="bg-slate-900 border border-violet-500/30 rounded-2xl shadow-2xl w-full max-w-2xl max-h-[85vh] overflow-y-auto"
              onClick={e => e.stopPropagation()}
            >
              <div className="flex items-center justify-between p-5 pb-3 border-b border-slate-800">
                <div className="flex items-center gap-3">
                  <div className="p-2 rounded-lg bg-violet-500/15 text-violet-300">
                    <Sparkles size={18} />
                  </div>
                  <div>
                    <h3 className="text-base font-bold text-slate-100">AI 로그 설명</h3>
                    <p className="text-[11px] text-slate-500">{llmStatus?.provider ?? 'LLM'}{explainResult?.cached && ' · 캐시'}{explainResult?.masked && ' · PII 마스킹됨'}</p>
                  </div>
                </div>
                <button onClick={() => setExplainLine(null)} className="p-1 rounded-md text-slate-500 hover:text-slate-300 hover:bg-slate-800">
                  <X size={16} />
                </button>
              </div>

              <div className="px-5 py-3 border-b border-slate-800 bg-slate-950/50">
                <div className="text-[10px] font-bold text-slate-500 uppercase tracking-wide mb-1">대상 로그</div>
                <code className="text-xs text-slate-300 font-mono break-words whitespace-pre-wrap">{explainLine}</code>
              </div>

              <div className="p-5">
                {explainLoading && (
                  <div className="flex items-center gap-2 text-sm text-slate-400">
                    <RefreshCcw size={14} className="animate-spin" />
                    AI가 로그를 분석 중...
                  </div>
                )}
                {explainError && (
                  <div className="flex items-start gap-2 px-3 py-2 rounded-lg bg-rose-500/10 border border-rose-500/30 text-sm text-rose-300">
                    <AlertCircle size={14} className="shrink-0 mt-0.5" />
                    <span>{explainError}</span>
                  </div>
                )}
                {explainResult && (
                  <pre className="text-sm text-slate-200 whitespace-pre-wrap break-words font-sans leading-relaxed">
                    {explainResult.explanation}
                  </pre>
                )}
              </div>

              <div className="p-3 bg-slate-800/30 border-t border-slate-800 text-[10px] text-slate-500">
                AI 답변은 참고용입니다. 구체 조치 전 검증 권장.
              </div>
            </div>
          </div>
        )}

        {/* Pagination */}
        {totalPages > 1 && (
          <div className="flex items-center justify-between px-4 py-3 bg-slate-800/30 border-t border-slate-800">
            <div className="text-[11px] font-bold text-slate-400 uppercase tracking-wider">
              Page <span className="text-slate-100">{page + 1}</span> / <span className="text-slate-100">{totalPages.toLocaleString()}</span>
            </div>
            <div className="flex gap-2">
              <button
                className="p-1.5 rounded-lg border border-slate-800 bg-slate-900 text-slate-400 hover:text-indigo-400 hover:border-indigo-500/30 disabled:opacity-30 disabled:cursor-not-allowed transition-all shadow-sm"
                disabled={page === 0}
                onClick={() => setPage(p => p - 1)}
              >
                <ChevronLeft size={18} />
              </button>
              <button
                className="p-1.5 rounded-lg border border-slate-800 bg-slate-900 text-slate-400 hover:text-indigo-400 hover:border-indigo-500/30 disabled:opacity-30 disabled:cursor-not-allowed transition-all shadow-sm"
                disabled={page >= totalPages - 1}
                onClick={() => setPage(p => p + 1)}
              >
                <ChevronRight size={18} />
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
