import { useState } from 'react'
import { searchLogs, getLogSources, type LogSearchParams, type LogRecord } from '../api/logs'
import { listRules, severityLabel } from '../api/rules'
import { useQuery } from '@tanstack/react-query'
import { Search, RefreshCcw, Server, FileText, ChevronLeft, ChevronRight, Info, AlertCircle, Tag, ListChecks } from 'lucide-react'
import { cn } from '../utils/cn'
import AnnotateModal from './AnnotateModal'

const LEVEL_CONFIG: Record<string, { bg: string, text: string }> = {
  ERROR: { bg: 'bg-rose-500/15', text: 'text-rose-300' },
  FATAL: { bg: 'bg-rose-500/15', text: 'text-rose-300' },
  WARN: { bg: 'bg-amber-500/15', text: 'text-amber-300' },
  WARNING: { bg: 'bg-amber-500/15', text: 'text-amber-300' },
  INFO: { bg: 'bg-blue-500/15', text: 'text-blue-300' },
  DEBUG: { bg: 'bg-slate-800', text: 'text-slate-400' },
}

export default function LogViewer() {
  const [autoRefresh, setAutoRefresh] = useState(false)
  const [refreshSec, setRefreshSec] = useState(30) // 5 → 30 (사용자 읽는 동안 깜빡임 방지)
  const [host, setHost] = useState('')
  const [source, setSource] = useState('')
  const [level, setLevel] = useState('')
  const [ruleID, setRuleID] = useState<number | ''>('')
  const [queryText, setQueryText] = useState('')
  const [page, setPage] = useState(0)
  const [selectedLog, setSelectedLog] = useState<LogRecord | null>(null)
  const limit = 50

  const { data: sources = [] } = useQuery({
    queryKey: ['logSources'],
    queryFn: getLogSources
  })

  const { data: rules = [] } = useQuery({
    queryKey: ['log-rules'],
    queryFn: listRules,
    staleTime: 60_000,
  })

  const { data: logData, isLoading, refetch } = useQuery({
    queryKey: ['logs', host, source, level, ruleID, queryText, page],
    queryFn: () => {
      const params: LogSearchParams = { limit, offset: page * limit }
      if (host) params.host = host
      if (source) params.source = source
      if (level) params.level = level
      if (queryText) params.query = queryText
      if (ruleID) params.rule_id = ruleID
      return searchLogs(params)
    },
    refetchInterval: autoRefresh ? refreshSec * 1000 : false,
  })

  const records = logData?.records || []
  const total = logData?.total || 0
  const uniqueHosts = [...new Set(sources.map(s => s.host))]
  const filteredSources = host ? sources.filter(s => s.host === host) : sources
  const uniqueSources = [...new Set(filteredSources.map(s => s.source))]
  const totalPages = Math.ceil(total / limit)

  const handleSearch = () => {
    setPage(0)
    refetch()
  }

  return (
    <div className="space-y-6">
      {/* Filter Bar */}
      <div className="bg-slate-900 p-4 rounded-3xl border border-slate-800 shadow-sm">
        <div className="flex flex-col lg:flex-row gap-4 items-end lg:items-center">
          <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 flex-1 w-full">
            <div className="space-y-1">
              <label className="text-[10px] font-bold text-slate-400 uppercase ml-1 tracking-wider">Host</label>
              <select 
                className="w-full bg-slate-800 border border-slate-800 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition-all cursor-pointer"
                value={host} 
                onChange={e => { setHost(e.target.value); setPage(0) }}
              >
                <option value="">전체 호스트</option>
                {uniqueHosts.map(h => <option key={h} value={h}>{h}</option>)}
              </select>
            </div>
            <div className="space-y-1">
              <label className="text-[10px] font-bold text-slate-400 uppercase ml-1 tracking-wider">Source</label>
              <select 
                className="w-full bg-slate-800 border border-slate-800 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition-all cursor-pointer"
                value={source} 
                onChange={e => { setSource(e.target.value); setPage(0) }}
              >
                <option value="">전체 소스</option>
                {uniqueSources.map(s => <option key={s} value={s}>{s}</option>)}
              </select>
            </div>
            <div className="space-y-1">
              <label className="text-[10px] font-bold text-slate-400 uppercase ml-1 tracking-wider">Level</label>
              <select
                className="w-full bg-slate-800 border border-slate-800 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition-all cursor-pointer"
                value={level}
                onChange={e => { setLevel(e.target.value); setPage(0) }}
              >
                <option value="">전체 레벨</option>
                <option value="ERROR">ERROR</option>
                <option value="WARN">WARN</option>
                <option value="INFO">INFO</option>
                <option value="DEBUG">DEBUG</option>
              </select>
            </div>
            <div className="space-y-1">
              <label className="text-[10px] font-bold text-slate-400 uppercase ml-1 tracking-wider">Rule</label>
              <select
                className="w-full bg-slate-800 border border-slate-800 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition-all cursor-pointer"
                value={ruleID}
                onChange={e => { setRuleID(e.target.value ? Number(e.target.value) : ''); setPage(0) }}
              >
                <option value="">전체 룰</option>
                {rules.map(r => <option key={r.id} value={r.id}>{r.name}</option>)}
              </select>
            </div>
          </div>

          <div className="flex gap-2 w-full lg:w-auto">
            <div className="relative flex-1 lg:w-64">
              <input
                className="w-full bg-slate-800 border border-slate-800 rounded-lg pl-9 pr-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition-all"
                placeholder="검색 키워드..."
                value={queryText}
                onChange={e => setQueryText(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter') handleSearch() }}
              />
              <Search className="absolute left-3 top-2.5 text-slate-400" size={16} />
            </div>
            <button 
              className="bg-indigo-600 hover:bg-indigo-700 text-white px-4 py-2 rounded-lg text-sm font-bold transition-colors shadow-sm shadow-indigo-500/20 shrink-0"
              onClick={handleSearch}
            >
              검색
            </button>
          </div>
        </div>

        <div className="mt-4 pt-4 border-t border-slate-800 flex items-center justify-between">
          <div className="flex items-center gap-4">
            <label className="flex items-center gap-2 cursor-pointer group">
              <div className="relative inline-flex items-center cursor-pointer">
                <input 
                  type="checkbox" 
                  className="sr-only peer" 
                  checked={autoRefresh} 
                  onChange={e => setAutoRefresh(e.target.checked)} 
                />
                <div className="w-9 h-5 bg-slate-700 peer-focus:outline-none peer-focus:ring-4 peer-focus:ring-indigo-300 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-slate-900 after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-indigo-600"></div>
              </div>
              <span className="text-xs font-semibold text-slate-500 group-hover:text-slate-400 transition-colors">자동 새로고침</span>
            </label>
            <select
              value={refreshSec}
              onChange={e => setRefreshSec(Number(e.target.value))}
              disabled={!autoRefresh}
              className="text-xs bg-slate-800 border border-slate-700 rounded px-2 py-1 text-slate-300 disabled:opacity-40"
            >
              <option value={10}>10초</option>
              <option value={30}>30초</option>
              <option value={60}>1분</option>
              <option value={300}>5분</option>
            </select>
            {autoRefresh && (
              <RefreshCcw size={12} className="text-indigo-500 animate-spin" />
            )}
          </div>
          <div className="text-[11px] font-bold text-slate-400 bg-slate-800 px-2 py-1 rounded">
            검색 결과: <span className="text-slate-400">{total.toLocaleString()}</span>건
          </div>
        </div>
      </div>

      {/* Log Content */}
      <div className="bg-slate-900 rounded-3xl border border-slate-800 shadow-sm overflow-hidden">
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
        ) : (
          <>
            {/* Desktop: Table view (md+) */}
            <div className="hidden md:block overflow-x-auto">
              <table className="w-full text-left border-collapse">
                <thead>
                  <tr className="bg-slate-800/50 border-b border-slate-800">
                    <th className="px-3 py-2 text-[10px] font-bold text-slate-400 uppercase tracking-wider w-24 shrink-0">시간</th>
                    <th className="px-3 py-2 text-[10px] font-bold text-slate-400 uppercase tracking-wider w-20 shrink-0">레벨</th>
                    <th className="px-3 py-2 text-[10px] font-bold text-slate-400 uppercase tracking-wider w-28 shrink-0">호스트</th>
                    <th className="px-3 py-2 text-[10px] font-bold text-slate-400 uppercase tracking-wider w-24 shrink-0">소스</th>
                    <th className="px-3 py-2 text-[10px] font-bold text-slate-400 uppercase tracking-wider">내용</th>
                    <th className="px-3 py-2 text-[10px] font-bold text-slate-400 uppercase tracking-wider w-16 shrink-0 text-right">라벨</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-800">
                  {records.map(r => {
                    const lvlCfg = LEVEL_CONFIG[r.level?.toUpperCase()] ?? { bg: 'bg-slate-800', text: 'text-slate-400' }
                    const ts = new Date(r.timestamp)
                    return (
                      <tr key={r.id} className="hover:bg-slate-800/50 transition-colors group align-top">
                        <td className="px-3 py-1.5 text-[11px] text-slate-400 font-mono whitespace-nowrap"
                            title={ts.toLocaleString('ko-KR')}>
                          {ts.toLocaleTimeString('ko-KR', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })}
                        </td>
                        <td className="px-3 py-1.5">
                          {r.level && (
                            <span className={cn(
                              "px-1.5 py-0.5 rounded text-[10px] font-bold uppercase tracking-tight",
                              lvlCfg.bg, lvlCfg.text
                            )}>
                              {r.level}
                            </span>
                          )}
                        </td>
                        <td className="px-3 py-1.5 text-xs font-semibold text-slate-300 truncate group-hover:text-indigo-400 transition-colors">
                          <div className="flex items-center gap-1">
                            <Server size={10} className="opacity-40 shrink-0" />
                            {r.host}
                          </div>
                        </td>
                        <td className="px-3 py-1.5 text-[11px] text-slate-500 truncate">
                          {r.source}
                        </td>
                        <td className="px-3 py-1.5">
                          <code className="block text-[11px] leading-snug text-slate-300 whitespace-pre-wrap break-all font-mono">
                            {r.line}
                          </code>
                          {r.matched_rules && r.matched_rules.length > 0 && (
                            <div className="flex flex-wrap gap-1 mt-1.5">
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
                                      "inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-semibold border hover:opacity-80",
                                      c
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
                        </td>
                        <td className="px-3 py-1.5 text-right">
                          <button
                            onClick={() => setSelectedLog(r)}
                            className="opacity-0 group-hover:opacity-100 transition-opacity inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-bold text-indigo-300 bg-indigo-500/10 hover:bg-indigo-500/20 border border-indigo-500/30"
                            title="이 로그에 원인/조치 라벨 부여"
                          >
                            <Tag size={10} />
                            라벨
                          </button>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>

            {/* Mobile: Card view (< md) */}
            <div className="md:hidden divide-y divide-slate-800">
              {records.map(r => {
                const lvlCfg = LEVEL_CONFIG[r.level?.toUpperCase()] ?? { bg: 'bg-slate-800', text: 'text-slate-400' }
                const ts = new Date(r.timestamp)
                return (
                  <div key={r.id} className="px-3 py-2.5 hover:bg-slate-800/40 transition-colors">
                    {/* Top row: 시간 + 레벨 + 라벨 버튼 */}
                    <div className="flex items-center justify-between gap-2 mb-1.5">
                      <div className="flex items-center gap-2 min-w-0">
                        <span className="text-[10px] text-slate-400 font-mono whitespace-nowrap shrink-0"
                              title={ts.toLocaleString('ko-KR')}>
                          {ts.toLocaleTimeString('ko-KR', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })}
                        </span>
                        {r.level && (
                          <span className={cn(
                            "px-1.5 py-0.5 rounded text-[10px] font-bold uppercase tracking-tight shrink-0",
                            lvlCfg.bg, lvlCfg.text
                          )}>
                            {r.level}
                          </span>
                        )}
                      </div>
                      <button
                        onClick={() => setSelectedLog(r)}
                        className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-bold text-indigo-300 bg-indigo-500/10 hover:bg-indigo-500/20 border border-indigo-500/30 shrink-0"
                        title="이 로그에 원인/조치 라벨 부여"
                      >
                        <Tag size={10} />
                        라벨
                      </button>
                    </div>

                    {/* Meta: host · source */}
                    <div className="flex items-center gap-1.5 mb-1.5 text-[10px] text-slate-500">
                      <Server size={10} className="opacity-50 shrink-0" />
                      <span className="font-semibold text-slate-400 truncate">{r.host}</span>
                      <span className="opacity-50">·</span>
                      <span className="truncate">{r.source}</span>
                    </div>

                    {/* Line */}
                    <code className="block text-[11px] leading-snug text-slate-300 whitespace-pre-wrap break-all font-mono">
                      {r.line}
                    </code>

                    {/* Matched rules */}
                    {r.matched_rules && r.matched_rules.length > 0 && (
                      <div className="flex flex-wrap gap-1 mt-1.5">
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
                                "inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-semibold border hover:opacity-80",
                                c
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
          </>
        )}

        {/* Annotate Modal */}
        {selectedLog && (
          <AnnotateModal
            log={selectedLog}
            onClose={() => setSelectedLog(null)}
          />
        )}

        {/* Pagination */}
        {totalPages > 1 && (
          <div className="flex items-center justify-between px-6 py-4 bg-slate-800/50 border-t border-slate-800">
            <div className="text-[11px] font-bold text-slate-400 uppercase tracking-wider">
              Page <span className="text-slate-100">{page + 1}</span> of <span className="text-slate-100">{totalPages}</span>
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
