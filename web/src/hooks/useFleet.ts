import { useQuery } from '@tanstack/react-query'
import { useHosts, useAllHostStatuses, useAllHostMetrics, useAlertStatus, useMaintenanceHosts, useAnomalyData, useAlertAcks } from './useMonitoring'
import { getAlertConfig, thresholds, type ActiveAlert } from '../api/alert'
import { getSNMPStatus } from '../api/snmp'
import { getSyntheticStatus } from '../api/synthetic'
import { getSSLStatus } from '../api/ssl'
import { getDPMStatus } from '../api/dpm'
import { fetchAssets } from '../api/asset'
import { hostLabel } from '../config/hostLabels'
import { defaultLayout, getTopologyLayout } from '../api/topology'

// 상황판과 메인 화면이 같은 기준(원인별 묶음, 확인된 문제 제외)으로 세도록 장비 상태 집계를 한곳에 둔다
export type State = 'crit' | 'warn' | 'maint' | 'ok' | 'wait'
export type Group = '서버' | '네트워크 장비' | '웹사이트' | '데이터베이스'
export interface AckKey { host: string; category: string; severity: string }

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
  ackKeys?: AckKey[] // 확인 버튼이 서버에 보낼 키 — 서버가 메일 억제에 쓰는 키와 같아야 함
  alertCount?: number // 서버: 미확인 알림 수
  purpose?: string
  since?: string
}

export const RANK: Record<State, number> = { crit: 0, warn: 1, wait: 2, maint: 3, ok: 4 }
export const LABEL: Record<State, string> = { crit: '위험', warn: '주의', maint: '점검 중', ok: '정상', wait: '확인 중' }
export const GROUPS: { name: Group; unit: string; to: string }[] = [
  { name: '서버', unit: '대', to: '/#hosts' },
  { name: '네트워크 장비', unit: '대', to: '/snmp' },
  { name: '웹사이트', unit: '곳', to: '/website' },
  { name: '데이터베이스', unit: '개', to: '/dpm' },
]
// 가장 느린 조회 주기(1분)의 두 배 — 이보다 오래 갱신이 없으면 멈춘 화면으로 봄
export const STALE_MS = 2 * 60_000

