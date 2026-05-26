import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend } from 'recharts'
import { TrendingUp, Activity, MemoryStick, HardDrive } from 'lucide-react'
import { fetchHosts, queryRangeMultiHost } from '../api/prometheus'
import { cn } from '../utils/cn'
import PageToolbar from '../components/PageToolbar'

/**
 * 자원 사용 추이 페이지
 * - CPU / 메모리 / 디스크 3가지 메트릭을 호스트별로 비교
 * - 시간 범위: 1일 / 7일 / 30일
 * - 영업 시 "시계열 모니터링" 시연 자료
 */

const RANGES = [
  { label: '24시간', minutes: 1440 },
  { label: '7일',    minutes: 7 * 1440 },
  { label: '30일',   minutes: 30 * 1440 },
] as const

const METRICS = [
  { key: 'cpu',    label: 'CPU 사용률',   expr: 'system_cpu_usage_percent',    icon: Activity,    color: 'sky' },
  { key: 'memory', label: '메모리 사용률', expr: 'system_memory_usage_percent', icon: MemoryStick, color: 'violet' },
  { key: 'disk',   label: '디스크 사용률', expr: 'system_disk_usage_percent',   icon: HardDrive,   color: 'amber' },
] as const

// 호스트별 라인 색깔 (4~6대 가정)
const HOST_COLORS = ['#60a5fa', '#a78bfa', '#f59e0b', '#34d399', '#f472b6', '#fb7185']

export default function TrendsPage() {
  const [rangeIdx, setRangeIdx] = useState(0) // 기본 24h

  const { data: hosts = [] } = useQuery({
    queryKey: ['hosts'],
    queryFn: fetchHosts,
    refetchInterval: 60_000,
  })

  return (
    <div className="space-y-6">
      <PageToolbar
        icon={TrendingUp}
        title="자원 사용 추이"
        description="호스트별 CPU / 메모리 / 디스크 시계열 비교 — 변화 시점 추적 + 사전 감지"
      >
        <div className="flex gap-1 bg-slate-800 rounded-lg p-1">
          {RANGES.map((r, i) => (
            <button
              key={r.label}
              onClick={() => setRangeIdx(i)}
              className={cn(
                "px-3 py-1.5 rounded text-xs font-bold transition-colors",
                rangeIdx === i ? "bg-indigo-600 text-white" : "text-slate-400 hover:text-slate-200"
              )}
            >
              {r.label}
            </button>
          ))}
        </div>
      </PageToolbar>

      {METRICS.map(metric => (
        <MetricTrendCard
          key={metric.key}
          metric={metric}
          hosts={hosts}
          minutes={RANGES[rangeIdx].minutes}
        />
      ))}
    </div>
  )
}

function MetricTrendCard({
  metric, hosts, minutes,
}: {
  metric: typeof METRICS[number]
  hosts: string[]
  minutes: number
}) {
  const { data = [], isLoading } = useQuery({
    queryKey: ['trend', metric.key, minutes, hosts.join(',')],
    queryFn: () => queryRangeMultiHost(metric.expr, hosts, minutes),
    refetchInterval: 60_000,
    enabled: hosts.length > 0,
  })

  // 호스트별 최신값 + 시간 윈도우 내 max
  const summary = useMemo(() => {
    return hosts.map(h => {
      const vals = data.map(d => d[h]).filter(v => typeof v === 'number')
      const latest = vals.length > 0 ? vals[vals.length - 1] : null
      const max = vals.length > 0 ? Math.max(...vals) : null
      const min = vals.length > 0 ? Math.min(...vals) : null
      const change = (vals.length >= 2 && vals[0] != null && vals[vals.length - 1] != null)
        ? (vals[vals.length - 1] as number) - (vals[0] as number)
        : null
      return { host: h, latest, max, min, change }
    })
  }, [data, hosts])

  const Icon = metric.icon
  const iconColor = {
    sky: 'text-sky-400 bg-sky-500/10',
    violet: 'text-violet-400 bg-violet-500/10',
    amber: 'text-amber-400 bg-amber-500/10',
  }[metric.color]

  return (
    <div className="bg-slate-900 rounded-2xl border border-slate-800 shadow-sm p-5">
      <div className="flex items-center gap-3 mb-4">
        <div className={cn("p-2 rounded-lg", iconColor)}>
          <Icon size={18} />
        </div>
        <div>
          <h3 className="text-sm font-bold text-slate-100">{metric.label}</h3>
          <p className="text-[11px] text-slate-500">호스트별 비교 (max by host_name)</p>
        </div>
      </div>

      {isLoading ? (
        <div className="h-64 flex items-center justify-center text-slate-500 text-sm">로딩 중...</div>
      ) : data.length === 0 ? (
        <div className="h-64 flex items-center justify-center text-slate-500 text-sm">데이터 없음</div>
      ) : (
        <>
          <div className="h-64 -mx-2">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={data} margin={{ top: 5, right: 15, left: -10, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="#1e293b" />
                <XAxis dataKey="time" stroke="#64748b" fontSize={10} interval="preserveStartEnd" minTickGap={50} />
                <YAxis stroke="#64748b" fontSize={10} domain={[0, 100]} unit="%" />
                <Tooltip
                  contentStyle={{ background: '#0f172a', border: '1px solid #334155', borderRadius: '8px', fontSize: '12px' }}
                  labelStyle={{ color: '#cbd5e1' }}
                />
                <Legend wrapperStyle={{ fontSize: '11px', paddingTop: '4px' }} />
                {hosts.map((h, i) => (
                  <Line
                    key={h}
                    type="monotone"
                    dataKey={h}
                    stroke={HOST_COLORS[i % HOST_COLORS.length]}
                    strokeWidth={2}
                    dot={false}
                    isAnimationActive={false}
                    connectNulls
                  />
                ))}
              </LineChart>
            </ResponsiveContainer>
          </div>

          {/* 호스트별 요약 */}
          <div className="mt-4 grid grid-cols-2 md:grid-cols-4 gap-2">
            {summary.map((s, i) => (
              <div key={s.host} className="bg-slate-800/50 rounded-lg p-2.5 border border-slate-800">
                <div className="flex items-center gap-1.5 mb-1.5">
                  <div className="w-2 h-2 rounded-full" style={{ background: HOST_COLORS[i % HOST_COLORS.length] }} />
                  <span className="text-[11px] font-bold text-slate-300 truncate">{s.host}</span>
                </div>
                <div className="grid grid-cols-3 gap-1 text-[10px] text-slate-400">
                  <div title="최신">
                    <div className="opacity-60">현재</div>
                    <div className="font-bold text-slate-200 tabular-nums">{s.latest != null ? `${s.latest.toFixed(1)}%` : '—'}</div>
                  </div>
                  <div title="기간 내 최대">
                    <div className="opacity-60">최대</div>
                    <div className="font-bold text-amber-300 tabular-nums">{s.max != null ? `${s.max.toFixed(1)}%` : '—'}</div>
                  </div>
                  <div title="시작 → 끝 변화">
                    <div className="opacity-60">변화</div>
                    <div className={cn(
                      "font-bold tabular-nums",
                      s.change == null ? "text-slate-500"
                        : s.change > 5 ? "text-rose-300"
                        : s.change < -5 ? "text-emerald-300"
                        : "text-slate-200"
                    )}>
                      {s.change != null ? `${s.change >= 0 ? '+' : ''}${s.change.toFixed(1)}%p` : '—'}
                    </div>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  )
}
