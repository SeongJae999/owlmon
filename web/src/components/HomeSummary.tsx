import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Server, Network, Globe, Database } from 'lucide-react'
import type { ActiveAlert } from '../api/alert'
import { getSNMPStatus } from '../api/snmp'
import { getSyntheticStatus } from '../api/synthetic'
import { getSSLStatus } from '../api/ssl'
import { getDPMStatus } from '../api/dpm'
import { TONES } from '../utils/severity'
import { cn } from '../utils/cn'

interface Props {
  hosts: string[]
  hostStatuses: Record<string, string>
  activeAlerts: ActiveAlert[]
  maintenanceHosts: string[]
}

// 서버만 보이던 홈에서 네트워크·웹사이트·DB 장애도 한 줄로 보이게 함
export default function HomeSummary({ hosts, hostStatuses, activeAlerts, maintenanceHosts }: Props) {
  const opts = { refetchInterval: 60_000 }
  // 각 페이지와 같은 queryKey — 페이지 이동 시 캐시 재사용
  const { data: snmp } = useQuery({ queryKey: ['snmpStatus'], queryFn: getSNMPStatus, ...opts })
  const { data: synth } = useQuery({ queryKey: ['syntheticStatus'], queryFn: getSyntheticStatus, ...opts })
  const { data: ssl } = useQuery({ queryKey: ['sslStatus'], queryFn: getSSLStatus, ...opts })
  const { data: dpm } = useQuery({ queryKey: ['dpmStatus'], queryFn: getDPMStatus, ...opts })

  const maint = new Set(maintenanceHosts)
  const serverDown = hosts.filter(h => !maint.has(h) && (
    hostStatuses[h] === 'offline' ||
    activeAlerts.some(a => a.host === h && a.severity === 'critical' && !a.in_maintenance)
  )).length

  const synthItems = (synth ?? []).filter(s => s.monitor.enabled)
  const webDown = synthItems.filter(s => s.latest && !s.latest.success).length +
    (ssl ?? []).filter(c => c.status === 'expired').length

  const tiles = [
    { to: '/', icon: Server, label: '서버', total: hosts.length, bad: serverDown },
    { to: '/snmp', icon: Network, label: '네트워크 장비', total: snmp?.length, bad: snmp?.filter(d => !d.Up).length ?? 0 },
    { to: '/website', icon: Globe, label: '웹사이트', total: synthItems.length + (ssl?.length ?? 0), bad: webDown },
    { to: '/dpm', icon: Database, label: '데이터베이스', total: dpm?.length, bad: dpm?.filter(d => d.metrics?.error).length ?? 0 },
  ]

  return (
    <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
      {tiles.map(t => {
        const loading = t.total === undefined
        const empty = t.total === 0
        const tone = TONES[loading || empty ? 'slate' : t.bad > 0 ? 'rose' : 'emerald']
        return (
          <Link
            key={t.label}
            to={t.to}
            className={cn('rounded-xl border p-3 flex items-center gap-3 transition-colors hover:brightness-125', tone.bg, tone.border)}
          >
            <t.icon size={20} className={cn('shrink-0 opacity-80', tone.text)} />
            <div className="min-w-0">
              <div className="text-xs font-semibold text-base-400">{t.label}</div>
              <div className={cn('text-sm font-bold', tone.text)}>
                {loading ? '…' : empty ? '등록 없음' : t.bad > 0 ? `문제 ${t.bad} / ${t.total}` : `모두 정상 (${t.total})`}
              </div>
            </div>
          </Link>
        )
      })}
    </div>
  )
}
