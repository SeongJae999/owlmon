import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { listRules, getRuleStatsDetailed, severityLabel, categoryLabel } from '../api/rules'
import { BarChart3, AlertTriangle, CheckCircle2, Clock, TrendingUp, Filter } from 'lucide-react'
import { cn } from '../utils/cn'
import SummaryCard from '../components/SummaryCard'
import PageToolbar from '../components/PageToolbar'

/**
 * 룰 매칭 통계 페이지
 * - 룰별 1h/24h/7d/30d 매칭 카운트
 * - 알림 발사 카운트 (전 기간)
 * - 마지막 매칭 시각
 * - 가장 시끄러운 룰 / 매칭 0건 룰 식별
 */
// 룰 통계 콘텐츠 — RulesPage 탭에서도 재사용
export function RuleStatsContent() {
  const { data: rules = [] } = useQuery({ queryKey: ['log-rules-all'], queryFn: listRules })
  const { data: stats = {} } = useQuery({
    queryKey: ['log-rules-stats-detailed'],
    queryFn: getRuleStatsDetailed,
    refetchInterval: 30_000,
  })

  // 룰 + 통계 결합
  const rows = useMemo(() => {
    return rules.map(r => ({
      ...r,
      stat: stats[r.id] || {
        rule_id: r.id, matches_1h: 0, matches_24h: 0, matches_7d: 0, matches_30d: 0,
        alerts_fired: 0,
      },
    })).sort((a, b) => b.stat.matches_30d - a.stat.matches_30d) // 시끄러운 순
  }, [rules, stats])

  // 요약 통계
  const summary = useMemo(() => {
    const total = rows.length
    const matched = rows.filter(r => r.stat.matches_30d > 0).length
    const silent = total - matched
    const totalMatches30d = rows.reduce((s, r) => s + r.stat.matches_30d, 0)
    const totalAlerts = rows.reduce((s, r) => s + r.stat.alerts_fired, 0)
    return { total, matched, silent, totalMatches30d, totalAlerts }
  }, [rows])

  return (
    <div className="space-y-6">
      {/* Summary Cards */}
      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-5 gap-3">
        <SummaryCard icon={Filter}        label="전체 룰"       value={summary.total} color="indigo" />
        <SummaryCard icon={CheckCircle2}  label="매칭된 룰"     value={summary.matched} color="emerald" suffix={`/ ${summary.total}`} />
        <SummaryCard icon={AlertTriangle} label="매칭 0건"      value={summary.silent} color="slate" />
        <SummaryCard icon={TrendingUp}    label="총 매칭 (30d)" value={summary.totalMatches30d.toLocaleString()} color="amber" />
        <SummaryCard icon={AlertTriangle} label="총 알림 발사"   value={summary.totalAlerts.toLocaleString()} color="rose" />
      </div>

      {/* Rules table */}
      <div className="bg-slate-900 rounded-2xl border border-slate-800 shadow-sm overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="bg-slate-800/50 border-b border-slate-800">
              <tr>
                <th className="px-3 py-2.5 text-[11px] font-bold text-slate-400 uppercase tracking-wider">룰 이름</th>
                <th className="px-3 py-2.5 text-[11px] font-bold text-slate-400 uppercase tracking-wider">카테고리</th>
                <th className="px-3 py-2.5 text-[11px] font-bold text-slate-400 uppercase tracking-wider">심각도</th>
                <th className="px-3 py-2.5 text-[11px] font-bold text-slate-400 uppercase tracking-wider text-right tabular-nums">1h</th>
                <th className="px-3 py-2.5 text-[11px] font-bold text-slate-400 uppercase tracking-wider text-right tabular-nums">24h</th>
                <th className="px-3 py-2.5 text-[11px] font-bold text-slate-400 uppercase tracking-wider text-right tabular-nums">7d</th>
                <th className="px-3 py-2.5 text-[11px] font-bold text-slate-400 uppercase tracking-wider text-right tabular-nums">30d</th>
                <th className="px-3 py-2.5 text-[11px] font-bold text-slate-400 uppercase tracking-wider text-right tabular-nums">알림 발사</th>
                <th className="px-3 py-2.5 text-[11px] font-bold text-slate-400 uppercase tracking-wider">마지막 매칭</th>
                <th className="px-3 py-2.5 text-[11px] font-bold text-slate-400 uppercase tracking-wider">상태</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800">
              {rows.map(r => {
                const isSilent = r.stat.matches_30d === 0
                const isNoisy = r.stat.matches_30d > 50
                const matchEfficiency = r.stat.matches_30d > 0
                  ? (r.stat.alerts_fired / r.stat.matches_30d * 100).toFixed(0)
                  : null

                return (
                  <tr key={r.id} className="hover:bg-slate-800/30 transition-colors">
                    <td className="px-3 py-2">
                      <div className="font-semibold text-slate-100 text-sm">{r.name}</div>
                      <code className="text-[10px] text-slate-500 font-mono">{r.pattern}</code>
                    </td>
                    <td className="px-3 py-2 text-xs text-slate-400">{categoryLabel(r.category)}</td>
                    <td className="px-3 py-2">
                      <span className={cn(
                        "px-1.5 py-0.5 rounded text-[10px] font-bold uppercase",
                        r.severity === 'critical' ? 'bg-rose-500/15 text-rose-300'
                          : r.severity === 'warning' ? 'bg-amber-500/15 text-amber-300'
                          : 'bg-blue-500/15 text-blue-300'
                      )}>
                        {severityLabel(r.severity)}
                      </span>
                    </td>
                    <td className="px-3 py-2 text-right text-xs font-mono tabular-nums text-slate-300">
                      {r.stat.matches_1h > 0 ? r.stat.matches_1h : <span className="text-slate-600">·</span>}
                    </td>
                    <td className="px-3 py-2 text-right text-xs font-mono tabular-nums text-slate-300">
                      {r.stat.matches_24h > 0 ? r.stat.matches_24h : <span className="text-slate-600">·</span>}
                    </td>
                    <td className="px-3 py-2 text-right text-xs font-mono tabular-nums text-slate-300">
                      {r.stat.matches_7d > 0 ? r.stat.matches_7d : <span className="text-slate-600">·</span>}
                    </td>
                    <td className={cn(
                      "px-3 py-2 text-right text-xs font-bold tabular-nums",
                      isNoisy ? "text-amber-300" : "text-slate-200"
                    )}>
                      {r.stat.matches_30d > 0 ? r.stat.matches_30d.toLocaleString() : <span className="text-slate-600 font-normal">·</span>}
                    </td>
                    <td className="px-3 py-2 text-right text-xs font-mono tabular-nums text-slate-300">
                      {r.stat.alerts_fired > 0 ? (
                        <span title={`매칭 ${r.stat.matches_30d}건 중 ${r.stat.alerts_fired}건 알림 (${matchEfficiency}%)`}>
                          {r.stat.alerts_fired}
                        </span>
                      ) : <span className="text-slate-600">·</span>}
                    </td>
                    <td className="px-3 py-2 text-[11px] text-slate-400 font-mono whitespace-nowrap">
                      {r.stat.last_match_at
                        ? <span title={new Date(r.stat.last_match_at).toLocaleString('ko-KR')}>{relativeTime(r.stat.last_match_at)}</span>
                        : <span className="text-slate-600">—</span>}
                    </td>
                    <td className="px-3 py-2">
                      {!r.enabled ? (
                        <span className="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-slate-700 text-slate-400">비활성</span>
                      ) : isSilent ? (
                        <span className="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-slate-700/50 text-slate-500" title="30일간 매칭 없음 — 시스템 조용(정상) 또는 패턴 검토">
                          조용
                        </span>
                      ) : isNoisy ? (
                        <span className="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-amber-500/15 text-amber-300" title="30일간 50회 이상 — 패턴이 너무 광범위하지 않은지 점검">
                          시끄러움
                        </span>
                      ) : (
                        <span className="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-emerald-500/15 text-emerald-300">
                          정상
                        </span>
                      )}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
        <div className="px-4 py-3 bg-slate-800/30 border-t border-slate-800 flex items-center justify-between text-[11px] text-slate-500">
          <div className="flex items-center gap-3">
            <span>총 {rows.length}개 룰</span>
            <span className="text-slate-600">·</span>
            <span>30초 자동 갱신</span>
          </div>
          <div className="flex items-center gap-3">
            <span className="flex items-center gap-1"><span className="w-2 h-2 rounded-full bg-amber-400" /> 시끄러움 (30d 50회+)</span>
            <span className="flex items-center gap-1"><span className="w-2 h-2 rounded-full bg-slate-500" /> 조용 (한 번도 매칭 X)</span>
          </div>
        </div>
      </div>
    </div>
  )
}

// Legacy 페이지 wrapper — /rules/stats URL 호환
export default function RuleStatsPage() {
  return (
    <div className="space-y-6">
      <PageToolbar
        icon={BarChart3}
        title="룰 매칭 통계"
        description="어떤 룰이 시끄러운가, 어떤 룰이 한 번도 안 잡혔는가 — 운영자가 룰을 다듬는 도구"
      />
      <RuleStatsContent />
    </div>
  )
}


function relativeTime(iso: string): string {
  const ts = new Date(iso).getTime()
  const diff = Date.now() - ts
  const sec = Math.floor(diff / 1000)
  if (sec < 60)    return `${sec}초 전`
  const min = Math.floor(sec / 60)
  if (min < 60)    return `${min}분 전`
  const hr = Math.floor(min / 60)
  if (hr < 24)     return `${hr}시간 전`
  const day = Math.floor(hr / 24)
  return `${day}일 전`
}