// 서버 한 대의 상태 — 메인·상황판·상세가 같은 기준을 쓰도록 순수 함수로 둔다.
// 확인한 알림만 남았으면 그 알림들로, 새 알림이 있으면 미확인 알림만으로 등급을 정한다
// (위험을 확인한 뒤 주의가 새로 오면 "주의·미확인"이어야 함 — 확인한 위험으로 다시 "위험·미확인"이 되면 구성도 고정이 잘못 켜짐)
export function serverState(status: 'online' | 'offline' | undefined, mine: ActiveAlert[], inMaint: boolean): State {
  if (inMaint) return 'maint'
  if (status === 'offline') return 'crit'
  const unacked = mine.filter(a => !a.acked)
  const basis = unacked.length ? unacked : mine
  if (basis.some(a => a.severity === 'critical')) return 'crit'
  if (basis.length > 0) return 'warn'
  return status ? 'ok' : 'wait'
}

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
  const qMaint = useMaintenanceHosts()
  const maintenance = qMaint.data ?? []
  // 재조회 실패 땐 React Query가 지난 값을 들고 있음 — 이미 풀린 확인으로 새 장애를 "처리 중"으로 가리지 않게 버림.
  // 실패하면 모두 미확인(더 경고하는 쪽)으로 보이므로 화면 전체를 "확인 불가"로 만들진 않음
  const qAcks = useAlertAcks()
  const acks = qAcks.isError ? [] : (qAcks.data ?? [])
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
  const sources: { name: string; group?: Group; q: { isError: boolean; isPending: boolean; fetchStatus: string; dataUpdatedAt: number } }[] = [
    { name: '서버 목록', group: '서버', q: qHosts }, { name: '서버 상태', group: '서버', q: qStatus },
    { name: '서버 지표', group: '서버', q: qMetrics }, { name: '알림', group: '서버', q: qAlerts },
    { name: '점검 목록', group: '서버', q: qMaint },
    { name: '네트워크 장비', group: '네트워크 장비', q: qSnmp }, { name: '웹사이트', group: '웹사이트', q: qSynth },
    { name: '보안 인증서', group: '웹사이트', q: qSsl }, { name: '데이터베이스', group: '데이터베이스', q: qDpm },
  ]
  const failed = sources.filter(x => x.q.isError).map(x => x.name)
  const groupFailed = (g: Group) => sources.some(x => x.group === g && x.q.isError)
  const times = sources.map(x => x.q.dataUpdatedAt).filter(t => t > 0)
  const oldestUpdate = times.length ? Math.min(...times) : 0
  const stale = (now: Date) => failed.length > 0 || (oldestUpdate > 0 && now.getTime() - oldestUpdate > STALE_MS)
  // 첫 응답 전(오프라인으로 멈춘 경우 포함)이면 아직 판정할 수 없음 — "모두 정상"을 먼저 띄우지 않게.
  // 꺼 둔 쿼리(서버 0대일 때 상태 조회)는 idle이라 영구 "확인 중"이 되지 않음
  const waiting = (q: { isPending: boolean; fetchStatus: string }) => q.isPending && q.fetchStatus !== 'idle'
  const loading = waiting(qHosts)
  const pending = sources.some(x => waiting(x.q))
  const groupPending = (g: Group) => sources.some(x => x.group === g && waiting(x.q))
  // 한 번도 못 받은 채 실패한 경우도 모름 — 빈 목록으로 판정하면 위험 알림이 걸린 서버가 구성도에 초록으로 그려짐
  const unknown = (q: { isPending: boolean; fetchStatus: string; isError: boolean; data?: unknown }) => waiting(q) || (q.isError && q.data === undefined)
  const alertsUnknown = unknown(qAlerts) || unknown(qMaint)

  const { data: layout } = useQuery({ queryKey: ['topologyLayout'], queryFn: getTopologyLayout, staleTime: 5 * 60_000 })
  const { data: anomaly } = useAnomalyData()
  const { data: assets = [] } = useQuery({ queryKey: ['assets'], queryFn: fetchAssets, staleTime: 5 * 60_000 })
  // 장비 이름만으로는 무슨 장비인지 모르니 장비 대장의 설명을 용도로 붙인다
  const purposeOf = (name: string) => assets.find(a => a.host_name === name)?.description || hostLabel(name)
  const ackOf = (k: AckKey) => acks.find(a => a.host === k.host && a.category === k.category && a.severity === k.severity)

  const t = thresholds(cfg)
  const maint = new Set(maintenance)
  const tiles: Tile[] = []

  const fullIn = new Map<string, number>()
  for (const p of anomaly?.disk_predictions ?? []) {
    if (p.days_left >= 0) fullIn.set(p.host, Math.min(fullIn.get(p.host) ?? Infinity, p.days_left))
  }

  for (const h of hosts) {
    const mine = alerts.filter(a => a.host === h && !a.in_maintenance)
    const unacked = mine.filter(a => !a.acked)
    const m = hostMetrics[h]
    // 알림·점검 목록을 받기 전엔 끊김만 확실하고 나머지는 모름
    const state = alertsUnknown && hostStatuses[h] !== 'offline' ? 'wait' : serverState(hostStatuses[h], mine, maint.has(h))
    const since = mine.map(a => a.since).filter((x): x is string => !!x).sort()[0]
    // 미확인 알림을 먼저 보여 줌 — 확인한 알림 문구가 새 문제를 가리지 않게
    const lead = unacked[0] ?? mine[0]
    const more = mine.length > 1 ? ` 외 ${mine.length - 1}건` : ''
    tiles.push({
      key: `h-${h}`, group: '서버', name: h, purpose: purposeOf(h), state, since,
      detail: hostStatuses[h] === 'offline' ? '연결 끊김'
        : [lead && `${lead.message}${more}`, (fullIn.get(h) ?? Infinity) <= 30 && `약 ${Math.max(1, Math.ceil(fullIn.get(h)!))}일 후 디스크 가득 참`].filter(Boolean).join(' · '),
      down: hostStatuses[h] === 'offline',
      acked: mine.length > 0 && unacked.length === 0 ? { by: mine[0].acked_by ?? '', at: mine[0].acked_at } : undefined,
      ackKeys: unacked.map(a => ({ host: a.host, category: a.category, severity: a.severity })),
      alertCount: unacked.length,
      bars: hostStatuses[h] === 'offline' ? undefined : [
        { label: 'CPU', value: m?.cpu ?? null, warn: t.cpuWarn, crit: t.cpuCrit },
        { label: '메모리', value: m?.memory ?? null, warn: t.memWarn, crit: t.memCrit },
        { label: '디스크', value: m?.disk ?? null, warn: t.diskWarn, crit: t.diskCrit },
      ],
      to: `/detail?host=${encodeURIComponent(h)}`,
    })
  }
  for (const d of snmp ?? []) {
    const down = !!d.CollectedAt && !d.Up
    const key = { host: d.Device.Name, category: 'snmp', severity: 'critical' }
    // 네트워크 장비는 메일 경로가 없어 서버가 확인을 풀지 않음 — 이번 장애가 시작된 뒤의 확인만 인정
    const a = down ? ackOf(key) : undefined
    const ackValid = a && (!d.DownSince || a.acked_at >= d.DownSince)
    tiles.push({
      key: `n-${d.Device.ID}`, group: '네트워크 장비', name: d.Device.Name, sub: d.Device.IP, purpose: purposeOf(d.Device.Name),
      state: !d.CollectedAt ? 'wait' : d.Up ? 'ok' : 'crit',
      down,
      since: d.Up ? undefined : d.DownSince,
      detail: !d.CollectedAt ? '첫 응답 대기' : d.Up ? `응답 ${d.ResponseMs ?? 0}ms` : '응답 없음',
      acked: ackValid ? { by: a.acked_by, at: a.acked_at } : undefined,
      ackKeys: down && !ackValid ? [key] : undefined,
      to: '/snmp',
    })
  }
  for (const s of (synth ?? []).filter(s => s.monitor.enabled)) {
    const ok = s.latest?.success
    const failing = !!s.latest && !ok
    const key = { host: s.monitor.name, category: 'synthetic', severity: 'critical' }
    const a = failing ? ackOf(key) : undefined
    tiles.push({
      key: `w-${s.monitor.id}`, group: '웹사이트', name: s.monitor.name, sub: s.monitor.url.replace(/^https?:\/\//, ''),
      state: !s.latest ? 'wait' : ok ? (s.latest.response_time_ms > 3000 ? 'warn' : 'ok') : 'crit',
      detail: !s.latest ? '첫 점검 대기' : ok
        ? `응답 ${s.latest.response_time_ms}ms${s.latest.response_time_ms > 3000 ? ' (느림)' : ''}`
        : (s.latest.error || `HTTP ${s.latest.status_code}`),
      // 느림(주의)은 메일도 해제 경로도 없어 확인 대상이 아님 — 실패일 때만
      acked: a ? { by: a.acked_by, at: a.acked_at } : undefined,
      ackKeys: failing && !a ? [key] : undefined,
      to: '/website',
    })
  }
  for (const c of ssl ?? []) {
    const crit = c.status === 'expired' || c.status === 'critical' || c.status === 'error'
    // 확인 실패(error)는 메일이 없어 확인 대상에서 뺌 — 확인해 두면 뒤이은 진짜 만료 임박 메일이 막힘
    const key = { host: c.domain, category: 'ssl', severity: 'critical' }
    const ackable = c.status === 'expired' || c.status === 'critical'
    // 임박·만료가 같은 확인 키를 씀 — 만료 판정 전에 누른 확인(임박 건)은 만료 건 확인으로 보지 않음 (서버는 다음 주기에야 풂).
    // 서버는 남은 시간이 하루 미만이면 이미 만료로 판정하므로 기준도 만료 하루 전
    const a0 = ackable ? ackOf(key) : undefined
    const expiredFrom = new Date(c.not_after).getTime() - 86_400_000
    const a = a0 && !(c.status === 'expired' && new Date(a0.acked_at).getTime() < expiredFrom) ? a0 : undefined
    tiles.push({
      key: `s-${c.domain}:${c.port}`, group: '웹사이트', name: c.domain, sub: '보안 인증서',
      state: crit ? 'crit' : 'ok',
      detail: c.status === 'error' ? '인증서 확인 실패' : c.days_left < 0 ? '인증서 만료' : `인증서 ${c.days_left}일 남음`,
      acked: a ? { by: a.acked_by, at: a.acked_at } : undefined,
      ackKeys: ackable && !a ? [key] : undefined,
      to: '/website',
    })
  }
  for (const d of dpm ?? []) {
    const m = d.metrics
    const key = { host: d.instance.name, category: 'dpm', severity: 'critical' }
    const a = m?.error ? ackOf(key) : undefined
    tiles.push({
      key: `d-${d.instance.id}`, group: '데이터베이스', name: d.instance.name, sub: d.instance.db_type,
      state: !m ? 'wait' : m.error ? 'crit' : 'ok',
      detail: !m ? '첫 응답 대기' : m.error ? '연결 실패' : `연결 ${m.connections_active}/${m.connections_max}`,
      acked: a ? { by: a.acked_by, at: a.acked_at } : undefined,
      ackKeys: m?.error && !a ? [key] : undefined,
      to: '/dpm',
    })
  }
  const GORDER = GROUPS.map(g => g.name)
  // 문제 있는 타일을 맨 앞에, 나머지는 종류별로 묶어서
  tiles.sort((a, b) => Math.min(RANK[a.state], 2) - Math.min(RANK[b.state], 2) || GORDER.indexOf(a.group) - GORDER.indexOf(b.group) || a.name.localeCompare(b.name))

  // 위쪽 장비가 끊겨 함께 끊긴 장비는 따로 세지 않고 원인 장비 아래로 묶는다
  const topology = layout?.devices.length ? layout : defaultLayout(hosts, (snmp ?? []).map(d => d.Device.Name))
  const parentOf = new Map(topology.devices.map(d => [d.name, d.parent]))
  // 구성도엔 서버와 네트워크 장비만 있음 — 같은 이름의 웹사이트·인증서 타일과 섞이지 않게 그 둘만 본다
  const topoTiles = tiles.filter(x => x.group === '서버' || x.group === '네트워크 장비')
  // 점검 중인 장비는 목록에 안 나오므로 원인으로 묶으면 하위 장비 장애가 아무 데도 안 보임
  const downNames = new Set(topoTiles.filter(x => x.down && x.state !== 'maint').map(x => x.name))
  for (const x of topoTiles.filter(t => t.down)) {
    for (let p = parentOf.get(x.name), hops = 0; p && hops < 50; p = parentOf.get(p), hops++) {
      if (downNames.has(p)) x.causedBy = p
    }
    if (x.causedBy) x.detail = `${x.causedBy} 장애 영향`
  }
  // 원인 장비를 확인한 뒤 함께 끊긴 하위 장비 알림이 새로 생기면 원인 행을 다시 미확인으로 올려 확인 버튼을 살림
  for (const x of topoTiles) {
    if (x.acked && topoTiles.some(c => c.causedBy === x.name && c.ackKeys?.length)) x.acked = undefined
  }
  const impactOf = (name: string) => topoTiles.filter(x => x.causedBy === name).map(x => x.name)
  // 원인 장비 한 행을 확인하면 함께 끊긴 하위 장비의 끊김 메일도 멈춰야 관제사 기대와 맞음
  const rowAckKeys = (x: Tile): AckKey[] => [
    ...(x.ackKeys ?? []),
    ...topoTiles.filter(c => c.causedBy === x.name).flatMap(c => c.ackKeys ?? []),
  ]
  const topoLookup = (name: string) => topoTiles.find(x => x.name === name)

  // 서버 알림만이 아니라 네트워크 장비, 웹사이트까지 위 합계와 같은 기준으로
  // 담당자가 확인한 문제는 목록 아래로 내리고 합계에서 빼서, 아직 아무도 안 본 문제가 먼저 보이게
  const open = tiles.filter(x => (x.state === 'crit' || x.state === 'warn') && !x.causedBy)
    .sort((a, b) => Number(!!a.acked) - Number(!!b.acked))
  const fresh = open.filter(x => !x.acked)
  const handling = open.length - fresh.length
  const crit = fresh.filter(x => x.state === 'crit').length
  const warn = fresh.filter(x => x.state === 'warn').length

  return {
    tiles, open, fresh, handling, crit, warn, impactOf, rowAckKeys, topoLookup, topology, layout, synth,
    dataUpdatedAt, failed, groupFailed, oldestUpdate, stale, loading, pending, groupPending,
  }
}
