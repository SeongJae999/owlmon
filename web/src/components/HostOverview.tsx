import React from 'react'
import { useQuery } from '@tanstack/react-query'
import type { ActiveAlert } from '../api/alert'
import { CheckCircle2, AlertCircle, AlertTriangle, Wrench, Monitor, ArrowRight, Activity, Zap, Cpu } from 'lucide-react'
import { cn } from '../utils/cn'
import { listHostSpecs, shortCPU, formatBytes, type HostSpec } from '../api/specs'

interface HostMetrics {
  cpu: number | null
  memory: number | null
  disk: number | null
}

interface Props {
  hosts: string[]
  hostStatuses: Record<string, 'online' | 'offline'>
  hostMetrics: Record<string, HostMetrics>
  activeAlerts: ActiveAlert[]
  uptimes: Record<string, number>
  maintenanceHosts: string[]
  onSelect: (host: string) => void
  onToggleMaintenance: (host: string, enabled: boolean) => void
}

function MetricBar({ value, warning, critical, label }: { value: number | null; warning: number; critical: number; label: string }) {
  const isEmpty = value === null

  const statusColor = isEmpty
    ? 'bg-slate-700'
    : value >= critical
      ? 'bg-rose-500'
      : value >= warning
        ? 'bg-amber-500'
        : 'bg-emerald-500'

  const textColor = isEmpty
    ? 'text-slate-600'
    : value >= critical
      ? 'text-rose-400'
      : value >= warning
        ? 'text-amber-400'
        : 'text-emerald-400'

  return (
    <div className="space-y-1">
      <div className="flex justify-between items-baseline">
        <span className="text-xs font-medium text-slate-500">{label}</span>
        <span className={cn("text-xs font-semibold tabular-nums", textColor)}>
          {isEmpty ? '—' : `${value.toFixed(0)}%`}
        </span>
      </div>
      <div className="h-1.5 w-full bg-slate-800 rounded-full overflow-hidden">
        <div
          className={cn("h-full rounded-full transition-all duration-300", statusColor)}
          style={{ width: isEmpty ? '0%' : `${Math.min(value, 100)}%` }}
        />
      </div>
    </div>
  )
}

function SummaryCard({ label, count, color, icon: Icon }: { label: string; count: number; color: string; icon: any }) {
  const themes: Record<string, string> = {
    blue:   'text-indigo-400 bg-indigo-500/10 border-indigo-500/20',
    green:  'text-emerald-400 bg-emerald-500/10 border-emerald-500/20',
    amber:  'text-amber-400 bg-amber-500/10 border-amber-500/20',
    rose:   'text-rose-400 bg-rose-500/10 border-rose-500/20',
    purple: 'text-purple-400 bg-purple-500/10 border-purple-500/20',
  }

  return (
    <div className={cn("flex items-center gap-3 p-4 rounded-xl border transition-colors", themes[color])}>
      <div className="p-2 rounded-lg bg-slate-900/40 border border-current/20">
        <Icon size={16} />
      </div>
      <div className="flex flex-col min-w-0">
        <span className="text-xs font-medium opacity-80 leading-none mb-1">{label}</span>
        <span className="text-xl font-bold tabular-nums leading-none">{count}</span>
      </div>
    </div>
  )
}

