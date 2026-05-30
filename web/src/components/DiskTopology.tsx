import { useQuery } from '@tanstack/react-query'
import { getHostSpec, formatBytes, type BlockDevice } from '../api/specs'
import { fetchDiskUsageByMount, type DiskMountUsage } from '../api/prometheus'
import { cn } from '../utils/cn'

interface Props {
  host: string
}

// 할당 막대/트리 점 색 팔레트 — 파티션(자식) 순서대로 순환
const SEG_COLORS = [
  'bg-indigo-500/70',
  'bg-violet-500/70',
  'bg-sky-500/70',
  'bg-emerald-500/70',
  'bg-amber-500/70',
  'bg-rose-500/70',
  'bg-teal-500/70',
]

/**
 * 물리 디스크 분해 뷰 — 디스크별 "트리 + 할당 막대 헤더".
 *   - 헤더: SSD/HDD 배지 + 이름 + 총량 + 파티션 할당 비율 막대
 *   - 트리: 파티션→LVM→마운트 계층, 마운트된 볼륨은 사용량 바 표시
 *   - 데이터: 에이전트 lsblk 토폴로지(spec.disk_topology) + Prometheus 마운트 사용량 join
 */
export default function DiskTopology({ host }: Props) {
  const { data: spec, isLoading } = useQuery({
    queryKey: ['spec', host],
    queryFn: () => getHostSpec(host),
    enabled: !!host,
    staleTime: 5 * 60 * 1000,
  })
  const { data: usage = [] } = useQuery({
    queryKey: ['disk-usage-mounts', host],
    queryFn: () => fetchDiskUsageByMount(host),
    enabled: !!host,
    refetchInterval: 30000,
  })

  const disks = (spec?.disk_topology ?? []).filter((d) => d.type === 'disk')

  // 마운트포인트 → 사용량 맵 (트리 노드와 join)
  const usageByMount = new Map<string, DiskMountUsage>()
  usage.forEach((u) => usageByMount.set(u.mountpoint, u))

  if (isLoading) {
    return (
      <div className="rounded-lg border border-slate-800 bg-slate-900/50 p-4 text-sm text-slate-500">
        디스크 구조 불러오는 중…
      </div>
    )
  }

  if (!disks.length) {
    return (
      <div className="rounded-lg border border-slate-800 bg-slate-900/50 p-4 text-sm text-slate-500">
        물리 디스크 구조 데이터가 없습니다.
        <span className="block mt-0.5 text-xs text-slate-600">
          (에이전트 업데이트 후 Linux 호스트에서 lsblk로 수집됩니다)
        </span>
      </div>
    )
  }

  return (
    <div className="space-y-3">
      {disks.map((d) => (
        <PhysicalDisk key={d.name} disk={d} usageByMount={usageByMount} />
      ))}
    </div>
  )
}

