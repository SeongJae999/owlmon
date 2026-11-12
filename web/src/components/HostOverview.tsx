import { useQuery } from '@tanstack/react-query'
import { AlertCircle, AlertTriangle, Wrench, Monitor, ArrowRight, Cpu } from 'lucide-react'
import { cn } from '../utils/cn'
import { listHostSpecs, shortCPU, formatBytes, type HostSpec } from '../api/specs'
import { hostLabel } from '../config/hostLabels'
import { LABEL, type State, type Tile } from '../hooks/useFleet'

interface Props {
  tiles: Tile[] // 서버 그룹 — 상태 판정은 useFleet에서 끝난 값만 표시
  failed: boolean // 서버 상태 조회 실패
  uptimes: Record<string, number>
  onSelect: (host: string) => void
  onToggleMaintenance: (host: string, enabled: boolean) => void
}

const DOT: Record<State, string> = { crit: 'bg-rose-500', warn: 'bg-amber-400', maint: 'bg-purple-400', ok: 'bg-emerald-400', wait: 'bg-base-500' }

function MetricBar({ value, warning, critical, label }: { value: number | null; warning: number; critical: number; label: string }) {
  const isEmpty = value === null

  const statusColor = isEmpty
    ? 'bg-base-700'
    : value >= critical
      ? 'bg-rose-500'
      : value >= warning
        ? 'bg-amber-500'
        : 'bg-emerald-500'

  const textColor = isEmpty
    ? 'text-base-600'
    : value >= critical
      ? 'text-rose-400'
      : value >= warning
        ? 'text-amber-400'
        : 'text-emerald-400'

  return (
    <div className="space-y-1">
      <div className="flex justify-between items-baseline">
        <span className="text-xs font-medium text-base-500">{label}</span>
        <span className={cn("text-xs font-semibold tabular-nums", textColor)}>
          {isEmpty ? '—' : `${value.toFixed(0)}%`}
        </span>
      </div>
      <div className="h-1.5 w-full bg-base-800 rounded-full overflow-hidden">
        <div
          className={cn("h-full rounded-full transition duration-300", statusColor)}
          style={{ width: isEmpty ? '0%' : `${Math.min(value, 100)}%` }}
        />
      </div>
    </div>
  )
}

