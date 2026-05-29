import { useState, useMemo } from 'react'
import { getAlertHistory, downloadAlertHistory, type AlertHistoryFilter } from '../api/alert'
import { severityLabel } from '../api/rules'
import { useQuery } from '@tanstack/react-query'
import {
  History, AlertCircle, AlertTriangle, Info, Server, RefreshCcw, Clock, Download, Settings, Sparkles, X,
} from 'lucide-react'
import { Link } from 'react-router-dom'
import { cn } from '../utils/cn'
import PageToolbar from './PageToolbar'
import { getLLMStatus, summarizeAlerts, type SummaryResult } from '../api/llm'

const SEVERITY_CONFIG: Record<string, { bg: string, text: string, icon: any }> = {
  critical: { bg: 'bg-rose-500/15',  text: 'text-rose-300',  icon: AlertCircle },
  warning:  { bg: 'bg-amber-500/15', text: 'text-amber-300', icon: AlertTriangle },
  info:     { bg: 'bg-blue-500/15',  text: 'text-blue-300',  icon: Info },
}

const RANGE_PRESETS = [
  { label: '24시간', minutes: 24 * 60 },
  { label: '7일',    minutes: 7 * 24 * 60 },
  { label: '30일',   minutes: 30 * 24 * 60 },
  { label: '90일',   minutes: 90 * 24 * 60 },
] as const

