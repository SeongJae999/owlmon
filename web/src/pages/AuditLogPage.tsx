import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { searchAudit, downloadAudit, type AuditFilter } from '../api/audit'
import {
  ShieldCheck, Clock, Download, RefreshCcw, AlertCircle, User, CheckCircle2, XCircle, Globe,
} from 'lucide-react'
import { cn } from '../utils/cn'
import PageToolbar from '../components/PageToolbar'

const RANGE_PRESETS = [
  { label: '24시간', minutes: 24 * 60 },
  { label: '7일', minutes: 7 * 24 * 60 },
  { label: '30일', minutes: 30 * 24 * 60 },
  { label: '90일', minutes: 90 * 24 * 60 },
] as const

const RESULT_CONFIG: Record<string, { bg: string; text: string; icon: any; label: string }> = {
  success:      { bg: 'bg-emerald-500/15', text: 'text-emerald-300', icon: CheckCircle2, label: '성공' },
  failure:      { bg: 'bg-rose-500/15',    text: 'text-rose-300',    icon: XCircle,      label: '실패' },
  unauthorized: { bg: 'bg-amber-500/15',   text: 'text-amber-300',   icon: AlertCircle,  label: '권한 거부' },
}

export default function AuditLogPage() {
  const [rangeMinutes, setRangeMinutes] = useState<number>(7 * 24 * 60)
  const [filterActor, setFilterActor] = useState('')
  const [filterAction, setFilterAction] = useState('')
  const [searchText, setSearchText] = useState('')
  const [downloading, setDownloading] = useState(false)

  const buildFilter = (): AuditFilter => {
    const now = new Date()
    const from = new Date(now.getTime() - rangeMinutes * 60_000)
    return {
      from: from.toISOString(),
      to: now.toISOString(),
      actor: filterActor || undefined,
      action: filterAction || undefined,
      limit: 1000,
    }
  }

  const { data: entries = [], isLoading, error } = useQuery({
    queryKey: ['audit', rangeMinutes, filterActor, filterAction],
    queryFn: () => searchAudit(buildFilter()),
    refetchInterval: 60_000,
  })

  // 클라이언트 사이드 본문 검색
  const filtered = useMemo(() => {
    if (!searchText.trim()) return entries
    const q = searchText.toLowerCase()
    return entries.filter(e =>
      e.actor.toLowerCase().includes(q) ||
      e.action.toLowerCase().includes(q) ||
      e.ip.toLowerCase().includes(q) ||
      (e.target_id ?? '').toLowerCase().includes(q) ||
      (e.target_type ?? '').toLowerCase().includes(q)
    )
  }, [entries, searchText])

  const uniqueActors = useMemo(() => {
    return [...new Set(entries.map(e => e.actor))].filter(Boolean).sort()
  }, [entries])

  const uniqueActions = useMemo(() => {
    return [...new Set(entries.map(e => e.action))].filter(Boolean).sort()
  }, [entries])

  const summary = useMemo(() => {
    return {
      total: filtered.length,
      success: filtered.filter(e => e.result === 'success').length,
      failure: filtered.filter(e => e.result === 'failure').length,
    }
  }, [filtered])

  const handleDownload = async (format: 'csv' | 'json') => {
    setDownloading(true)
    try {
      await downloadAudit(buildFilter(), format)
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
        icon={ShieldCheck}
        title="감사 로그"
        description="시스템 변경/접근 이력 — ISMS-P 변경 감사 추적 / 행안부 관리자 이력 보관 의무 충족"
      >
        <button
          onClick={() => handleDownload('csv')}
          disabled={downloading || filtered.length === 0}
          className="inline-flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-bold bg-slate-800 text-slate-300 border border-slate-700 hover:bg-slate-700 transition-colors disabled:opacity-40"
          title="CSV 다운로드 (감사 자료 제출용)"
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

      {/* Summary */}
      <div className="grid grid-cols-3 gap-3">
        <SummaryCard label="총 이벤트" value={summary.total} color="indigo" icon={ShieldCheck} />
        <SummaryCard label="성공" value={summary.success} color="emerald" icon={CheckCircle2} />
        <SummaryCard label="실패" value={summary.failure} color="rose" icon={XCircle} />
      </div>

      {/* Filters */}
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
            value={filterActor}
            onChange={e => setFilterActor(e.target.value)}
            className="bg-slate-800 border border-slate-700 rounded-lg px-3 py-1.5 text-sm text-slate-200 focus:outline-none focus:ring-2 focus:ring-indigo-500/30"
          >
            <option value="">전체 사용자</option>
            {uniqueActors.map(a => <option key={a} value={a}>{a}</option>)}
          </select>
          <select
            value={filterAction}
            onChange={e => setFilterAction(e.target.value)}
            className="bg-slate-800 border border-slate-700 rounded-lg px-3 py-1.5 text-sm text-slate-200 focus:outline-none focus:ring-2 focus:ring-indigo-500/30"
          >
            <option value="">전체 액션</option>
            {uniqueActions.map(a => <option key={a} value={a}>{a}</option>)}
          </select>
          <input
            value={searchText}
            onChange={e => setSearchText(e.target.value)}
            placeholder="사용자/IP/대상 검색…"
            className="flex-1 min-w-[180px] bg-slate-800 border border-slate-700 rounded-lg px-3 py-1.5 text-sm text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-indigo-500/30"
          />
          {(filterActor || filterAction || searchText) && (
            <button
              onClick={() => { setFilterActor(''); setFilterAction(''); setSearchText('') }}
              className="text-[11px] font-semibold text-slate-400 hover:text-slate-200 px-2 py-1.5"
            >
              필터 초기화
            </button>
          )}
        </div>
      </div>

      {/* Table */}
      <div className="bg-slate-900 rounded-2xl border border-slate-800 shadow-sm overflow-hidden">
        {isLoading ? (
          <div className="flex flex-col items-center justify-center py-20 text-slate-400 animate-pulse">
            <RefreshCcw size={36} className="mb-3 opacity-20 animate-spin" />
            <p className="font-medium text-sm">감사 로그 불러오는 중...</p>
          </div>
        ) : error ? (
          <div className="flex flex-col items-center justify-center py-20 text-rose-400 text-center px-6">
            <AlertCircle size={36} className="mb-3 opacity-30" />
            <p className="font-bold text-sm">감사 로그를 불러올 수 없습니다.</p>
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-20 text-slate-400 text-center px-6">
            <ShieldCheck size={36} className="mb-3 opacity-20" />
            <p className="font-medium text-sm">조건에 맞는 감사 로그가 없습니다.</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left">
              <thead className="bg-slate-800/50 border-b border-slate-800">
                <tr>
                  <th className="px-4 py-3 text-xs font-bold text-slate-400 tracking-wide w-40">시각</th>
                  <th className="px-4 py-3 text-xs font-bold text-slate-400 tracking-wide w-32">사용자</th>
                  <th className="px-4 py-3 text-xs font-bold text-slate-400 tracking-wide w-32">IP</th>
                  <th className="px-4 py-3 text-xs font-bold text-slate-400 tracking-wide w-40">액션</th>
                  <th className="px-4 py-3 text-xs font-bold text-slate-400 tracking-wide w-40">대상</th>
                  <th className="px-4 py-3 text-xs font-bold text-slate-400 tracking-wide w-24">결과</th>
                  <th className="px-4 py-3 text-xs font-bold text-slate-400 tracking-wide">상세</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-800">
                {filtered.map(e => {
                  const cfg = RESULT_CONFIG[e.result] ?? RESULT_CONFIG.success
                  return (
                    <tr key={e.id} className="hover:bg-slate-800/30 transition-colors">
                      <td className="px-4 py-2.5 whitespace-nowrap text-xs text-slate-300 tabular-nums font-mono">
                        {new Date(e.ts).toLocaleString('ko-KR', {
                          year: 'numeric', month: '2-digit', day: '2-digit',
                          hour: '2-digit', minute: '2-digit', second: '2-digit',
                        })}
                      </td>
                      <td className="px-4 py-2.5">
                        <div className="flex items-center gap-1.5 text-xs font-semibold text-slate-300">
                          <User size={11} className="text-slate-500" />
                          <span className="truncate">{e.actor}</span>
                        </div>
                      </td>
                      <td className="px-4 py-2.5 text-xs text-slate-400 font-mono">
                        <div className="flex items-center gap-1">
                          <Globe size={10} className="text-slate-600" />
                          {e.ip || '—'}
                        </div>
                      </td>
                      <td className="px-4 py-2.5 text-xs font-mono">
                        <code className="text-indigo-300">{e.action}</code>
                      </td>
                      <td className="px-4 py-2.5 text-xs text-slate-400">
                        {e.target_type ? (
                          <span className="font-mono">
                            {e.target_type}{e.target_id && <>:<span className="text-slate-300">{e.target_id}</span></>}
                          </span>
                        ) : '—'}
                      </td>
                      <td className="px-4 py-2.5">
                        <span className={cn(
                          "inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-bold whitespace-nowrap",
                          cfg.bg, cfg.text
                        )}>
                          <cfg.icon size={10} />
                          {cfg.label}
                        </span>
                      </td>
                      <td className="px-4 py-2.5 text-xs text-slate-400">
                        {e.details ? (
                          <code className="font-mono text-[11px] break-all line-clamp-2" title={JSON.stringify(e.details)}>
                            {JSON.stringify(e.details)}
                          </code>
                        ) : '—'}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
            <div className="px-4 py-2.5 bg-slate-800/30 border-t border-slate-800 text-[11px] text-slate-500">
              {filtered.length}건 표시 {entries.length > filtered.length && `(전체 ${entries.length}건 중)`}
              {entries.length >= 1000 && (
                <span className="ml-2 inline-flex items-center gap-1 text-amber-400">
                  <AlertCircle size={11} /> 1000건 한도 도달 — 시간 범위 좁히세요
                </span>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

function SummaryCard({ label, value, color, icon: Icon }: { label: string; value: number; color: string; icon: any }) {
  const themes: Record<string, string> = {
    indigo:  'bg-indigo-500/10 border-indigo-500/30 text-indigo-300',
    emerald: 'bg-emerald-500/10 border-emerald-500/30 text-emerald-300',
    rose:    'bg-rose-500/10 border-rose-500/30 text-rose-300',
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