function PhysicalDisk({
  disk,
  usageByMount,
}: {
  disk: BlockDevice
  usageByMount: Map<string, DiskMountUsage>
}) {
  const kind = disk.rotational ? 'HDD' : 'SSD'
  const kindColor = disk.rotational
    ? 'bg-amber-900/40 text-amber-300'
    : 'bg-emerald-900/40 text-emerald-300'
  const children = disk.children ?? []
  const total = disk.size_bytes || 1

  return (
    <div className="rounded-lg border border-slate-800 bg-slate-900/50 overflow-hidden">
      {/* 헤더: 배지 + 이름 + 모델 + 총량 */}
      <div className="flex items-center gap-2 px-4 pt-3 pb-2">
        <span className={cn('px-1.5 py-0.5 rounded text-[10px] font-bold shrink-0', kindColor)}>{kind}</span>
        <span className="text-sm font-mono font-semibold text-slate-200 shrink-0">{disk.name}</span>
        {disk.model && (
          <span className="text-[11px] text-slate-500 truncate" title={disk.model}>
            {disk.model}
          </span>
        )}
        <span className="ml-auto text-sm font-bold text-slate-300 tabular-nums shrink-0">
          {formatBytes(disk.size_bytes)}
        </span>
      </div>

      {/* 할당 막대 — 파티션 크기 비율 (사용량 아님, "어떻게 쪼개졌나") */}
      {children.length > 0 && (
        <div className="px-4 pb-2.5">
          <div className="flex h-2.5 w-full rounded-full overflow-hidden bg-slate-800 gap-px">
            {children.map((c, i) => {
              const w = Math.max(0.5, (c.size_bytes / total) * 100)
              return (
                <div
                  key={c.name}
                  className={cn('h-full', SEG_COLORS[i % SEG_COLORS.length])}
                  style={{ width: `${w}%` }}
                  title={`${c.name} · ${formatBytes(c.size_bytes)}`}
                />
              )
            })}
          </div>
        </div>
      )}

      {/* 트리 — 파티션→LVM→마운트 */}
      <div className="border-t border-slate-800/60 px-2 py-1.5">
        {children.map((c, i) => (
          <TreeNode key={c.name} node={c} depth={0} colorIdx={i} usageByMount={usageByMount} />
        ))}
      </div>
    </div>
  )
}

function TreeNode({
  node,
  depth,
  colorIdx,
  usageByMount,
}: {
  node: BlockDevice
  depth: number
  colorIdx: number
  usageByMount: Map<string, DiskMountUsage>
}) {
  const u = node.mountpoint ? usageByMount.get(node.mountpoint) : undefined
  const isLVM = node.type === 'lvm'
  const children = node.children ?? []

  return (
    <>
      <div
        className="flex items-center gap-2 py-1 text-xs"
        style={{ paddingLeft: `${depth * 16 + 8}px` }}
        title={u ? `${formatBytes(u.usedBytes)} / ${formatBytes(u.totalBytes)} 사용` : undefined}
      >
        {/* 할당 막대와 매칭되는 색 점 (최상위 파티션 색을 자식까지 상속) */}
        <span className={cn('w-2 h-2 rounded-sm shrink-0', SEG_COLORS[colorIdx % SEG_COLORS.length])} />

        {/* LVM 논리 볼륨 표시 */}
        {isLVM && (
          <span className="px-1 py-0.5 rounded text-[9px] font-bold bg-sky-900/40 text-sky-300 shrink-0">LVM</span>
        )}
        <span className="font-mono text-slate-300 shrink-0">{node.name}</span>

        {/* 마운트포인트 / 미마운트 표시 */}
        {node.mountpoint ? (
          <span className="text-slate-400 shrink-0">{node.mountpoint}</span>
        ) : children.length === 0 ? (
          <span className="text-slate-600 shrink-0">{node.fstype === 'swap' ? 'swap' : '미마운트'}</span>
        ) : null}

        <span className="ml-auto text-slate-500 tabular-nums shrink-0">{formatBytes(node.size_bytes)}</span>

        {/* 마운트된 볼륨만 사용량 미니바 */}
        {u && u.totalBytes > 0 ? (
          <div className="flex items-center gap-1.5 w-28 shrink-0">
            <div className="h-1.5 flex-1 rounded-full bg-slate-800 overflow-hidden">
              <div
                className={cn(
                  'h-full rounded-full',
                  u.usedPercent >= 90 ? 'bg-rose-500' : u.usedPercent >= 85 ? 'bg-amber-500' : 'bg-emerald-500',
                )}
                style={{ width: `${Math.min(100, u.usedPercent)}%` }}
              />
            </div>
            <span className="text-[10px] text-slate-400 tabular-nums w-9 text-right">
              {u.usedPercent.toFixed(0)}%
            </span>
          </div>
        ) : (
          <div className="w-28 shrink-0" />
        )}
      </div>

      {children.map((c) => (
        <TreeNode key={c.name} node={c} depth={depth + 1} colorIdx={colorIdx} usageByMount={usageByMount} />
      ))}
    </>
  )
}
