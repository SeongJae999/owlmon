import React from 'react'
import { useSearchParams, Link } from 'react-router-dom'
import { 
  useHosts, 
  useAllHostStatuses, 
  useHostMetrics, 
  useServiceChecks, 
  useAlertStatus, 
  useAnomalyData 
} from '../hooks/useMonitoring'
import axios from 'axios'
import { getAlertConfig } from '../api/alert'
import { useQuery } from '@tanstack/react-query'
import { queryRange } from '../api/prometheus'
import MetricCard from '../components/MetricCard'
import ServiceCheckCard from '../components/ServiceCheckCard'
import HostSpecCard from '../components/HostSpecCard'
import { getHostSpec } from '../api/specs'
import { ChevronLeft, Server, Activity, ShieldAlert, Zap, ArrowLeft } from 'lucide-react'
import { cn } from '../utils/cn'

export default function HostDetailPage() {
  const [searchParams] = useSearchParams()
  const hostName = searchParams.get('host') || ''

  const { data: hosts = [] } = useHosts()
  const { data: hostStatuses = {} } = useAllHostStatuses(hosts)
  const { data: metrics } = useHostMetrics(hostName)
  const { data: serviceChecks = [] } = useServiceChecks(hostName)
  const { data: activeAlerts = [] } = useAlertStatus()
  const { data: anomalyData } = useAnomalyData()
  const { data: alertCfg } = useQuery({ queryKey: ['alertConfig'], queryFn: getAlertConfig })

  // Chart Data Queries
  const { data: cpuChart = [] } = useQuery({
    queryKey: ['chart', 'cpu', hostName],
    queryFn: () => queryRange(`system_cpu_usage_percent{host_name="${hostName}"}`),
    enabled: !!hostName,
    refetchInterval: 30000
  })
  const { data: memChart = [] } = useQuery({
    queryKey: ['chart', 'memory', hostName],
    queryFn: () => queryRange(`system_memory_usage_percent{host_name="${hostName}"}`),
    enabled: !!hostName,
    refetchInterval: 30000
  })
  const { data: diskChart = [] } = useQuery({
    queryKey: ['chart', 'disk', hostName],
    queryFn: () => queryRange(`max(system_disk_usage_percent{host_name="${hostName}"})`),
    enabled: !!hostName,
    refetchInterval: 30000
  })

  // 호스트 스펙 (메모리 총량 등)
  const { data: hostSpec } = useQuery({
    queryKey: ['host-spec', hostName],
    queryFn: () => getHostSpec(hostName),
    enabled: !!hostName,
    staleTime: 5 * 60 * 1000,
  })

  // 절대값 (메모리/디스크 GB 표시용) — Prometheus instant query
  // axios 사용해야 JWT 인터셉터로 인증 헤더 자동 첨부 (fetch는 수동 첨부 필요)
  const promQuery = async (q: string): Promise<number | null> => {
    const res = await axios.get('/api/v1/query', { params: { query: q } })
    const v = res.data?.data?.result?.[0]?.value?.[1]
    return v ? parseFloat(v) : null
  }
  const { data: memUsedBytes } = useQuery({
    queryKey: ['absolute', 'mem-used', hostName],
    queryFn: () => promQuery(`system_memory_used_bytes{host_name="${hostName}"}`),
    enabled: !!hostName,
    refetchInterval: 30000,
  })
  const { data: diskAbsolute } = useQuery({
    queryKey: ['absolute', 'disk', hostName],
    queryFn: async () => {
      const [used, total] = await Promise.all([
        promQuery(`sum(system_disk_used_bytes{host_name="${hostName}"})`),
        promQuery(`sum(system_disk_total_bytes{host_name="${hostName}"})`),
      ])
      return { used, total }
    },
    enabled: !!hostName,
    refetchInterval: 30000,
  })

  if (!hostName) {
    return (
      <div className="flex flex-col items-center justify-center h-[60vh] text-slate-400 space-y-4">
        <Server size={64} className="mb-2 opacity-10" />
        <p className="font-bold">호스트가 선택되지 않았습니다.</p>
        <Link to="/" className="flex items-center gap-2 text-indigo-600 font-bold text-xs uppercase tracking-widest hover:gap-3 transition-all">
          <ArrowLeft size={16} /> Dashboard
        </Link>
      </div>
    )
  }

  const isOffline = hostStatuses[hostName] === 'offline'
  const hostAnomalies = anomalyData?.anomalies.filter(a => a.host === hostName) || []

  return (
    <div className="space-y-8">
      {/* Top Navigation & Status */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-slate-800 pb-6">
        <div className="flex items-center gap-4 min-w-0">
          <Link to="/" className="p-2 bg-slate-900 hover:bg-slate-800 rounded-lg border border-slate-800 transition-colors text-slate-400 shrink-0" title="목록으로">
            <ChevronLeft size={18} />
          </Link>
          <div className="min-w-0">
            <div className="flex items-center gap-2.5">
              <h1 className="text-xl font-bold text-slate-100 truncate" title={hostName}>{hostName}</h1>
              <span className={cn(
                "w-2.5 h-2.5 rounded-full shrink-0",
                isOffline ? "bg-rose-500" : "bg-emerald-400"
              )} />
              <span className="text-xs font-medium text-slate-400 shrink-0">{isOffline ? '연결 끊김' : '정상 동작'}</span>
            </div>
            {/* 호스트 스펙 인라인 (CPU/RAM 한 줄) — 별도 카드 대신 헤더에 압축 */}
            {hostSpec ? (
              <p className="text-xs text-slate-500 mt-0.5">
                {hostSpec.cpu_cores}코어
                {hostSpec.cpu_model && <> · {hostSpec.cpu_model.replace(/\s+CPU\s+/i, ' ').replace(/@.*$/, '').trim()}</>}
                {hostSpec.memory_total_bytes && <> · RAM {(hostSpec.memory_total_bytes / 1024 / 1024 / 1024).toFixed(0)}GB</>}
              </p>
            ) : (
              <p className="text-xs text-slate-500 mt-0.5">호스트 상세</p>
            )}
          </div>
        </div>

        <div className="flex items-center gap-4">
          <div className="flex flex-col items-end">
            <span className="text-xs font-medium text-slate-500">활성 알림</span>
            <span className={cn(
              "text-base font-bold tabular-nums",
              hostAnomalies.length > 0 ? "text-rose-400" : "text-slate-500"
            )}>{hostAnomalies.length}</span>
          </div>
          <div className="w-px h-6 bg-slate-800" />
          <div className="flex flex-col items-end">
            <span className="text-xs font-medium text-slate-500">갱신 주기</span>
            <span className="text-base font-bold text-emerald-400 tabular-nums">30초</span>
          </div>
        </div>
      </div>

      {/* Offline Alert */}
      {isOffline && (
        <div className="bg-rose-500/10 border border-rose-500/30 rounded-xl p-4 flex items-center gap-3 text-rose-300">
          <ShieldAlert className="text-rose-400 shrink-0" size={20} />
          <div>
            <p className="font-semibold text-sm">에이전트 연결 끊김</p>
            <p className="text-xs font-medium opacity-80 mt-0.5">에이전트로부터 응답이 없습니다. 마지막으로 수집된 캐시 데이터를 표시합니다.</p>
          </div>
        </div>
      )}

      {/* Metric Cards Grid */}
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            <div className="w-1 h-5 bg-indigo-500 rounded-full" />
            <h2 className="text-base font-bold text-slate-100">리소스 사용률</h2>
            <span className="text-[11px] font-semibold text-slate-500">24시간 추이</span>
          </div>
          <Link
            to="/trends"
            className="inline-flex items-center gap-1 text-[11px] font-semibold text-indigo-400 hover:text-indigo-300 transition-colors"
          >
            전체 호스트 비교 / 7일·30일 추이 →
          </Link>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8">
          <MetricCard
            title="CPU"
            value={metrics?.cpu ?? null}
            data={cpuChart}
            color="#4f46e5"
            warning={alertCfg ? alertCfg.cpu_threshold * 0.8 : 70}
            critical={alertCfg?.cpu_threshold ?? 90}
            anomaly={hostAnomalies.find(a => a.metric === 'cpu')}
          />
          <MetricCard
            title="메모리"
            value={metrics?.memory ?? null}
            data={memChart}
            color="#8b5cf6"
            warning={alertCfg ? alertCfg.mem_threshold * 0.85 : 80}
            critical={alertCfg?.mem_threshold ?? 95}
            anomaly={hostAnomalies.find(a => a.metric === 'memory')}
            usedBytes={memUsedBytes}
            totalBytes={hostSpec?.memory_total_bytes}
          />
          <MetricCard
            title="디스크"
            value={metrics?.disk ?? null}
            data={diskChart}
            color="#10b981"
            warning={alertCfg?.disk_warn ?? 85}
            critical={alertCfg?.disk_crit ?? 90}
            anomaly={hostAnomalies.find(a => a.metric === 'disk')}
            diskPrediction={anomalyData?.disk_predictions.find(p => p.host === hostName)}
            usedBytes={diskAbsolute?.used}
            totalBytes={diskAbsolute?.total}
            absoluteHint="agent 업데이트 후 표시"
          />
        </div>
      </div>

      {/* 보조 정보: 호스트 스펙 + 서비스 체크 (collapsible — 디폴트 접힘) */}
      <details className="group bg-slate-900 rounded-2xl border border-slate-800 overflow-hidden">
        <summary className="flex items-center justify-between px-5 py-3 cursor-pointer hover:bg-slate-800/40 transition-colors list-none">
          <div className="flex items-center gap-2.5">
            <div className="w-1 h-5 bg-violet-500 rounded-full" />
            <h2 className="text-sm font-bold text-slate-200">상세 정보</h2>
            <span className="text-[11px] font-medium text-slate-500">
              호스트 스펙 + 서비스 체크 {serviceChecks.length > 0 && `(${serviceChecks.length})`}
            </span>
          </div>
          <span className="text-xs text-slate-500 group-open:rotate-180 transition-transform">▼</span>
        </summary>
        <div className="px-5 pb-5 space-y-4">
          <HostSpecCard host={hostName} />
          {serviceChecks.length > 0 && (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
              {serviceChecks.map((check) => (
                <ServiceCheckCard
                  key={check.name}
                  name={check.name}
                  type={check.type}
                  target={check.target}
                  status={check.status}
                  latencyMs={check.latencyMs}
                />
              ))}
            </div>
          )}
        </div>
      </details>
    </div>
  )
}
