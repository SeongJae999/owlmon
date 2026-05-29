import { useQuery } from '@tanstack/react-query'
import { HardDrive, AlertTriangle } from 'lucide-react'
import { fetchDiskUsageByMount, type DiskMountUsage } from '../api/prometheus'
import { formatBytes, getHostSpec, type DiskInfo } from '../api/specs'
import { cn } from '../utils/cn'

interface Props {
  host: string
  // 알림 설정과 동일한 임계치를 내려받아 색상/경고를 통일 (기본값은 서버 기본값과 일치)
  warnThreshold?: number
  critThreshold?: number
}

/**
 * 마운트포인트(디스크)별 사용량 — SSD/HDD 각각 얼마 남았는지 표시.
 * Prometheus의 system_disk_* 메트릭(mountpoint/device 라벨)을 마운트별로 분리해서 보여준다.
 */
export default function DiskUsageBreakdown({ host, warnThreshold = 85, critThreshold = 90 }: Props) {
  const { data: mounts = [], isLoading, error } = useQuery({
    queryKey: ['disk-usage-mounts', host],
    queryFn: () => fetchDiskUsageByMount(host),
    enabled: !!host,
    refetchInterval: 30000,
  })

  // 물리 디스크 스펙 — device 라벨을 SSD/HDD로 매핑하는 데 사용 (HostSpecCard와 캐시 공유)
  const { data: spec } = useQuery({
    queryKey: ['spec', host],
    queryFn: () => getHostSpec(host),
    enabled: !!host,
    staleTime: 5 * 60 * 1000,
  })

  if (isLoading) {
    return (
      <div className="rounded-lg border border-slate-800 bg-slate-900/50 p-4 text-sm text-slate-500">
        디스크 사용량 불러오는 중…
      </div>
    )
  }

  if (error) {
    return (
      <div className="rounded-lg border border-slate-800 bg-slate-900/50 p-4 text-sm text-slate-500">
        디스크 사용량을 불러오지 못했습니다.
        <span className="block mt-0.5 text-xs text-slate-600">
          (네트워크 또는 Prometheus 연결을 확인하세요)
        </span>
      </div>
    )
  }

  if (mounts.length === 0) {
    return (
      <div className="rounded-lg border border-slate-800 bg-slate-900/50 p-4 text-sm text-slate-500">
        디스크 사용량 데이터가 없습니다.
        <span className="block mt-0.5 text-xs text-slate-600">
          (에이전트가 마운트별 용량을 수집하면 표시됩니다)
        </span>
      </div>
    )
  }

  return (
    <div className="rounded-lg border border-slate-800 bg-slate-900/50 divide-y divide-slate-800/60">
      {mounts.map((m) => (
        <DiskUsageRow
          key={`${m.mountpoint}|${m.device}`}
          m={m}
          warn={warnThreshold}
          crit={critThreshold}
          kind={deviceToDiskKind(m.device, spec?.disks)}
        />
      ))}
    </div>
  )
}

/**
 * Prometheus device 라벨(/dev/sda1, /dev/nvme0n1p2 등)을 디스크 종류로 매핑.
 *   - SSD/HDD: 물리 파티션을 spec의 물리 디스크(rotational)로 역추적
 *   - LVM:     device-mapper(/dev/mapper, dm-)는 논리 볼륨 → 'LVM' (dm 번호는 무의미해 라벨 숨김)
 *   - null:    spec에 없는 물리 디바이스 → 아이콘 폴백
 */
function deviceToDiskKind(device: string, disks?: DiskInfo[]): 'SSD' | 'HDD' | 'LVM' | null {
  if (!device) return null
  // /dev/ 접두 제거
  const dev = device.replace(/^\/dev\//, '')
  // LVM / device-mapper는 물리 디스크가 아닌 "논리 볼륨" — dm-N 번호는 운영자에게 무의미
  if (dev.startsWith('mapper/') || dev.startsWith('dm-')) return 'LVM'
  if (!disks?.length) return null
  // 파티션 번호 제거 → 베이스 디스크명
  //   nvme0n1p2 → nvme0n1 (nvme는 p<숫자>가 파티션), sda1 → sda, vdb3 → vdb
  const base = /^nvme/.test(dev) ? dev.replace(/p\d+$/, '') : dev.replace(/\d+$/, '')
  const match = disks.find((d) => d.name.replace(/^\/dev\//, '') === base)
  if (!match) return null
  return match.rotational ? 'HDD' : 'SSD'
}

function DiskUsageRow({
  m, warn, crit, kind,
}: {
  m: DiskMountUsage
  warn: number
  crit: number
  kind: 'SSD' | 'HDD' | 'LVM' | null
}) {
  const pct = Math.min(100, Math.max(0, m.usedPercent))
  const barColor = pct >= crit ? 'bg-rose-500' : pct >= warn ? 'bg-amber-500' : 'bg-emerald-500'
  const pctColor = pct >= crit ? 'text-rose-400' : pct >= warn ? 'text-amber-400' : 'text-emerald-400'
  const freeLabel = m.freeBytes > 0 ? formatBytes(m.freeBytes) : '0 B'

  return (
    <div className="px-4 py-3">
      <div className="flex items-center justify-between gap-3 mb-2">
        <div className="flex items-center gap-2 min-w-0">
          {/* 종류 배지 — SSD/HDD(물리) · LVM(논리 볼륨). 매핑 실패 시 아이콘 폴백 */}
          {kind ? (
            <span
              className={cn(
                'px-1.5 py-0.5 rounded text-[10px] font-bold shrink-0',
                kind === 'HDD'
                  ? 'bg-amber-900/40 text-amber-300'
                  : kind === 'SSD'
                    ? 'bg-emerald-900/40 text-emerald-300'
                    : 'bg-sky-900/40 text-sky-300',
              )}
            >
              {kind}
            </span>
          ) : (
            <HardDrive size={13} className="text-slate-500 shrink-0" />
          )}
          {/* 핵심 정보인 mountpoint는 잘리지 않게(shrink-0) */}
          <span className="text-sm font-semibold text-slate-200 shrink-0" title={m.mountpoint}>
            {m.mountpoint}
          </span>
          {/* 루트(/)는 시스템 핵심 디스크 — 운영자가 우선 보게 살짝 강조 */}
          {m.mountpoint === '/' && (
            <span className="text-[10px] font-medium text-indigo-300/80 bg-indigo-500/10 px-1.5 py-0.5 rounded shrink-0">
              시스템
            </span>
          )}
          {/* device 보조 표기 — dm-N(LVM)은 무의미한 번호라 숨김, 물리 디바이스만 표시 */}
          {m.device && kind !== 'LVM' && (
            <span className="text-[11px] font-mono text-slate-500 truncate max-w-[140px]" title={m.device}>
              {m.device}
            </span>
          )}
        </div>
        <span className={cn('flex items-center gap-1 text-sm font-bold tabular-nums shrink-0', pctColor)}>
          {pct >= warn && <AlertTriangle size={12} />}
          {pct.toFixed(0)}%
        </span>
      </div>

      <div className="h-2 w-full rounded-full bg-slate-800 overflow-hidden">
        <div className={cn('h-full rounded-full transition-all duration-500', barColor)} style={{ width: `${pct}%` }} />
      </div>

      <div className="flex items-center justify-between mt-1.5 text-[11px] text-slate-500 tabular-nums">
        <span>
          {formatBytes(m.usedBytes)} / {formatBytes(m.totalBytes)}
        </span>
        <span className="font-semibold text-slate-400">{freeLabel} 남음</span>
      </div>
    </div>
  )
}