export default function HostOverview({ hosts, hostStatuses, hostMetrics, activeAlerts, uptimes, maintenanceHosts, onSelect, onToggleMaintenance }: Props) {
  const maintenanceSet = new Set(maintenanceHosts)

  // 모든 호스트 스펙 1회 fetch (5분 캐시 — 자주 안 변함)
  const { data: specs = [] } = useQuery({
    queryKey: ['host-specs'],
    queryFn: listHostSpecs,
    staleTime: 5 * 60 * 1000,
  })
  const specByHost = new Map<string, HostSpec>(specs.map((s) => [s.host_name, s]))

  const counts = hosts.reduce(
    (acc, host) => {
      if (maintenanceSet.has(host)) { acc.maintenance++; return acc }
      const offline = hostStatuses[host] === 'offline'
      const hasCritical = activeAlerts.some(a => a.host === host && a.severity === 'critical')
      const hasWarning = activeAlerts.some(a => a.host === host && a.severity === 'warning')
      if (offline || hasCritical) acc.fault++
      else if (hasWarning) acc.warning++
      else acc.ok++
      return acc
    },
    { ok: 0, warning: 0, fault: 0, maintenance: 0 },
  )

  return (
    <div className="space-y-6">
      {/* Overview Statistics */}
      <div className="grid grid-cols-2 md:grid-cols-5 gap-3">
        <SummaryCard label="전체 호스트" count={hosts.length} color="blue" icon={Monitor} />
        <SummaryCard label="정상" count={counts.ok} color="green" icon={CheckCircle2} />
        <SummaryCard label="경고" count={counts.warning} color="amber" icon={AlertTriangle} />
        <SummaryCard label="장애" count={counts.fault} color="rose" icon={AlertCircle} />
        <SummaryCard label="점검 중" count={counts.maintenance} color="purple" icon={Wrench} />
      </div>

      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <div className="flex items-baseline gap-2">
            <h2 className="text-base font-bold text-slate-100">호스트 목록</h2>
            <span className="text-xs font-medium text-slate-500 tabular-nums">{hosts.length}개</span>
          </div>
          <div className="flex items-center gap-1.5 text-xs font-medium text-slate-500">
            <Zap size={12} className="text-amber-400" /> 자동 갱신
          </div>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 gap-3">
          {hosts.map((host) => {
            const status = hostStatuses[host]
            const metrics = hostMetrics[host]
            const alertCount = activeAlerts.filter(a => a.host === host && !a.acked).length
            const isOffline = status === 'offline'
            const uptime = uptimes[host]
            const inMaintenance = maintenanceSet.has(host)

            // 메트릭 임계 상태 — 알림이 아직 안 떴어도 시각적 경고
            // (CPU 70/90, MEM 80/95, DISK 85/90)
            const cpu = metrics?.cpu ?? 0
            const mem = metrics?.memory ?? 0
            const dsk = metrics?.disk ?? 0
            const hasCritical = cpu >= 90 || mem >= 95 || dsk >= 90
            const hasWarning  = !hasCritical && (cpu >= 70 || mem >= 80 || dsk >= 85)

            return (
              <div
                key={host}
                className={cn(
                  "group bg-slate-900 rounded-xl border p-4 transition-all duration-200 flex flex-col h-full cursor-pointer",
                  inMaintenance
                    ? "border-purple-500/30 bg-purple-500/5 hover:border-purple-500/60"
                    : (alertCount > 0 || hasCritical)
                      ? "border-rose-500/40 hover:border-rose-500/70 hover:bg-rose-500/[0.03]"
                      : hasWarning
                        ? "border-amber-500/40 hover:border-amber-500/70 hover:bg-amber-500/[0.03]"
                        : isOffline
                          ? "border-slate-800 bg-slate-900/60 hover:border-slate-700"
                          : "border-slate-800 hover:border-indigo-500/60 hover:bg-slate-900/80"
                )}
              >
                {/* Card Header */}
                <div className="flex items-start justify-between gap-2 mb-4">
                  <div
                    className="flex items-start gap-2 cursor-pointer flex-1 min-w-0"
                    onClick={() => onSelect(host)}
                    title={host}
                  >
                    <div className={cn(
                      "w-2 h-2 rounded-full shrink-0 mt-1.5",
                      inMaintenance ? "bg-purple-400"
                        : isOffline ? "bg-rose-500"
                        : (alertCount > 0 || hasCritical) ? "bg-rose-400"
                        : hasWarning ? "bg-amber-400"
                        : "bg-emerald-400"
                    )} />
                    <div className="min-w-0 flex-1">
                      <span className="font-semibold text-sm text-slate-100 group-hover:text-indigo-400 transition-colors block break-words leading-snug">{host}</span>
                      <span className="text-xs font-medium text-slate-500 mt-0.5 block">
                        {isOffline ? '연결 끊김'
                          : inMaintenance ? '점검 중'
                          : (alertCount > 0 || hasCritical) ? '경고 (위험)'
                          : hasWarning ? '주의'
                          : '정상 동작'}
                      </span>
                    </div>
                  </div>

                  {!inMaintenance && alertCount > 0 && (
                    <span className="px-1.5 py-0.5 rounded-md bg-rose-500/15 text-rose-400 border border-rose-500/30 text-xs font-bold flex items-center gap-1 shrink-0">
                      <AlertCircle size={11} /> {alertCount}
                    </span>
                  )}
                </div>

                {/* Hardware Mini Line (스펙 등록된 경우만) */}
                {(() => {
                  const spec = specByHost.get(host)
                  if (!spec) return null
                  return (
                    <div className="flex items-center gap-1.5 mb-3 px-1 text-xs text-slate-500 truncate" title={`${spec.cpu_model} · ${formatBytes(spec.memory_total_bytes)}`}>
                      <Cpu size={11} className="text-slate-600 shrink-0" />
                      <span className="font-medium tabular-nums">{spec.cpu_cores}코어</span>
                      <span className="text-slate-700">·</span>
                      <span className="truncate">{shortCPU(spec.cpu_model)}</span>
                      <span className="text-slate-700">·</span>
                      <span className="font-medium shrink-0">{formatBytes(spec.memory_total_bytes)}</span>
                    </div>
                  )
                })()}

                {/* Metrics Section — 항상 표시 (오프라인이면 placeholder) */}
                <div
                  className="space-y-2.5 mb-4 cursor-pointer flex-1"
                  onClick={() => onSelect(host)}
                >
                  <MetricBar label="CPU" value={metrics?.cpu ?? null} warning={70} critical={90} />
                  <MetricBar label="메모리" value={metrics?.memory ?? null} warning={80} critical={95} />
                  <MetricBar label="디스크" value={metrics?.disk ?? null} warning={85} critical={90} />
                </div>

                {/* Footer */}
                <div className="pt-3 border-t border-slate-800 flex items-center justify-between gap-2">
                  <div className="flex items-baseline gap-1.5 min-w-0">
                    <span className="text-xs font-medium text-slate-500 shrink-0">가동률</span>
                    {uptime !== undefined ? (
                      <span className={cn(
                        "text-sm font-semibold tabular-nums",
                        uptime >= 99 ? "text-emerald-400" : uptime >= 95 ? "text-amber-400" : "text-rose-400"
                      )}>
                        {uptime.toFixed(1)}%
                      </span>
                    ) : <span className="text-sm font-semibold text-slate-600">—</span>}
                  </div>

                  <div className="flex gap-1 shrink-0">
                    <button
                      onClick={() => onToggleMaintenance(host, !inMaintenance)}
                      className={cn(
                        "p-1.5 rounded-md transition-colors border",
                        inMaintenance
                          ? "bg-purple-500 text-white border-transparent"
                          : "bg-slate-800 text-slate-400 border-slate-700 hover:text-slate-200 hover:bg-slate-700"
                      )}
                      title={inMaintenance ? "점검 모드 해제" : "점검 모드 설정"}
                    >
                      <Wrench size={14} />
                    </button>
                    <button
                      onClick={() => onSelect(host)}
                      className="p-1.5 rounded-md bg-indigo-500 text-white hover:bg-indigo-600 transition-colors"
                      title="상세 보기"
                    >
                      <ArrowRight size={14} />
                    </button>
                  </div>
                </div>
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}