export default function AlertHistory() {
  const [rangeMinutes, setRangeMinutes] = useState<number>(7 * 24 * 60) // 디폴트 7일
  const [filterSev, setFilterSev] = useState<string>('')
  const [filterHost, setFilterHost] = useState<string>('')
  const [searchText, setSearchText] = useState('')
  const [downloading, setDownloading] = useState(false)
  const [summaryOpen, setSummaryOpen] = useState(false)
  const [aiSummary, setAiSummary] = useState<SummaryResult | null>(null)
  const [summaryLoading, setSummaryLoading] = useState(false)
  const [summaryError, setSummaryError] = useState<string | null>(null)

  const { data: llmStatus } = useQuery({
    queryKey: ['llmStatus'],
    queryFn: getLLMStatus,
    staleTime: 5 * 60_000,
  })

  const handleSummary = async () => {
    setSummaryOpen(true)
    setAiSummary(null)
    setSummaryError(null)
    setSummaryLoading(true)
    try {
      const hours = Math.round(rangeMinutes / 60)
      const res = await summarizeAlerts(hours)
      setAiSummary(res)
    } catch (e: any) {
      setSummaryError(e?.response?.data?.toString?.() || e?.message || 'AI 요약 실패')
    } finally {
      setSummaryLoading(false)
    }
  }

  // 시간 범위 매 쿼리 시점에 새로 계산
  const buildFilter = (): AlertHistoryFilter => {
    const now = new Date()
    const from = new Date(now.getTime() - rangeMinutes * 60_000)
    return {
      from: from.toISOString(),
      to: now.toISOString(),
      severity: filterSev || undefined,
      host: filterHost || undefined,
      limit: 1000,
    }
  }

  const { data: records = [], isLoading, error } = useQuery({
    queryKey: ['alertHistory', rangeMinutes, filterSev, filterHost],
    queryFn: () => getAlertHistory(buildFilter()),
    refetchInterval: 60_000,
  })

  // 클라이언트 사이드 본문 검색
  const filtered = useMemo(() => {
    if (!searchText.trim()) return records
    const q = searchText.toLowerCase()
    return records.filter(r =>
      r.subject?.toLowerCase().includes(q) ||
      r.body?.toLowerCase().includes(q) ||
      r.host?.toLowerCase().includes(q)
    )
  }, [records, searchText])

  // 호스트 목록 (드롭다운용)
  const uniqueHosts = useMemo(() => {
    return [...new Set(records.map(r => r.host))].filter(Boolean).sort()
  }, [records])

  // 요약 통계 (현재 필터 결과 기준)
  const summary = useMemo(() => {
    return {
      total: filtered.length,
      critical: filtered.filter(r => r.severity === 'critical').length,
      warning: filtered.filter(r => r.severity === 'warning').length,
      info: filtered.filter(r => r.severity === 'info').length,
    }
  }, [filtered])

  const handleDownload = async (format: 'csv' | 'json') => {
    setDownloading(true)
    try {
      await downloadAlertHistory(buildFilter(), format)
    } catch (e) {
      console.error(e)
      alert('다운로드 실패')
    } finally {
      setDownloading(false)
    }
  }

  return (
    <div className="space-y-5">
      <PageToolbar
        icon={History}
        title="알림 히스토리"
        description="이메일로 발송된 모든 알림 기록 — 감사 자료/회고용"
      >
        {llmStatus?.enabled && (
          <button
            onClick={handleSummary}
            disabled={summaryLoading}
            className="inline-flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-bold bg-violet-500/15 text-violet-300 border border-violet-500/30 hover:bg-violet-500/25 transition-colors disabled:opacity-40"
            title="AI가 선택 기간 알림을 한국어로 요약"
          >
            {summaryLoading ? <RefreshCcw size={12} className="animate-spin" /> : <Sparkles size={12} />}
            AI 요약
          </button>
        )}
        <Link
          to="/settings"
          className="inline-flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-bold bg-slate-800 text-slate-300 border border-slate-700 hover:bg-slate-700 transition-colors"
          title="알림 정책/임계값/수신자 설정"
        >
          <Settings size={12} /> 알림 설정
        </Link>
        <button
          onClick={() => handleDownload('csv')}
          disabled={downloading || filtered.length === 0}
          className="inline-flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-bold bg-slate-800 text-slate-300 border border-slate-700 hover:bg-slate-700 transition-colors disabled:opacity-40"
          title="CSV 다운로드 (감사 자료용)"
        >
          {downloading ? <RefreshCcw size={12} className="animate-spin" /> : <Download size={12} />}
          CSV
        </button>
        <button
          onClick={() => handleDownload('json')}
          disabled={downloading || filtered.length === 0}
          className="inline-flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-bold bg-slate-800 text-slate-300 border border-slate-700 hover:bg-slate-700 transition-colors disabled:opacity-40"
          title="JSON 다운로드"
        >
          {downloading ? <RefreshCcw size={12} className="animate-spin" /> : <Download size={12} />}
          JSON
        </button>
      </PageToolbar>

      {records.length >= 1000 && (
        <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-amber-500/10 border border-amber-500/30 text-xs text-amber-300">
          <AlertTriangle size={14} className="shrink-0" />
          <span>전체 결과 1000건 한도 도달 — 시간 범위를 좁히거나 필터를 적용하세요.</span>
        </div>
      )}

      {/* Summary Cards */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <SummaryCard icon={History}        label="총 알림"    value={summary.total}    color="indigo" />
        <SummaryCard icon={AlertCircle}    label="심각"      value={summary.critical} color="rose" />
        <SummaryCard icon={AlertTriangle}  label="주의"      value={summary.warning}  color="amber" />
        <SummaryCard icon={Info}           label="정보"      value={summary.info}     color="blue" />
      </div>

      {/* Filter Bar */}
      <div className="bg-slate-900 rounded-2xl border border-slate-800 p-3 space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-[11px] font-bold text-slate-500 uppercase tracking-wider flex items-center gap-1">
            <Clock size={11} /> 시간 범위
          </span>
          <div className="flex gap-1 bg-slate-800 rounded-lg p-0.5">
            {RANGE_PRESETS.map(r => (
              <button
                key={r.label}
                onClick={() => setRangeMinutes(r.minutes)}
                className={cn(
                  "px-2.5 py-1 rounded text-[11px] font-bold transition-colors",
                  rangeMinutes === r.minutes ? "bg-indigo-600 text-white" : "text-slate-400 hover:text-slate-200"
                )}
              >
                {r.label}
              </button>
            ))}
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <select
            value={filterSev}
            onChange={e => setFilterSev(e.target.value)}
            className="bg-slate-800 border border-slate-700 rounded-lg px-3 py-1.5 text-sm text-slate-200 focus:outline-none focus:ring-2 focus:ring-indigo-500/30"
          >
            <option value="">전체 심각도</option>
            <option value="critical">심각</option>
            <option value="warning">주의</option>
            <option value="info">정보</option>
          </select>
          <select
            value={filterHost}
            onChange={e => setFilterHost(e.target.value)}
            className="bg-slate-800 border border-slate-700 rounded-lg px-3 py-1.5 text-sm text-slate-200 focus:outline-none focus:ring-2 focus:ring-indigo-500/30 max-w-[200px] truncate"
          >
            <option value="">전체 호스트</option>
            {uniqueHosts.map(h => <option key={h} value={h}>{h}</option>)}
          </select>
          <input
            value={searchText}
            onChange={e => setSearchText(e.target.value)}
            placeholder="제목/본문 검색…"
            className="flex-1 min-w-[180px] bg-slate-800 border border-slate-700 rounded-lg px-3 py-1.5 text-sm text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-indigo-500/30"
          />
          {(filterSev || filterHost || searchText) && (
            <button
              onClick={() => { setFilterSev(''); setFilterHost(''); setSearchText('') }}
              className="text-[11px] font-semibold text-slate-400 hover:text-slate-200 px-2 py-1.5"
            >
              필터 초기화
            </button>
          )}
        </div>
      </div>

      {/* Content */}
      <div className="bg-slate-900 rounded-2xl border border-slate-800 shadow-sm overflow-hidden">
        {isLoading ? (
          <div className="flex flex-col items-center justify-center py-20 text-slate-400 animate-pulse">
            <RefreshCcw size={36} className="mb-3 opacity-20 animate-spin" />
            <p className="font-medium text-sm">알림 이력을 불러오는 중...</p>
          </div>
        ) : error ? (
          <div className="flex flex-col items-center justify-center py-20 text-rose-400 text-center px-6">
            <AlertCircle size={36} className="mb-3 opacity-30" />
            <p className="font-bold text-sm">히스토리를 불러올 수 없습니다.</p>
            <p className="text-xs opacity-70 mt-1">서버 또는 DB 연결 오류일 수 있습니다.</p>
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-20 text-slate-400 text-center px-6">
            <History size={36} className="mb-3 opacity-20" />
            <p className="font-medium text-sm">조건에 맞는 알림이 없습니다.</p>
            <p className="text-xs opacity-70 mt-1">시간 범위를 늘리거나 필터를 해제해 보세요.</p>
          </div>
        ) : (
          <>
            {/* Desktop table */}
            <div className="hidden md:block overflow-x-auto">
              <table className="w-full text-left border-collapse">
                <thead>
                  <tr className="bg-slate-800/50 border-b border-slate-800">
                    <th className="px-4 py-3 text-xs font-bold text-slate-400 tracking-wide w-48">발송 시각</th>
                    <th className="px-4 py-3 text-xs font-bold text-slate-400 tracking-wide w-24">심각도</th>
                    <th className="px-4 py-3 text-xs font-bold text-slate-400 tracking-wide w-40">호스트</th>
                    <th className="px-4 py-3 text-xs font-bold text-slate-400 tracking-wide">내용</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-800">
                  {filtered.map(r => {
                    const cfg = SEVERITY_CONFIG[r.severity] ?? { bg: 'bg-slate-800', text: 'text-slate-400', icon: Info }
                    return (
                      <tr key={r.id} className="hover:bg-slate-800/30 transition-colors">
                        <td className="px-4 py-3 whitespace-nowrap text-xs font-medium text-slate-300 tabular-nums">
                          {new Date(r.sent_at).toLocaleString('ko-KR', {
                            year: 'numeric', month: '2-digit', day: '2-digit',
                            hour: '2-digit', minute: '2-digit', second: '2-digit',
                          })}
                        </td>
                        <td className="px-4 py-3">
                          <span className={cn(
                            "inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-bold whitespace-nowrap",
                            cfg.bg, cfg.text
                          )}>
                            <cfg.icon size={10} />
                            {severityLabel(r.severity)}
                          </span>
                        </td>
                        <td className="px-4 py-3">
                          <div className="flex items-center gap-1.5 text-xs font-semibold text-slate-300">
                            <Server size={11} className="text-slate-500" />
                            <span className="truncate">{r.host}</span>
                          </div>
                        </td>
                        <td className="px-4 py-3">
                          <div className="text-sm font-bold text-slate-100 line-clamp-1" title={r.subject}>{r.subject}</div>
                          <div className="text-xs text-slate-400 line-clamp-2 break-words mt-0.5" title={r.body}>{r.body}</div>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>

            {/* Mobile cards */}
            <div className="md:hidden divide-y divide-slate-800">
              {filtered.map(r => {
                const cfg = SEVERITY_CONFIG[r.severity] ?? { bg: 'bg-slate-800', text: 'text-slate-400', icon: Info }
                return (
                  <div key={r.id} className="p-3 space-y-1.5">
                    <div className="flex items-center justify-between gap-2">
                      <span className={cn(
                        "inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-bold",
                        cfg.bg, cfg.text
                      )}>
                        <cfg.icon size={10} />
                        {severityLabel(r.severity)}
                      </span>
                      <span className="text-[11px] text-slate-400 tabular-nums">
                        {new Date(r.sent_at).toLocaleString('ko-KR', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })}
                      </span>
                    </div>
                    <div className="flex items-center gap-1.5 text-xs text-slate-300">
                      <Server size={11} className="text-slate-500" />
                      <span className="font-semibold truncate">{r.host}</span>
                    </div>
                    <div className="text-sm font-bold text-slate-100 line-clamp-1">{r.subject}</div>
                    <div className="text-xs text-slate-400 line-clamp-2 break-words">{r.body}</div>
                  </div>
                )
              })}
            </div>

            <div className="px-4 py-2.5 bg-slate-800/30 border-t border-slate-800 text-[11px] text-slate-500">
              {filtered.length}건 표시 {records.length > filtered.length && `(전체 ${records.length}건 중)`}
            </div>
          </>
        )}
      </div>

      {/* AI 요약 Modal */}
      {summaryOpen && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4"
          onClick={() => setSummaryOpen(false)}
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
                  <h3 className="text-base font-bold text-slate-100">AI 알림 요약</h3>
                  <p className="text-[11px] text-slate-500">
                    최근 {Math.round(rangeMinutes / 60)}시간
                    {aiSummary?.cached && ' · 캐시'}
                    {aiSummary !== null && ` · ${aiSummary.total}건 분석`}
                    {llmStatus?.provider && ` · ${llmStatus.provider}`}
                  </p>
                </div>
              </div>
              <button onClick={() => setSummaryOpen(false)} className="p-1 rounded-md text-slate-500 hover:text-slate-300 hover:bg-slate-800">
                <X size={16} />
              </button>
            </div>

            <div className="p-5">
              {summaryLoading && (
                <div className="flex items-center gap-2 text-sm text-slate-400">
                  <RefreshCcw size={14} className="animate-spin" />
                  AI가 알림 패턴 분석 중...
                </div>
              )}
              {summaryError && (
                <div className="flex items-start gap-2 px-3 py-2 rounded-lg bg-rose-500/10 border border-rose-500/30 text-sm text-rose-300">
                  <AlertCircle size={14} className="shrink-0 mt-0.5" />
                  <span>{summaryError}</span>
                </div>
              )}
              {aiSummary && (
                <pre className="text-sm text-slate-200 whitespace-pre-wrap break-words font-sans leading-relaxed">
                  {aiSummary.summary}
                </pre>
              )}
            </div>

            <div className="p-3 bg-slate-800/30 border-t border-slate-800 text-[10px] text-slate-500">
              AI 요약은 참고용입니다. 핵심 알림은 본문에서 직접 확인 권장.
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

function SummaryCard({ icon: Icon, label, value, color }: {
  icon: any; label: string; value: number; color: string
}) {
  const themes: Record<string, string> = {
    indigo: 'bg-indigo-500/10 border-indigo-500/30 text-indigo-300',
    rose:   'bg-rose-500/10 border-rose-500/30 text-rose-300',
    amber:  'bg-amber-500/10 border-amber-500/30 text-amber-300',
    blue:   'bg-blue-500/10 border-blue-500/30 text-blue-300',
  }
  return (
    <div className={cn("rounded-xl p-3 border flex items-center gap-3", themes[color])}>
      <Icon size={20} className="opacity-70 shrink-0" />
      <div className="min-w-0">
        <div className="text-[11px] font-semibold opacity-80 truncate">{label}</div>
        <div className="text-xl font-bold tabular-nums">{value.toLocaleString()}</div>
      </div>
    </div>
  )
}
