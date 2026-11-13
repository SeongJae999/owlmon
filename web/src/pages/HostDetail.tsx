import { useSearchParams, Link } from 'react-router-dom'
import {
  useHosts,
  useAllHostStatuses,
  useHostMetrics,
  useServiceChecks,
  useAnomalyData,
  useAlertStatus,
  useMaintenanceHosts
} from '../hooks/useMonitoring'
import { serverState, LABEL } from '../hooks/useFleet'
import ActiveAlertsPanel from '../components/ActiveAlertsPanel'
import axios from 'axios'
import { getAlertConfig, thresholds } from '../api/alert'
import { useQuery } from '@tanstack/react-query'
import { queryRange } from '../api/prometheus'
import MetricCard from '../components/MetricCard'
import ServiceCheckCard from '../components/ServiceCheckCard'
import HostSpecCard from '../components/HostSpecCard'
import DiskUsageBreakdown from '../components/DiskUsageBreakdown'
import DiskTopology from '../components/DiskTopology'
import { getHostSpec } from '../api/specs'
import { fetchAssets } from '../api/asset'
import { ChevronLeft, Server, ShieldAlert, ArrowLeft } from 'lucide-react'
import { cn } from '../utils/cn'

export default function HostDetailPage() {
  const [searchParams] = useSearchParams()
  const hostName = searchParams.get('host') || ''

  const { data: hosts = [], isError: hostsError, isSuccess: hostsReady } = useHosts()
  const { data: hostStatuses = {}, isError: statusError, dataUpdatedAt: statusAt } = useAllHostStatuses(hosts)
  const { data: metrics } = useHostMetrics(hostName)
  const { data: serviceChecks = [] } = useServiceChecks(hostName)
  const { data: anomalyData } = useAnomalyData()
  const { data: alertCfg } = useQuery({ queryKey: ['alertConfig'], queryFn: getAlertConfig })
  const th = thresholds(alertCfg)
  const { data: activeAlerts = [], refetch: refetchAlerts, isError: alertsError, isPending: alertsPending } = useAlertStatus()
  const { data: maintenanceHosts = [] } = useMaintenanceHosts()
  const { data: assets = [] } = useQuery({ queryKey: ['assets'], queryFn: fetchAssets, staleTime: 5 * 60_000 })
  const location = assets.find(a => a.host_name === hostName)?.location.trim()

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
  // 디스크 절대 용량/잔여는 "디스크 사용량"(DiskUsageBreakdown)이 마운트별로 단독 표시.
  // 여기서 합산 절대값을 또 보여주면 정보가 중복돼 제거함.

  if (!hostName) {
    return (
      <div className="flex flex-col items-center justify-center h-[60vh] text-base-400 space-y-4">
        <Server size={64} className="mb-2 opacity-10" />
        <p className="font-bold">호스트가 선택되지 않았습니다.</p>
        <Link to="/" className="flex items-center gap-2 text-brand-600 font-bold text-xs hover:gap-3 transition">
          <ArrowLeft size={16} /> Dashboard
        </Link>
      </div>
    )
  }

  const isOffline = hostStatuses[hostName] === 'offline'
  // 상태를 모르면(조회 실패·아직 응답 전) 초록 "정상"으로 보이면 안 됨 — 관제에서 가장 위험한 오표시
  // 이전 결과가 남아 있어도 최근 조회가 실패했으면 "정상"보다 실패를 먼저 보여 줌
  // 메인·상황판과 같은 판정(useFleet.serverState) — 알림이 있으면 "정상 동작"이 아니라 위험·주의
  const state = serverState(hostStatuses[hostName], activeAlerts.filter(a => a.host === hostName && !a.in_maintenance), maintenanceHosts.includes(hostName))
  const DOT = { crit: 'bg-rose-500', warn: 'bg-amber-400', maint: 'bg-purple-400', ok: 'bg-emerald-400', wait: 'bg-base-500' }
  const status = isOffline ? { dot: 'bg-rose-500', label: '연결 끊김' }
    : statusError || hostsError || alertsError ? { dot: 'bg-amber-400', label: '상태 조회 실패' }
    : alertsPending ? { dot: 'bg-base-500', label: '확인 중' }
    : hostStatuses[hostName] === 'online' ? { dot: DOT[state], label: state === 'ok' ? '정상 동작' : LABEL[state] }
    : hostsReady && !hosts.includes(hostName) ? { dot: 'bg-base-500', label: '수집 이력 없음' }
    : { dot: 'bg-base-500', label: '확인 중' }
  const hostAnomalies = anomalyData?.anomalies.filter(a => a.host === hostName) || []

  return (
    <div className="space-y-5 sm:space-y-8">
      {/* Top Navigation & Status */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-base-800 pb-6">
        <div className="flex items-center gap-4 min-w-0">
          <Link to="/" className="p-2 bg-base-900 hover:bg-base-800 rounded-lg border border-base-800 transition-colors text-base-400 shrink-0" title="목록으로">
            <ChevronLeft size={18} />
          </Link>
          <div className="min-w-0">
            <div className="flex items-center gap-2.5">
              <h1 className="text-xl font-bold text-base-100 truncate" title={hostName}>{hostName}</h1>
              <span className={cn("w-2.5 h-2.5 rounded-full shrink-0", status.dot)} />
              <span className="text-xs font-medium text-base-400 shrink-0">{status.label}</span>
            </div>
            {location && <p className="text-sm text-base-300 mt-0.5 truncate" title={location}>위치 {location}</p>}
            {/* 호스트 스펙 인라인 (CPU/RAM 한 줄) — 별도 카드 대신 헤더에 압축 */}
            {hostSpec ? (
              <p className="text-xs text-base-500 mt-0.5">
                {hostSpec.cpu_cores}코어
                {hostSpec.cpu_model && <> · {hostSpec.cpu_model.replace(/\s+CPU\s+/i, ' ').replace(/@.*$/, '').trim()}</>}
                {hostSpec.memory_total_bytes && <> · RAM {(hostSpec.memory_total_bytes / 1024 / 1024 / 1024).toFixed(0)}GB</>}
              </p>
            ) : (
              <p className="text-xs text-base-500 mt-0.5">호스트 상세</p>
            )}
          </div>
        </div>

        <div className="flex items-center gap-4">
          <div className="flex flex-col items-end">
            <span className="text-xs font-medium text-base-500">이상 탐지</span>
            <span className={cn(
              "text-base font-bold tabular-nums",
              hostAnomalies.length > 0 ? "text-rose-400" : "text-base-500"
            )}>{hostAnomalies.length}</span>
          </div>
          <div className="w-px h-6 bg-base-800" />
          <div className="flex flex-col items-end">
            <span className="text-xs font-medium text-base-500">상태 갱신</span>
            <span className="text-base font-bold text-base-300 tabular-nums">
              {statusAt ? new Date(statusAt).toLocaleTimeString('ko-KR', { hour: '2-digit', minute: '2-digit', second: '2-digit' }) : '—'}
            </span>
          </div>
        </div>
      </div>

      {/* Offline Alert */}
      {isOffline && (
        <div className="bg-rose-500/10 border border-rose-500/30 rounded-xl p-4 flex items-center gap-3 text-rose-300">
          <ShieldAlert className="text-rose-400 shrink-0" size={20} />
          <div>
            <p className="font-semibold text-sm">에이전트 연결 끊김</p>
            <p className="text-xs font-medium opacity-80 mt-0.5">에이전트로부터 2분 넘게 데이터가 오지 않습니다. 아래 차트는 끊기기 전까지의 기록입니다.</p>
          </div>
        </div>
      )}

      {hostName && (
        <ActiveAlertsPanel alerts={activeAlerts} onChanged={refetchAlerts} host={hostName} error={alertsError} loading={alertsPending} />
      )}

      {/* Metric Cards Grid */}
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            <div className="w-1 h-5 bg-brand-500 rounded-full" />
            <h2 className="text-base font-bold text-base-100">리소스 사용률</h2>
            <span className="text-xs font-semibold text-base-500">24시간 추이</span>
          </div>
          <Link
            to="/trends"
            className="inline-flex items-center gap-1 text-xs font-semibold text-brand-400 hover:text-brand-300 transition-colors"
          >
            전체 호스트 7일, 30일 추이 비교
          </Link>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3 sm:gap-5 lg:gap-6">
          <MetricCard
            title="CPU"
            value={metrics?.cpu ?? null}
            data={cpuChart}
            color="var(--color-brand-600)"
            warning={th.cpuWarn}
            critical={th.cpuCrit}
            anomaly={hostAnomalies.find(a => a.metric === 'cpu')}
          />
          <MetricCard
            title="메모리"
            value={metrics?.memory ?? null}
            data={memChart}
            color="var(--color-violet-500)"
            warning={th.memWarn}
            critical={th.memCrit}
            anomaly={hostAnomalies.find(a => a.metric === 'memory')}
            usedBytes={memUsedBytes}
            totalBytes={hostSpec?.memory_total_bytes}
          />
          {/* 디스크 카드 = 추이 + 예측 전담. 값은 max(마운트별 사용률) = 최고 사용률.
              절대 용량/잔여는 아래 "디스크 사용량" 섹션이 단독 표시 (중복 제거) */}
          <MetricCard
            title="디스크"
            value={metrics?.disk ?? null}
            data={diskChart}
            color="var(--color-emerald-500)"
            warning={th.diskWarn}
            critical={th.diskCrit}
            anomaly={hostAnomalies.find(a => a.metric === 'disk')}
            diskPrediction={anomalyData?.disk_predictions.find(p => p.host === hostName)}
            subtitle="최고 사용률 · 마운트별 상세 아래 ↓"
          />
        </div>
      </div>

      {/* 디스크별 사용량 (마운트포인트별 잔여 용량) */}
      <div className="space-y-4">
        <div className="flex items-center gap-2.5">
          <div className="w-1 h-5 bg-emerald-500 rounded-full" />
          <h2 className="text-base font-bold text-base-100">디스크 사용량</h2>
          <span className="text-xs font-semibold text-base-500">마운트별 잔여 용량</span>
        </div>
        <DiskUsageBreakdown
          host={hostName}
          warnThreshold={th.diskWarn}
          critThreshold={th.diskCrit}
        />
      </div>

      {/* 물리 디스크 구조 (디스크→파티션→LVM→마운트 트리) */}
      <div className="space-y-4">
        <div className="flex items-center gap-2.5">
          <div className="w-1 h-5 bg-sky-500 rounded-full" />
          <h2 className="text-base font-bold text-base-100">물리 디스크 구조</h2>
          <span className="text-xs font-semibold text-base-500">파티션 · LVM 분해</span>
        </div>
        <DiskTopology host={hostName} />
      </div>

      {/* 장비 정보 */}
      <div className="space-y-4">
        <div className="flex items-center gap-2.5">
          <div className="w-1 h-5 bg-violet-500 rounded-full" />
          <h2 className="text-base font-bold text-base-100">장비 정보</h2>
        </div>
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
    </div>
  )
}