export default function HostOverview({ tiles, failed, uptimes, onSelect, onToggleMaintenance }: Props) {
  // 모든 호스트 스펙 1회 fetch (5분 캐시 — 자주 안 변함)
  const { data: specs = [] } = useQuery({
    queryKey: ['host-specs'],
    queryFn: listHostSpecs,
    staleTime: 5 * 60 * 1000,
  })
  const specByHost = new Map<string, HostSpec>(specs.map((s) => [s.host_name, s]))
  // 문제 순 정렬은 위 "처리할 문제"가 맡음 — 여기선 이름순으로 고정해 30초마다 카드 위치가 바뀌지 않게
  const hosts = [...tiles].sort((a, b) => a.name.localeCompare(b.name))

  return (
    <div id="hosts" className="space-y-3 scroll-mt-20">
      <div className="flex items-baseline gap-2">
        <h2 className="text-base font-bold text-base-100">호스트 목록</h2>
        <span className="text-xs font-medium text-base-500 tabular-nums">{hosts.length}개</span>
      </div>

      {hosts.length === 0 && failed && (
        <div role="alert" className="rounded-xl border border-amber-500/30 bg-base-900/50 p-10 flex flex-col items-center text-center gap-2">
          <AlertTriangle size={40} className="text-amber-400" />
          <p className="text-sm font-semibold text-base-300">호스트 목록을 불러오지 못했습니다</p>
          <p className="text-xs text-base-500">서버 또는 수집기(Prometheus) 연결을 확인하세요. 1분마다 다시 시도합니다</p>
        </div>
      )}
      {/* 호스트 0개 — 빈 그리드 대신 설치 안내 (데모 첫 화면이 "고장난 화면"으로 보이는 것 방지) */}
      {hosts.length === 0 && !failed && (
        <div className="rounded-xl border border-base-800 bg-base-900/50 p-10 flex flex-col items-center text-center gap-2">
          <Monitor size={40} className="text-base-700" />
          <p className="text-sm font-semibold text-base-300">연결된 호스트가 없습니다</p>
          <p className="text-xs text-base-500">에이전트를 설치하면 자동으로 표시됩니다 (첫 수집 후 30초 이내)</p>
        </div>
      )}

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 gap-3">
        {hosts.map((x) => {
          const host = x.name
          const uptime = uptimes[host]
          const inMaintenance = x.state === 'maint'
          // 최근 조회가 실패하면 마지막 값의 "정상"을 그대로 믿지 않게 (위험·주의는 마지막 값이라도 계속 보여 줌)
          const unknown = failed && x.state === 'ok'
          const label = x.down ? '연결 끊김' : unknown ? '확인 불가' : x.acked ? `${LABEL[x.state]} · 확인함` : LABEL[x.state]
          const bars = x.bars ?? [
            { label: 'CPU', value: null, warn: 0, crit: 0 },
            { label: '메모리', value: null, warn: 0, crit: 0 },
            { label: '디스크', value: null, warn: 0, crit: 0 },
          ]
          const spec = specByHost.get(host)

          return (
            <div
              key={x.key}
              onClick={() => onSelect(host)}
              className={cn(
                "group bg-base-900 rounded-xl border p-4 transition duration-200 flex flex-col h-full cursor-pointer",
                inMaintenance
                  ? "border-purple-500/30 bg-purple-500/5 hover:border-purple-500/60"
                  : x.state === 'crit' && !x.acked
                    ? "border-rose-500/50 hover:border-rose-500/80 ring-1 ring-rose-500/25"
                    : x.state === 'warn' && !x.acked
                      ? "border-amber-500/40 hover:border-amber-500/70"
                      : "border-base-800 hover:border-brand-500/60 hover:bg-base-900/80"
              )}
            >
              {/* Card Header */}
              <div className="flex items-start justify-between gap-2 mb-4">
                <div className="flex items-start gap-2 flex-1 min-w-0" title={host}>
                  <div className={cn("w-2 h-2 rounded-full shrink-0 mt-1.5", unknown || x.acked ? 'bg-base-500' : DOT[x.state])} />
                  <div className="min-w-0 flex-1">
                    <span className="font-semibold text-sm text-base-100 group-hover:text-brand-400 transition-colors block break-words leading-snug">
                      {host}
                      {hostLabel(host) && (
                        <span className="ml-1.5 text-xs font-normal text-base-400">({hostLabel(host)})</span>
                      )}
                    </span>
                    <span className="text-xs font-medium text-base-500 mt-0.5 block">{label}</span>
                  </div>
                </div>

                {!inMaintenance && (x.alertCount ?? 0) > 0 && (
                  <span className={cn("px-1.5 py-0.5 rounded-md border text-xs font-bold flex items-center gap-1 shrink-0 tabular-nums",
                    x.state === 'crit' ? "bg-rose-500/15 text-rose-400 border-rose-500/30" : "bg-amber-500/15 text-amber-400 border-amber-500/30")}>
                    <AlertCircle size={11} /> {x.alertCount}
                  </span>
                )}
              </div>

              {/* Hardware Mini Line (스펙 등록된 경우만) */}
              {spec && (
                <div className="flex items-center gap-1.5 mb-3 px-1 text-xs text-base-500 truncate" title={`${spec.cpu_model} · ${formatBytes(spec.memory_total_bytes)}`}>
                  <Cpu size={11} className="text-base-600 shrink-0" />
                  <span className="font-medium tabular-nums">{spec.cpu_cores}코어</span>
                  <span className="text-base-700">·</span>
                  <span className="truncate">{shortCPU(spec.cpu_model)}</span>
                  <span className="text-base-700">·</span>
                  <span className="font-medium shrink-0">{formatBytes(spec.memory_total_bytes)}</span>
                </div>
              )}

              {/* Metrics Section — 항상 표시 (오프라인이면 placeholder) */}
              <div className="space-y-2.5 mb-4 flex-1">
                {bars.map(b => <MetricBar key={b.label} label={b.label} value={b.value} warning={b.warn} critical={b.crit} />)}
              </div>

              {/* Footer */}
              <div className="pt-3 border-t border-base-800 flex items-center justify-between gap-2">
                <div className="flex items-baseline gap-1.5 min-w-0">
                  <span className="text-xs font-medium text-base-500 shrink-0">가동률</span>
                  {uptime !== undefined ? (
                    <span className={cn(
                      "text-sm font-semibold tabular-nums",
                      uptime >= 99 ? "text-emerald-400" : uptime >= 95 ? "text-amber-400" : "text-rose-400"
                    )}>
                      {uptime.toFixed(1)}%
                    </span>
                  ) : <span className="text-sm font-semibold text-base-600">—</span>}
                </div>

                <div className="flex gap-1 shrink-0">
                  <button
                    onClick={(e) => { e.stopPropagation(); onToggleMaintenance(host, !inMaintenance) }}
                    className={cn(
                      "p-1.5 rounded-md transition-colors border",
                      inMaintenance
                        ? "bg-purple-500 text-white border-transparent"
                        : "bg-base-800 text-base-400 border-base-700 hover:text-base-200 hover:bg-base-700"
                    )}
                    title={inMaintenance ? "점검 모드 해제" : "점검 모드 설정"}
                    aria-label={inMaintenance ? `${host} 점검 모드 해제` : `${host} 점검 모드 설정`}
                  >
                    <Wrench size={14} />
                  </button>
                  <button
                    onClick={(e) => { e.stopPropagation(); onSelect(host) }}
                    className="p-1.5 rounded-md bg-brand-500 text-white hover:bg-brand-600 transition-colors"
                    title="상세 보기"
                    aria-label={`${host} 상세 보기`}
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
  )
}
