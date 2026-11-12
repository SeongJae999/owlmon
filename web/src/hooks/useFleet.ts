import { useQuery } from '@tanstack/react-query'
import { useHosts, useAllHostStatuses, useAllHostMetrics, useAlertStatus, useMaintenanceHosts } from './useMonitoring'
import { getAlertConfig, thresholds } from '../api/alert'
import { getSNMPStatus } from '../api/snmp'
import { getSyntheticStatus } from '../api/synthetic'
import { getSSLStatus } from '../api/ssl'
import { getDPMStatus } from '../api/dpm'
import { fetchAssets } from '../api/asset'
import { getAnomalyData } from '../api/anomaly'
import { hostLabel } from '../config/hostLabels'
import { defaultLayout, getTopologyLayout } from '../api/topology'

// 상황판과 메인 화면이 같은 기준(원인별 묶음, 확인된 문제 제외)으로 세도록 장비 상태 집계를 한곳에 둔다
export type State = 'crit' | 'warn' | 'maint' | 'ok' | 'wait'
export type Group = '서버' | '네트워크 장비' | '웹사이트' | '데이터베이스'

export interface Tile {
  key: string
  group: Group
  name: string
  sub?: string
  state: State
  detail: string
  down?: boolean
  bars?: { label: string; value: number | null; warn: number; crit: number }[]
  to: string
  causedBy?: string
  acked?: { by: string; at?: string }
  purpose?: string
  since?: string
}

export const RANK: Record<State, number> = { crit: 0, warn: 1, wait: 2, maint: 3, ok: 4 }
export const LABEL: Record<State, string> = { crit: '위험', warn: '주의', maint: '점검 중', ok: '정상', wait: '확인 중' }
export const GROUPS: { name: Group; unit: string }[] = [
  { name: '서버', unit: '대' },
  { name: '네트워크 장비', unit: '대' },
  { name: '웹사이트', unit: '곳' },
  { name: '데이터베이스', unit: '개' },
]

