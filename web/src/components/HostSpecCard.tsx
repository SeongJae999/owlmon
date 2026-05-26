import { useQuery } from '@tanstack/react-query'
import { Cpu, MemoryStick, HardDrive, Network, Server, Box } from 'lucide-react'
import {
  getHostSpec,
  formatBytes,
  formatVirtualization,
  shortCPU,
  summarizeDisks,
  type DiskInfo,
  type NetworkInfo,
} from '../api/specs'

interface Props {
  host: string
}

/**
 * 한 줄 요약 + "더보기" 토글로 풀 디테일.
 * 호스트 스펙은 자주 보는 정보가 아니라 평소엔 압축, 필요 시만 펼침.
 */
export default function HostSpecCard({ host }: Props) {

  const { data: spec, isLoading, error } = useQuery({
    queryKey: ['spec', host],
    queryFn: () => getHostSpec(host),
    enabled: !!host,
    staleTime: 5 * 60 * 1000, // 스펙은 자주 안 변함
  })

  if (isLoading) {
    return (
      <div className="rounded-lg border border-slate-800 bg-slate-900/50 p-4 text-sm text-slate-500">
        하드웨어 정보 불러오는 중…
      </div>
    )
  }

  if (error || !spec) {
    return (
      <div className="rounded-lg border border-slate-800 bg-slate-900/50 p-4 text-sm text-slate-500">
        이 호스트의 하드웨어 정보가 아직 등록되지 않았습니다.
        <span className="block mt-0.5 text-xs text-slate-600">
          (에이전트가 시작되며 자동 전송됩니다 — 첫 부팅 후 30초 이내)
        </span>
      </div>
    )
  }

  const oneLineSummary = [
    shortCPU(spec.cpu_model),
    spec.cpu_cores ? `${spec.cpu_cores}코어` : '',
    formatBytes(spec.memory_total_bytes),
    spec.os_pretty_name,
  ]
    .filter(Boolean)
    .join(' · ')

  const subLine = [
    formatVirtualization(spec.virtualization),
    summarizeDisks(spec.disks ?? []),
  ]
    .filter(Boolean)
    .join(' · ')

  return (
    <div className="rounded-lg border border-slate-800 bg-slate-900/50 overflow-hidden">
      {/* 한 줄 요약 — 외부 collapsible 안에 들어가므로 자체 toggle 제거 (이중 펼치기 UX 안티패턴) */}
      <div className="w-full flex items-center gap-3 px-4 py-3 text-left">
        <Server size={16} className="text-violet-400 shrink-0" />
        <div className="min-w-0 flex-1">
          <div className="text-sm text-slate-200 font-medium truncate">{oneLineSummary}</div>
          <div className="text-xs text-slate-500 mt-0.5 truncate">{subLine}</div>
        </div>
      </div>

      {/* 디테일 — 항상 표시 (외부 collapsible로 토글) */}
      <div className="border-t border-slate-800 grid grid-cols-1 md:grid-cols-2 gap-x-6 gap-y-4 p-4">
          <DetailRow icon={<Cpu size={14} />} label="CPU 모델">
            <div className="text-slate-300 text-sm">{spec.cpu_model || '—'}</div>
            <div className="text-xs text-slate-500 mt-0.5">
              {spec.cpu_cores}코어 · {spec.cpu_sockets}소켓 · {spec.arch}
            </div>
          </DetailRow>

          <DetailRow icon={<MemoryStick size={14} />} label="메모리">
            <div className="text-slate-300 text-sm">{formatBytes(spec.memory_total_bytes)}</div>
          </DetailRow>

          <DetailRow icon={<Box size={14} />} label="OS / 커널">
            <div className="text-slate-300 text-sm">{spec.os_pretty_name || '—'}</div>
            <div className="text-xs text-slate-500 mt-0.5">
              커널 {spec.kernel_version} · {formatVirtualization(spec.virtualization)}
            </div>
          </DetailRow>

          <DetailRow icon={<HardDrive size={14} />} label="디스크" wide>
            {spec.disks?.length ? (
              <div className="space-y-1">
                {spec.disks.map((d) => (
                  <DiskRow key={d.name} disk={d} />
                ))}
              </div>
            ) : (
              <div className="text-slate-500 text-sm">정보 없음</div>
            )}
          </DetailRow>

          <DetailRow icon={<Network size={14} />} label="네트워크" wide>
            {spec.networks?.length ? (
              <div className="space-y-1">
                {spec.networks.map((n) => (
                  <NetworkRow key={n.name} net={n} />
                ))}
              </div>
            ) : (
              <div className="text-slate-500 text-sm">정보 없음</div>
            )}
          </DetailRow>

          <div className="md:col-span-2 text-[11px] text-slate-600 pt-2 border-t border-slate-800/60">
            마지막 갱신: {new Date(spec.updated_at).toLocaleString('ko-KR')}
          </div>
      </div>
    </div>
  )
}

// ─── 내부 서브컴포넌트 ────────────────────────────────

function DetailRow({
  icon,
  label,
  children,
  wide,
}: {
  icon: React.ReactNode
  label: string
  children: React.ReactNode
  wide?: boolean
}) {
  return (
    <div className={wide ? 'md:col-span-2' : ''}>
      <div className="flex items-center gap-1.5 text-[11px] text-slate-500 mb-1">
        <span className="text-slate-600">{icon}</span>
        <span className="uppercase tracking-wide font-semibold">{label}</span>
      </div>
      {children}
    </div>
  )
}

function DiskRow({ disk }: { disk: DiskInfo }) {
  const kind = disk.rotational ? 'HDD' : 'SSD'
  const kindColor = disk.rotational
    ? 'bg-amber-900/40 text-amber-300'
    : 'bg-emerald-900/40 text-emerald-300'

  return (
    <div className="flex items-center gap-2 text-sm">
      <span className={`px-1.5 py-0.5 rounded text-[10px] font-bold ${kindColor}`}>{kind}</span>
      <span className="text-slate-300 font-mono text-xs">{disk.name}</span>
      <span className="text-slate-200">{formatBytes(disk.size_bytes)}</span>
      {disk.model && (
        <span className="text-slate-500 text-xs truncate" title={disk.model}>
          {disk.model}
        </span>
      )}
    </div>
  )
}

function NetworkRow({ net }: { net: NetworkInfo }) {
  return (
    <div className="flex items-center gap-2 text-sm">
      <span className="text-slate-300 font-mono text-xs">{net.name}</span>
      <span className="text-slate-200">{net.ipv4.join(', ') || '—'}</span>
      {net.mac && <span className="text-slate-500 text-xs font-mono">{net.mac}</span>}
    </div>
  )
}
