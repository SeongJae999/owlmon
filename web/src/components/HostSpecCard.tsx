import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Cpu, MemoryStick, HardDrive, Network, Box, ChevronDown } from 'lucide-react'
import {
  getHostSpec,
  formatBytes,
  formatVirtualization,
  shortCPU,
  type DiskInfo,
  type NetworkInfo,
} from '../api/specs'
import { cn } from '../utils/cn'

interface Props {
  host: string
}

/**
 * 호스트 스펙 — 핵심 4 stat + 보조 텍스트 + 부가 collapsible
 *
 * UX 위계:
 *   🥇 핵심 (즉시): CPU 코어 / RAM / 디스크 총량 / OS
 *   🥈 보조 (텍스트): CPU 모델 전체, 커널 버전, 가상화
 *   🥉 부가 (펼침): 디스크 마운트별, 네트워크 인터페이스
 */
export default function HostSpecCard({ host }: Props) {
  const [showAdvanced, setShowAdvanced] = useState(false)

  const { data: spec, isLoading, error } = useQuery({
    queryKey: ['spec', host],
    queryFn: () => getHostSpec(host),
    enabled: !!host,
    staleTime: 5 * 60 * 1000,
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

  // 디스크 총량 합산
  const diskTotalBytes = (spec.disks ?? []).reduce((sum, d) => sum + (d.size_bytes || 0), 0)
  const hasDisks = (spec.disks ?? []).length > 0
  const hasNetworks = (spec.networks ?? []).length > 0

  return (
    <div className="rounded-lg border border-slate-800 bg-slate-900/50 overflow-hidden">
      {/* 🥇 핵심 4 stat — 즉시 보이는 정보 */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 p-3">
        <StatBlock icon={Cpu}        label="CPU"    value={`${spec.cpu_cores}`}                       unit="코어" color="indigo" />
        <StatBlock icon={MemoryStick} label="메모리"  value={formatBytes(spec.memory_total_bytes).replace(/\s/g, '')} color="violet" splitUnit />
        <StatBlock icon={HardDrive}  label="디스크"  value={diskTotalBytes > 0 ? formatBytes(diskTotalBytes).replace(/\s/g, '') : '—'} color="sky" splitUnit />
        <StatBlock icon={Box}        label="OS"     value={shortenOS(spec.os_pretty_name)}            color="emerald" />
      </div>

      {/* 🥈 보조 정보 — 한 줄 텍스트 */}
      <div className="border-t border-slate-800 px-4 py-2.5 text-[11px] text-slate-500 space-y-0.5">
        <div className="truncate" title={spec.cpu_model}>
          <span className="text-slate-600">CPU:</span>{' '}
          <span className="text-slate-400">{spec.cpu_model || '—'}</span>
          <span className="text-slate-600"> · {spec.cpu_sockets}소켓 · {spec.arch}</span>
        </div>
        <div className="truncate">
          <span className="text-slate-600">커널:</span>{' '}
          <span className="text-slate-400">{spec.kernel_version || '—'}</span>
          {spec.virtualization && (
            <>
              <span className="text-slate-600"> · </span>
              <span className="text-slate-400">{formatVirtualization(spec.virtualization)}</span>
            </>
          )}
        </div>
      </div>

      {/* 🥉 부가 정보 — 디스크/네트워크 (collapsible, 의미 있을 때만 펼침) */}
      {(hasDisks || hasNetworks) && (
        <button
          onClick={() => setShowAdvanced(v => !v)}
          className="w-full border-t border-slate-800 px-4 py-2 flex items-center justify-between text-[11px] font-semibold text-slate-500 hover:bg-slate-800/30 transition-colors"
        >
          <span>
            디스크 {hasDisks && `${spec.disks!.length}개`}
            {hasDisks && hasNetworks && ' · '}
            {hasNetworks && `네트워크 ${spec.networks!.length}개`}
          </span>
          <ChevronDown size={12} className={cn("transition-transform", showAdvanced && "rotate-180")} />
        </button>
      )}
      {showAdvanced && (
        <div className="border-t border-slate-800 px-4 py-3 space-y-3 text-sm">
          {hasDisks && (
            <div>
              <div className="text-[10px] uppercase tracking-wider text-slate-500 mb-1.5 flex items-center gap-1.5">
                <HardDrive size={11} /> 디스크
              </div>
              <div className="space-y-1">
                {spec.disks!.map(d => <DiskRow key={d.name} disk={d} />)}
              </div>
            </div>
          )}
          {hasNetworks && (
            <div>
              <div className="text-[10px] uppercase tracking-wider text-slate-500 mb-1.5 flex items-center gap-1.5">
                <Network size={11} /> 네트워크
              </div>
              <div className="space-y-1">
                {spec.networks!.map(n => <NetworkRow key={n.name} net={n} />)}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// ─── 핵심 stat 블록 (4개) ────────────────────────────────
function StatBlock({
  icon: Icon, label, value, unit, color, splitUnit,
}: {
  icon: any
  label: string
  value: string
  unit?: string
  color: string
  splitUnit?: boolean
}) {
  const colorMap: Record<string, string> = {
    indigo:  'text-indigo-400 bg-indigo-500/10',
    violet:  'text-violet-400 bg-violet-500/10',
    sky:     'text-sky-400 bg-sky-500/10',
    emerald: 'text-emerald-400 bg-emerald-500/10',
  }

  // splitUnit: "31GB" → ["31", "GB"]로 분리
  let displayValue = value
  let displayUnit = unit
  if (splitUnit && !unit) {
    const m = value.match(/^([\d.]+)\s*([A-Za-z]+)$/)
    if (m) {
      displayValue = m[1]
      displayUnit = m[2]
    }
  }

  return (
    <div className="bg-slate-900/60 rounded-md p-2.5 border border-slate-800/60">
      <div className={cn("inline-flex items-center justify-center w-6 h-6 rounded mb-1.5", colorMap[color])}>
        <Icon size={12} />
      </div>
      <div className="text-[10px] font-semibold text-slate-500 uppercase tracking-wider leading-none mb-1">{label}</div>
      <div className="flex items-baseline gap-1 leading-none">
        <span className="text-lg font-bold text-slate-100 tabular-nums truncate" title={value}>{displayValue}</span>
        {displayUnit && <span className="text-[10px] font-semibold text-slate-500">{displayUnit}</span>}
      </div>
    </div>
  )
}

function DiskRow({ disk }: { disk: DiskInfo }) {
  const kind = disk.rotational ? 'HDD' : 'SSD'
  const kindColor = disk.rotational
    ? 'bg-amber-900/40 text-amber-300'
    : 'bg-emerald-900/40 text-emerald-300'

  return (
    <div className="flex items-center gap-2 text-xs">
      <span className={`px-1.5 py-0.5 rounded text-[10px] font-bold ${kindColor}`}>{kind}</span>
      <span className="text-slate-300 font-mono">{disk.name}</span>
      <span className="text-slate-200">{formatBytes(disk.size_bytes)}</span>
      {disk.model && (
        <span className="text-slate-500 text-[11px] truncate" title={disk.model}>
          {disk.model}
        </span>
      )}
    </div>
  )
}

function NetworkRow({ net }: { net: NetworkInfo }) {
  return (
    <div className="flex items-center gap-2 text-xs">
      <span className="text-slate-300 font-mono">{net.name}</span>
      <span className="text-slate-200">{net.ipv4.join(', ') || '—'}</span>
      {net.mac && <span className="text-slate-500 text-[11px] font-mono">{net.mac}</span>}
    </div>
  )
}

// OS 이름 짧게 (긴 "Ubuntu 24.04.3 LTS (Noble Numbat)" → "Ubuntu 24.04")
function shortenOS(name?: string): string {
  if (!name) return '—'
  // "Ubuntu 24.04 LTS (...)" → "Ubuntu 24.04"
  const m = name.match(/^(\w+)\s+(\d+\.\d+)/)
  if (m) return `${m[1]} ${m[2]}`
  return name.split('(')[0].trim()
}