export function useFleet() {
  const opts = { refetchInterval: 30_000, refetchIntervalInBackground: true }
  const qHosts = useHosts()
  const hosts = qHosts.data ?? []
  const qStatus = useAllHostStatuses(hosts)
  const hostStatuses = qStatus.data ?? {}
  const qMetrics = useAllHostMetrics()
  const hostMetrics = qMetrics.data ?? {}
  const qAlerts = useAlertStatus()
  const alerts = qAlerts.data ?? []
  const dataUpdatedAt = qAlerts.dataUpdatedAt
  const { data: maintenance = [] } = useMaintenanceHosts()
  const { data: cfg } = useQuery({ queryKey: ['alertConfig'], queryFn: getAlertConfig })
  const qSnmp = useQuery({ queryKey: ['snmpStatus'], queryFn: getSNMPStatus, ...opts })
  const snmp = qSnmp.data
  const qSynth = useQuery({ queryKey: ['syntheticStatus'], queryFn: getSyntheticStatus, ...opts })
  const synth = qSynth.data
  const qSsl = useQuery({ queryKey: ['sslStatus'], queryFn: getSSLStatus, ...opts })
  const ssl = qSsl.data
  const qDpm = useQuery({ queryKey: ['dpmStatus'], queryFn: getDPMStatus, ...opts })
  const dpm = qDpm.data

  // 조회가 실패해도 이전 값·빈 목록으로 그리면 "문제 없음"처럼 보인다 — 실패한 출처와 가장 오래된 갱신 시각을 함께 넘김
  const sources: { name: string; group?: Group; q: { isError: boolean; dataUpdatedAt: number } }[] = [
    { name: '서버 목록', group: '서버', q: qHosts }, { name: '서버 상태', group: '서버', q: qStatus },
    { name: '서버 지표', group: '서버', q: qMetrics }, { name: '알림', group: '서버', q: qAlerts },
    { name: '네트워크 장비', group: '네트워크 장비', q: qSnmp }, { name: '웹사이트', group: '웹사이트', q: qSynth },
    { name: '보안 인증서', group: '웹사이트', q: qSsl }, { name: '데이터베이스', group: '데이터베이스', q: qDpm },
  ]
  const failed = sources.filter(x => x.q.isError).map(x => x.name)
  const groupFailed = (g: Group) => sources.some(x => x.group === g && x.q.isError)
  const times = sources.map(x => x.q.dataUpdatedAt).filter(t => t > 0)
  const oldestUpdate = times.length ? Math.min(...times) : 0
  const { data: layout } = useQuery({ queryKey: ['topologyLayout'], queryFn: getTopologyLayout, staleTime: 5 * 60_000 })
  const { data: anomaly } = useQuery({ queryKey: ['anomaly'], queryFn: getAnomalyData, refetchInterval: 60_000 })
  const { data: assets = [] } = useQuery({ queryKey: ['assets'], queryFn: fetchAssets, staleTime: 5 * 60_000 })
  // 장비 이름만으로는 무슨 장비인지 모르니 장비 대장의 설명을 용도로 붙인다
  const purposeOf = (name: string) => assets.find(a => a.host_name === name)?.description || hostLabel(name)

  const t = thresholds(cfg)
  const maint = new Set(maintenance)
  const tiles: Tile[] = []

  const fullIn = new Map<string, number>()
  for (const p of anomaly?.disk_predictions ?? []) {
    if (p.days_left >= 0) fullIn.set(p.host, Math.min(fullIn.get(p.host) ?? Infinity, p.days_left))
  }

  for (const h of hosts) {
    const mine = alerts.filter(a => a.host === h && !a.in_maintenance)
    const m = hostMetrics[h]
    const state: State = maint.has(h) ? 'maint'
      : hostStatuses[h] === 'offline' || mine.some(a => a.severity === 'critical') ? 'crit'
      : mine.length > 0 ? 'warn'
      : hostStatuses[h] ? 'ok' : 'wait'
    const since = mine.map(a => a.since).filter((x): x is string => !!x).sort()[0]
    tiles.push({
      key: `h-${h}`, group: '서버', name: h, purpose: purposeOf(h), state, since,
      detail: hostStatuses[h] === 'offline' ? '연결 끊김'
        : [mine[0]?.message, (fullIn.get(h) ?? Infinity) <= 30 && `약 ${Math.max(1, Math.ceil(fullIn.get(h)!))}일 후 디스크 가득 참`].filter(Boolean).join(' · '),
      down: hostStatuses[h] === 'offline',
      acked: mine.length > 0 && mine.every(a => a.acked) ? { by: mine[0].acked_by ?? '', at: mine[0].acked_at } : undefined,
      bars: hostStatuses[h] === 'offline' ? undefined : [
        { label: 'CPU', value: m?.cpu ?? null, warn: t.cpuWarn, crit: t.cpuCrit },
        { label: '메모리', value: m?.memory ?? null, warn: t.memWarn, crit: t.memCrit },
        { label: '디스크', value: m?.disk ?? null, warn: t.diskWarn, crit: t.diskCrit },
      ],
      to: `/detail?host=${h}`,
    })
  }
  for (const d of snmp ?? []) {
    tiles.push({
      key: `n-${d.Device.ID}`, group: '네트워크 장비', name: d.Device.Name, sub: d.Device.IP, purpose: purposeOf(d.Device.Name),
      state: !d.CollectedAt ? 'wait' : d.Up ? 'ok' : 'crit',
      down: !!d.CollectedAt && !d.Up,
      since: d.Up ? undefined : d.DownSince,
      detail: !d.CollectedAt ? '첫 응답 대기' : d.Up ? `응답 ${d.ResponseMs ?? 0}ms` : '응답 없음',
      to: '/snmp',
    })
  }
  for (const s of (synth ?? []).filter(s => s.monitor.enabled)) {
    const ok = s.latest?.success
    tiles.push({
      key: `w-${s.monitor.id}`, group: '웹사이트', name: s.monitor.name, sub: s.monitor.url.replace(/^https?:\/\//, ''),
      state: !s.latest ? 'wait' : ok ? (s.latest.response_time_ms > 3000 ? 'warn' : 'ok') : 'crit',
      detail: !s.latest ? '첫 점검 대기' : ok
        ? `응답 ${s.latest.response_time_ms}ms${s.latest.response_time_ms > 3000 ? ' (느림)' : ''}`
        : (s.latest.error || `HTTP ${s.latest.status_code}`),
      to: '/website',
    })
  }
  for (const c of ssl ?? []) {
    const state: State = c.status === 'expired' || c.status === 'critical' || c.status === 'error' ? 'crit'
      // 상황판엔 바로 대응할 것만: 몇 주 남은 인증서까지 노랗게 두면 늘 켜져 있어 무시하게 된다
      : c.status === 'warning' && c.days_left <= 7 ? 'warn' : 'ok'
    tiles.push({
      key: `s-${c.domain}:${c.port}`, group: '웹사이트', name: c.domain, sub: '보안 인증서',
      state,
      detail: c.status === 'error' ? '인증서 확인 실패' : c.days_left < 0 ? '인증서 만료' : `인증서 ${c.days_left}일 남음`,
      to: '/website',
    })
  }
  for (const d of dpm ?? []) {
    const m = d.metrics
    tiles.push({
      key: `d-${d.instance.id}`, group: '데이터베이스', name: d.instance.name, sub: d.instance.db_type,
      state: !m ? 'wait' : m.error ? 'crit' : 'ok',
      detail: !m ? '첫 응답 대기' : m.error ? '연결 실패' : `연결 ${m.connections_active}/${m.connections_max}`,
      to: '/dpm',
    })
  }
  const GORDER = GROUPS.map(g => g.name)
  // 문제 있는 타일을 맨 앞에, 나머지는 종류별로 묶어서
  tiles.sort((a, b) => Math.min(RANK[a.state], 2) - Math.min(RANK[b.state], 2) || GORDER.indexOf(a.group) - GORDER.indexOf(b.group) || a.name.localeCompare(b.name))

  // 위쪽 장비가 끊겨 함께 끊긴 장비는 따로 세지 않고 원인 장비 아래로 묶는다
  const topology = layout?.devices.length ? layout : defaultLayout(hosts, (snmp ?? []).map(d => d.Device.Name))
  const parentOf = new Map(topology.devices.map(d => [d.name, d.parent]))
  const downNames = new Set(tiles.filter(x => x.down).map(x => x.name))
  for (const x of tiles.filter(t => t.down)) {
    for (let p = parentOf.get(x.name), hops = 0; p && hops < 50; p = parentOf.get(p), hops++) {
      if (downNames.has(p)) x.causedBy = p
    }
    if (x.causedBy) x.detail = `${x.causedBy} 장애 영향`
  }
  const impactOf = (name: string) => tiles.filter(x => x.causedBy === name).map(x => x.name)

  // 서버 알림만이 아니라 네트워크 장비, 웹사이트까지 위 합계와 같은 기준으로
  // 담당자가 확인한 문제는 목록 아래로 내리고 합계에서 빼서, 아직 아무도 안 본 문제가 먼저 보이게
  const open = tiles.filter(x => (x.state === 'crit' || x.state === 'warn') && !x.causedBy)
    .sort((a, b) => Number(!!a.acked) - Number(!!b.acked))
  const fresh = open.filter(x => !x.acked)
  const handling = open.length - fresh.length
  const crit = fresh.filter(x => x.state === 'crit').length
  const warn = fresh.filter(x => x.state === 'warn').length


  return { tiles, open, fresh, handling, crit, warn, impactOf, topology, layout, synth, dataUpdatedAt, failed, groupFailed, oldestUpdate }
}
