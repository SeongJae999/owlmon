import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Sparkles, RefreshCw, AlertTriangle, Filter, ChevronDown, ChevronRight } from 'lucide-react'
import {
  listInsights,
  getInsightStatus,
  type Severity,
  type Category,
} from '../api/insights'
import { cn } from '../utils/cn'

const SEVERITY_STYLE: Record<Severity, { bg: string; text: string; label: string }> = {
  critical: { bg: 'bg-rose-500/15 border-rose-500/40',   text: 'text-rose-300',   label: '심각' },
  high:     { bg: 'bg-orange-500/15 border-orange-500/40', text: 'text-orange-300', label: '높음' },
  medium:   { bg: 'bg-amber-500/15 border-amber-500/40',  text: 'text-amber-300',  label: '중간' },
  low:      { bg: 'bg-slate-700/30 border-slate-600/40',  text: 'text-slate-400',  label: '낮음' },
}

const CATEGORY_LABEL: Record<Category, string> = {
  auth: '인증',
  network: '네트워크',
  disk: '디스크',
  db: 'DB',
  app: '앱',
  security: '보안',
  other: '기타',
}

function formatTime(iso: string): string {
  const d = new Date(iso)
  return d.toLocaleString('ko-KR', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export default function InsightDashboard() {
  const [severity, setSeverity] = useState<Severity | ''>('')
  const [hostFilter, setHostFilter] = useState('')
  const [expanded, setExpanded] = useState<Set<number>>(new Set())

  const { data: status } = useQuery({
    queryKey: ['insightStatus'],
    queryFn: getInsightStatus,
    refetchInterval: 60_000,
  })

  const { data: listResult, isLoading, refetch, isFetching } = useQuery({
    queryKey: ['insightList', severity, hostFilter],
    queryFn: () =>
      listInsights({
        severity: severity || undefined,
        host: hostFilter || undefined,
        limit: 100,
      }),
    refetchInterval: 60_000,
    enabled: status?.enabled !== false,
  })

  const toggleExpand = (id: number) => {
    setExpanded(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const items = listResult?.items ?? []

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-xl bg-indigo-600/20 flex items-center justify-center">
            <Sparkles size={20} className="text-indigo-400" />
          </div>
          <div>
            <h1 className="text-xl font-bold text-slate-100">AI 로그 인사이트</h1>
            <p className="text-sm text-slate-500">
              5분마다 이상 로그(ERROR/WARN)를 LLM이 자동 분석한 결과
            </p>
          </div>
        </div>
        <button
          onClick={() => refetch()}
          disabled={isFetching}
          className="flex items-center gap-2 px-4 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 text-sm font-semibold transition-colors disabled:opacity-50"
        >
          <RefreshCw size={16} className={isFetching ? 'animate-spin' : ''} />
          새로고침
        </button>
      </div>

      {/* Status banner — 비활성 시만 표시 */}
      {status && !status.enabled && (
        <div className="rounded-xl bg-amber-500/10 border border-amber-500/30 p-4 flex items-start gap-3">
          <AlertTriangle size={20} className="text-amber-400 mt-0.5 shrink-0" />
          <div>
            <p className="text-sm font-semibold text-amber-300">AI 분석 기능이 비활성화되어 있습니다</p>
            <p className="text-xs text-amber-200/80 mt-1 leading-relaxed">
              서버 환경변수{' '}
              <code className="bg-amber-500/20 px-1.5 py-0.5 rounded text-amber-200">
                OWLMON_LOGINSIGHT_ENABLED=true
              </code>{' '}
              와 LLM Provider 설정(<code className="bg-amber-500/20 px-1.5 py-0.5 rounded text-amber-200">OWLMON_LLM_PROVIDER</code>)이 필요합니다.
            </p>
          </div>
        </div>
      )}

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-3 p-4 rounded-xl bg-slate-900 border border-slate-800">
        <Filter size={16} className="text-slate-500" />
        <select
          value={severity}
          onChange={e => setSeverity(e.target.value as Severity | '')}
          className="px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-slate-200 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
        >
          <option value="">전체 심각도</option>
          <option value="critical">심각</option>
          <option value="high">높음</option>
          <option value="medium">중간</option>
          <option value="low">낮음</option>
        </select>
        <input
          type="text"
          placeholder="호스트명 필터"
          value={hostFilter}
          onChange={e => setHostFilter(e.target.value)}
          className="px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-slate-200 text-sm placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-indigo-500 flex-1 min-w-[200px]"
        />
        <span className="text-xs text-slate-500 font-medium">총 {items.length}건</span>
      </div>

      {/* List */}
      {isLoading ? (
        <div className="text-center py-12 text-slate-500">로딩 중...</div>
      ) : items.length === 0 ? (
        <div className="text-center py-16 rounded-xl bg-slate-900 border border-slate-800">
          <Sparkles size={32} className="mx-auto text-slate-700 mb-3" />
          <p className="text-slate-400">분석된 인사이트가 없습니다</p>
          <p className="text-xs text-slate-500 mt-1">
            워커가 5분마다 동작합니다. 새 이상 로그가 발생하면 여기 표시됩니다.
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          {items.map(ins => {
            const style = SEVERITY_STYLE[ins.severity]
            const isOpen = expanded.has(ins.id)
            return (
              <div
                key={ins.id}
                className={cn(
                  'rounded-xl border transition-colors',
                  style.bg
                )}
              >
                <button
                  onClick={() => toggleExpand(ins.id)}
                  className="w-full p-4 text-left flex items-start justify-between gap-4 hover:bg-slate-900/30 transition-colors rounded-xl"
                >
                  <div className="flex-1 min-w-0">
                    <div className="flex flex-wrap items-center gap-2 mb-2">
                      <span
                        className={cn(
                          'px-2 py-0.5 rounded text-xs font-bold border',
                          style.text,
                          style.bg
                        )}
                      >
                        {style.label}
                      </span>
                      <span className="px-2 py-0.5 rounded text-xs font-medium bg-slate-800 text-slate-400">
                        {CATEGORY_LABEL[ins.category] ?? ins.category}
                      </span>
                      {ins.host_name && (
                        <span className="text-xs text-slate-400 font-semibold">
                          {ins.host_name}
                        </span>
                      )}
                      {ins.needs_human && (
                        <span className="px-2 py-0.5 rounded text-xs font-bold bg-rose-500/20 text-rose-300">
                          확인 필요
                        </span>
                      )}
                    </div>
                    <p className="text-sm font-semibold text-slate-100">{ins.summary_ko}</p>
                  </div>
                  <div className="flex items-center gap-2 shrink-0">
                    <span className="text-xs text-slate-500 font-medium">
                      {formatTime(ins.created_at)}
                    </span>
                    {isOpen ? (
                      <ChevronDown size={16} className="text-slate-500" />
                    ) : (
                      <ChevronRight size={16} className="text-slate-500" />
                    )}
                  </div>
                </button>

                {isOpen && (
                  <div className="px-4 pb-4 pt-1 border-t border-slate-700/40 space-y-3 text-sm">
                    {ins.root_cause_ko && (
                      <div>
                        <p className="text-xs font-semibold text-slate-500 mb-1 mt-3">추정 원인</p>
                        <p className="text-slate-200 leading-relaxed">{ins.root_cause_ko}</p>
                      </div>
                    )}
                    {ins.action_ko && (
                      <div>
                        <p className="text-xs font-semibold text-slate-500 mb-1">권장 조치</p>
                        <p className="text-slate-200 leading-relaxed whitespace-pre-wrap">
                          {ins.action_ko}
                        </p>
                      </div>
                    )}
                    <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-slate-500 pt-2 border-t border-slate-700/30">
                      <span>모델: {ins.model_name}</span>
                      {ins.latency_ms !== undefined && ins.latency_ms > 0 && (
                        <span>지연: {ins.latency_ms}ms</span>
                      )}
                      <span>템플릿: {ins.template_hash.slice(0, 8)}</span>
                      {ins.sample_log_ids?.length > 0 && (
                        <span>샘플: {ins.sample_log_ids.length}건</span>
                      )}
                    </div>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
