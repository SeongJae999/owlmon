import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useHosts, useAllHostStatuses, useAllHostMetrics, useAlertStatus, useMaintenanceHosts, useAnomalyData, useAlertAcks, useHostFreshness, REFRESH_MS } from './useMonitoring'
import { serverToLocal } from '../utils/serverClock'
import { getAlertConfig, thresholds, type ActiveAlert, type AckEntry } from '../api/alert'
import { getSNMPStatus } from '../api/snmp'
import { getSyntheticStatus } from '../api/synthetic'
import { getSSLStatus } from '../api/ssl'
import { getDPMStatus } from '../api/dpm'
import { fetchAssets, locationText, type Asset } from '../api/asset'
import { hostLabel } from '../config/hostLabels'
import { defaultLayout, getTopologyLayout } from '../api/topology'

// 상황판과 메인 화면이 같은 기준(원인별 묶음, 확인된 문제 제외)으로 세도록 장비 상태 집계를 한곳에 둔다
export type State = 'crit' | 'warn' | 'maint' | 'ok' | 'wait'
export type Kind = '서버' | '네트워크 장비' | '웹사이트' | '데이터베이스'
export interface AckKey { host: string; category: string; severity: string }

export interface Tile {
  key: string
  kind: Kind
  name: string
  sub?: string
  ip?: string // IP 대역 자동 그룹용 — 서버는 에이전트가 보낸 관리 IP
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
  location?: string // 자산 관리의 위치 — 장애 장비를 찾아가거나 업체에 알려 줄 때
  rackPlaced?: boolean // 랙 실장도에 그려지는 장비 — 위치를 누르면 랙 화면으로 보냄
  since?: string
  // 마지막으로 실제 수집된 시각(브라우저 시계, ms)과 이보다 오래되면 '수집 지연'으로 볼 기준 — 화면을 받아 온 시각과 다름
  sampledAt?: number
  expectMs?: number
}

// 끊김·점검·첫 응답 전 장비는 수집이 멈춘 게 당연하니 지연으로 세지 않음
export const lagOf = (t: Tile, now: number) =>
  t.sampledAt == null || t.down || t.state === 'maint' || t.state === 'wait' ? null : Math.max(0, now - t.sampledAt)
export const isLagging = (t: Tile, now: number) => { const l = lagOf(t, now); return l != null && l > (t.expectMs ?? Infinity) }

// 실제 데이터 나이 — 화면을 받아 온 시각이 아니라 장비에서 수집된 시각 기준. 그룹으로 거른 목록에도 쓰도록 타일 목록을 받음.
// 가장 오래된 값은 수집 주기가 짧은 서버·네트워크 장비만 (웹사이트·DB는 주기가 분 단위라 섞으면 늘 느려 보임)
export function dataAge(tiles: Tile[], now: number) {
  let oldestMs: number | null = null
  let lagging = 0
  for (const x of tiles) {
    const l = lagOf(x, now)
    if (l == null) continue
    if ((x.kind === '서버' || x.kind === '네트워크 장비') && (oldestMs == null || l > oldestMs)) oldestMs = l
    if (l > (x.expectMs ?? Infinity)) lagging++
  }
  return { oldestMs, lagging }
}

export const RANK: Record<State, number> = { crit: 0, warn: 1, wait: 2, maint: 3, ok: 4 }
// '확인 중'은 누가 보고 있는 것처럼 읽혀 응답 없는 장비를 방치하게 함
export const LABEL: Record<State, string> = { crit: '위험', warn: '주의', maint: '점검 중', ok: '정상', wait: '응답 대기' }
export const DOT: Record<State, string> = { crit: 'bg-rose-500', warn: 'bg-amber-400', maint: 'bg-purple-400', ok: 'bg-emerald-400', wait: 'bg-base-500' }
export const KINDS: { name: Kind; unit: string; to: string }[] = [
  { name: '서버', unit: '대', to: '/?kind=서버' },
  { name: '네트워크 장비', unit: '대', to: '/snmp' },
  { name: '웹사이트', unit: '곳', to: '/website' },
  { name: '데이터베이스', unit: '개', to: '/dpm' },
]
// 가장 느린 조회 주기(1분)의 두 배 — 이보다 오래 갱신이 없으면 멈춘 화면으로 봄
export const STALE_MS = 2 * 60_000
// 기본값을 렌더마다 새 배열·객체로 만들면 useMemo가 매번 다시 계산함
const EMPTY: never[] = []
const NONE: Record<string, never> = {}

// 웹사이트 화면은 도메인 단위 카드 — 등록 도메인(대소문자·한글 그대로)과 감시 URL을 같은 호스트 표기로 맞춰 찾음
const hostOf = (url: string) => { try { return new URL(url).hostname } catch { return url } }

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
  const opts = { refetchInterval: REFRESH_MS, refetchIntervalInBackground: true }
  const qHosts = useHosts()
  const hosts = qHosts.data ?? EMPTY
  const qStatus = useAllHostStatuses(hosts)
  const hostStatuses = qStatus.data ?? NONE
  const qMetrics = useAllHostMetrics()
  const hostMetrics = qMetrics.data ?? NONE
  const qAlerts = useAlertStatus()
  const alerts = qAlerts.data ?? EMPTY
  const dataUpdatedAt = qAlerts.dataUpdatedAt
  const qMaint = useMaintenanceHosts()
  const maintenance = qMaint.data ?? EMPTY
  // 재조회 실패 땐 React Query가 지난 값을 들고 있음 — 이미 풀린 확인으로 새 장애를 "처리 중"으로 가리지 않게 버림.
  // 실패하면 모두 미확인(더 경고하는 쪽)으로 보이므로 화면 전체를 "확인 불가"로 만들진 않음
  const qAcks = useAlertAcks()
  const acks = qAcks.isError ? EMPTY : (qAcks.data ?? EMPTY)
  const { data: cfg } = useQuery({ queryKey: ['alertConfig'], queryFn: getAlertConfig })
  const qSnmp = useQuery({ queryKey: ['snmpStatus'], queryFn: getSNMPStatus, ...opts })
  const snmp = qSnmp.data
  const qFresh = useHostFreshness()
  const hostAge = qFresh.data
  const hostAgeAt = qFresh.dataUpdatedAt
  const snmpAt = qSnmp.dataUpdatedAt
  const qSynth = useQuery({ queryKey: ['syntheticStatus'], queryFn: getSyntheticStatus, ...opts })
  const synth = qSynth.data
  const qSsl = useQuery({ queryKey: ['sslStatus'], queryFn: getSSLStatus, ...opts })
  const ssl = qSsl.data
  const qDpm = useQuery({ queryKey: ['dpmStatus'], queryFn: getDPMStatus, ...opts })
  const dpm = qDpm.data

  // 배치는 장비 상태가 아니라 설정이라 전체 "갱신 중단"에 넣지 않고, 실패하면 구성도 쪽에만 자동 배치임을 알림
  const qLayout = useQuery({ queryKey: ['topologyLayout'], queryFn: getTopologyLayout, ...opts })
  const layout = qLayout.data
  // 조회가 실패해도 이전 값·빈 목록으로 그리면 "문제 없음"처럼 보인다 — 실패한 출처와 가장 오래된 갱신 시각을 함께 넘김
  const sources: { name: string; kind?: Kind; q: { isError: boolean; isPending: boolean; fetchStatus: string; dataUpdatedAt: number } }[] = [
    { name: '서버 목록', kind: '서버', q: qHosts }, { name: '서버 상태', kind: '서버', q: qStatus },
    { name: '서버 지표', kind: '서버', q: qMetrics }, { name: '알림', kind: '서버', q: qAlerts },
    { name: '점검 목록', kind: '서버', q: qMaint },
    // 서버 IP가 여기서 옴 — 못 받으면 IP 대역으로 묶인 서버가 그룹에서 조용히 빠지므로 실패·대기를 드러냄
    { name: '서버 수집 시각', kind: '서버', q: qFresh },
    { name: '네트워크 장비', kind: '네트워크 장비', q: qSnmp }, { name: '웹사이트', kind: '웹사이트', q: qSynth },
    { name: '보안 인증서', kind: '웹사이트', q: qSsl }, { name: '데이터베이스', kind: '데이터베이스', q: qDpm },
  ]
  // 첫 응답 전(오프라인으로 멈춘 경우 포함)이면 아직 판정할 수 없음 — "모두 정상"을 먼저 띄우지 않게.
  // 꺼 둔 쿼리(서버 0대일 때 상태 조회)는 idle이라 영구 "확인 중"이 되지 않음
  const waiting = (q: { isPending: boolean; fetchStatus: string }) => q.isPending && q.fetchStatus !== 'idle'
  const loading = waiting(qHosts)
  // 쿼리 결과 객체는 렌더마다 새로 오므로 쓰는 값만 모아 키로 삼아야 반환 함수 참조가 유지됨
  const srcKey = sources.map(x => `${+x.q.isError}${+x.q.isPending}${x.q.fetchStatus}${x.q.dataUpdatedAt}`).join()
  const src = useMemo(() => {
    const failed = sources.filter(x => x.q.isError).map(x => x.name)
    const kindFailed = (g: Kind) => sources.some(x => x.kind === g && x.q.isError)
    // 그 종류 출처만 보고 지연 판정 — 다른 종류(DB 등) 조회가 멈춘 걸로 서버·네트워크까지 회색으로 만들지 않게
    const kindStale = (g: Kind, now: Date) => sources.some(x => x.kind === g && x.q.dataUpdatedAt > 0 && now.getTime() - x.q.dataUpdatedAt > STALE_MS)
    const times = sources.map(x => x.q.dataUpdatedAt).filter(t => t > 0)
    const oldestUpdate = times.length ? Math.min(...times) : 0
    const stale = (now: Date) => failed.length > 0 || (oldestUpdate > 0 && now.getTime() - oldestUpdate > STALE_MS)
    const pending = sources.some(x => waiting(x.q))
    const kindPending = (g: Kind) => sources.some(x => x.kind === g && waiting(x.q))
    return { failed, kindFailed, kindStale, oldestUpdate, stale, pending, kindPending }
  }, [srcKey]) // eslint-disable-line react-hooks/exhaustive-deps
  // 한 번도 못 받은 채 실패한 경우도 모름 — 빈 목록으로 판정하면 위험 알림이 걸린 서버가 구성도에 초록으로 그려짐
  const unknown = (q: { isPending: boolean; fetchStatus: string; isError: boolean; data?: unknown }) => waiting(q) || (q.isError && q.data === undefined)
  const alertsUnknown = unknown(qAlerts) || unknown(qMaint)

  const { data: anomaly } = useAnomalyData()
  const assets: Asset[] = useQuery({ queryKey: ['assets'], queryFn: fetchAssets, staleTime: 5 * 60_000 }).data ?? EMPTY

  const fleet = useMemo(() => {
    const assetOf = new Map<string, Asset>()
    for (const a of assets) if (!assetOf.has(a.host_name)) assetOf.set(a.host_name, a)
    // 장비 이름만으로는 무슨 장비인지 모르니 장비 대장의 설명을 용도로 붙인다
    const purposeOf = (name: string) => assetOf.get(name)?.description || hostLabel(name)
    const ackBy = new Map<string, AckEntry>()
    for (const a of acks) {
      const k = `${a.host}/${a.category}/${a.severity}`
      if (!ackBy.has(k)) ackBy.set(k, a)
    }
    const ackOf = (k: AckKey) => ackBy.get(`${k.host}/${k.category}/${k.severity}`)
    const alertsOf = new Map<string, ActiveAlert[]>()
    for (const a of alerts) {
      if (a.in_maintenance) continue
      const l = alertsOf.get(a.host)
      if (l) l.push(a)
      else alertsOf.set(a.host, [a])
    }

    const t = thresholds(cfg)
    const maint = new Set(maintenance)
    const tiles: Tile[] = []

    const fullIn = new Map<string, number>()
    for (const p of anomaly?.disk_predictions ?? []) {
      if (p.days_left >= 0) fullIn.set(p.host, Math.min(fullIn.get(p.host) ?? Infinity, p.days_left))
    }

    for (const h of hosts) {
      const mine: ActiveAlert[] = alertsOf.get(h) ?? EMPTY
      const unacked = mine.filter(a => !a.acked)
      const m = hostMetrics[h]
      // 알림·점검 목록을 받기 전엔 끊김만 확실하고 나머지는 모름
      const state = alertsUnknown && hostStatuses[h] !== 'offline' ? 'wait' : serverState(hostStatuses[h], mine, maint.has(h))
      const since = mine.map(a => a.since).filter((x): x is string => !!x).sort()[0]
      // 미확인 알림을 먼저 보여 줌 — 확인한 알림 문구가 새 문제를 가리지 않게
      const lead = unacked[0] ?? mine[0]
      const more = mine.length > 1 ? ` 외 ${mine.length - 1}건` : ''
      tiles.push({
        key: `h-${h}`, kind: '서버', name: h, sub: hostAge?.[h]?.ip, ip: hostAge?.[h]?.ip, purpose: purposeOf(h), state, since,
        // 수집 주기의 3배 + 전송 여유, 최소 60초 — 주기를 늘린 에이전트(OWLMON_COLLECT_INTERVAL)가 늘 지연으로 보이지 않게
        sampledAt: hostAge?.[h] ? hostAgeAt - hostAge[h].age_sec * 1000 : undefined,
        expectMs: Math.max(60_000, (hostAge?.[h]?.interval_sec ?? 15) * 3000 + 15_000),
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
      const down = !!d.CollectedAt && !d.Up && !d.NoData
      const key = { host: d.Device.Name, category: 'snmp', severity: 'critical' }
      // 이번 장애가 시작된 뒤의 확인만 인정 — 서버도 같은 기준(DownSince 이전 확인만 해제)으로 알림을 거름
      const a = down ? ackOf(key) : undefined
      const ackValid = a && (!d.DownSince || a.acked_at >= d.DownSince)
      tiles.push({
        key: `n-${d.Device.ID}`, kind: '네트워크 장비', name: d.Device.Name, sub: d.Device.IP, ip: d.Device.IP, purpose: purposeOf(d.Device.Name),
        // 폴링 30초 — 그 3배
        sampledAt: d.DataAgeSec != null ? snmpAt - d.DataAgeSec * 1000 : undefined, expectMs: 90_000,
        state: !d.CollectedAt || (!d.Up && d.NoData) ? 'wait' : d.Up ? 'ok' : 'crit',
        down,
        since: d.Up || d.NoData ? undefined : d.DownSince,
        detail: !d.CollectedAt ? '첫 응답 대기' : d.Up ? `응답 ${d.ResponseMs ?? 0}ms` : d.NoData ? '수집 끊김 — 담당 에이전트 확인' : '응답 없음',
        acked: ackValid ? { by: a.acked_by, at: a.acked_at } : undefined,
        ackKeys: down && !ackValid ? [key] : undefined,
        to: `/snmp?focus=${d.Device.ID}`,
      })
    }
    for (const s of (synth ?? []).filter(s => s.monitor.enabled)) {
      const ok = s.latest?.success
      const failing = !!s.latest && !ok
      const key = { host: s.monitor.name, category: 'synthetic', severity: 'critical' }
      const a = failing ? ackOf(key) : undefined
      tiles.push({
        key: `w-${s.monitor.id}`, kind: '웹사이트', name: s.monitor.name, sub: s.monitor.url.replace(/^https?:\/\//, ''), ip: hostOf(s.monitor.url),
        // 점검 시각은 서버 시계 — 관제 PC 시계 차이를 빼서 비교. 응답이 늦은 점검은 시간 제한만큼 늦게 끝남
        sampledAt: s.latest ? serverToLocal(Date.parse(s.latest.checked_at)) : undefined,
        expectMs: Math.max(s.monitor.interval_seconds, s.monitor.timeout_seconds) * 2000 + 30_000,
        state: !s.latest ? 'wait' : ok ? (s.latest.response_time_ms > 3000 ? 'warn' : 'ok') : 'crit',
        detail: !s.latest ? '첫 점검 대기' : ok
          ? `응답 ${s.latest.response_time_ms}ms${s.latest.response_time_ms > 3000 ? ' (느림)' : ''}`
          : (s.latest.error || `HTTP ${s.latest.status_code}`),
        // 느림(주의)은 메일도 해제 경로도 없어 확인 대상이 아님 — 실패일 때만
        acked: a ? { by: a.acked_by, at: a.acked_at } : undefined,
        ackKeys: failing && !a ? [key] : undefined,
        to: `/website?focus=${encodeURIComponent(hostOf(s.monitor.url))}`,
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
        key: `s-${c.domain}:${c.port}`, kind: '웹사이트', name: c.domain, sub: '보안 인증서',
        state: crit ? 'crit' : 'ok',
        detail: c.status === 'error' ? '인증서 확인 실패' : c.days_left < 0 ? '인증서 만료' : `인증서 ${c.days_left}일 남음`,
        acked: a ? { by: a.acked_by, at: a.acked_at } : undefined,
        ackKeys: ackable && !a ? [key] : undefined,
        to: `/website?focus=${encodeURIComponent(hostOf(`https://${c.domain}/`))}`,
      })
    }
    for (const d of dpm ?? []) {
      const m = d.metrics
      const key = { host: d.instance.name, category: 'dpm', severity: 'critical' }
      const a = m?.error ? ackOf(key) : undefined
      tiles.push({
        key: `d-${d.instance.id}`, kind: '데이터베이스', name: d.instance.name, sub: d.instance.db_type, ip: d.instance.host,
        sampledAt: m ? serverToLocal(Date.parse(m.collected_at)) : undefined, expectMs: Math.max(30, d.instance.poll_interval_sec) * 2000 + 30_000,
        state: !m ? 'wait' : m.error ? 'crit' : 'ok',
        detail: !m ? '첫 응답 대기' : m.error ? '연결 실패' : `연결 ${m.connections_active}/${m.connections_max}`,
        acked: a ? { by: a.acked_by, at: a.acked_at } : undefined,
        ackKeys: m?.error && !a ? [key] : undefined,
        to: `/dpm?focus=${d.instance.id}`,
      })
    }
    for (const x of tiles) {
      const a = assetOf.get(x.name)
      x.location = locationText(a) || undefined
      x.rackPlaced = !!a?.rack && a.rack_unit > 0
    }
    const KORDER = KINDS.map(g => g.name)
    // 문제 있는 타일을 맨 앞에, 나머지는 종류별로 묶어서
    tiles.sort((a, b) => Math.min(RANK[a.state], 2) - Math.min(RANK[b.state], 2) || KORDER.indexOf(a.kind) - KORDER.indexOf(b.kind) || a.name.localeCompare(b.name))

    // 위쪽 장비가 끊겨 함께 끊긴 장비는 따로 세지 않고 원인 장비 아래로 묶는다
    const topology = layout?.devices.length ? layout : defaultLayout(hosts, (snmp ?? []).map(d => d.Device.Name))
    const parentOf = new Map(topology.devices.map(d => [d.name, d.parent]))
    // 짝은 한쪽에만 적혀 있어도 서로 짝
    const pairOf = new Map<string, string>()
    for (const d of topology.devices) if (d.pair) { pairOf.set(d.name, d.pair); if (!pairOf.has(d.pair)) pairOf.set(d.pair, d.name) }
    // 구성도엔 서버와 네트워크 장비만 있음 — 같은 이름의 웹사이트·인증서 타일과 섞이지 않게 그 둘만 본다
    const topoTiles = tiles.filter(x => x.kind === '서버' || x.kind === '네트워크 장비')
    // 점검 중인 장비는 목록에 안 나오므로 원인으로 묶으면 하위 장비 장애가 아무 데도 안 보임
    const downNames = new Set(topoTiles.filter(x => x.down && x.state !== 'maint').map(x => x.name))
    // 점검 중 장비는 점검으로만 보여야 함 — 상위 영향으로 묶으면 점검과 영향에 두 번 세어짐
    const caused = new Map<string, Tile[]>()
    for (const x of topoTiles.filter(t => t.down && t.state !== 'maint')) {
      for (let p = parentOf.get(x.name), hops = 0; p && hops < 50; p = parentOf.get(p), hops++) {
        // 이중화 짝이 살아 있으면 그 장비가 끊겨도 아래는 이어져 있음 — 서버 알림 묶음(suppress.go causeOf)과 같은 기준
        const mate = pairOf.get(p)
        if (downNames.has(p) && (!mate || downNames.has(mate))) x.causedBy = p
      }
      if (x.causedBy) x.detail = `${x.causedBy} 장애 영향`
    }
    const topoByName = new Map<string, Tile>()
    for (const x of topoTiles) {
      if (!topoByName.has(x.name)) topoByName.set(x.name, x)
      if (!x.causedBy) continue
      const l = caused.get(x.causedBy)
      if (l) l.push(x)
      else caused.set(x.causedBy, [x])
    }
    // 원인 장비를 확인한 뒤 함께 끊긴 하위 장비 알림이 새로 생기면 원인 행을 다시 미확인으로 올려 확인 버튼을 살림
    for (const x of topoTiles) {
      if (x.acked && caused.get(x.name)?.some(c => c.ackKeys?.length)) x.acked = undefined
    }
    const impactOf = (name: string) => (caused.get(name) ?? []).map(x => x.name)
    // 원인 장비 한 행을 확인하면 함께 끊긴 하위 장비의 끊김 메일도 멈춰야 관제사 기대와 맞음
    const rowAckKeys = (x: Tile): AckKey[] => [
      ...(x.ackKeys ?? []),
      ...(caused.get(x.name) ?? []).flatMap(c => c.ackKeys ?? []),
    ]
    const topoLookup = (name: string) => topoByName.get(name)

    // 서버 알림만이 아니라 네트워크 장비, 웹사이트까지 위 합계와 같은 기준으로
    // 담당자가 확인한 문제는 목록 아래로 내리고 합계에서 빼서, 아직 아무도 안 본 문제가 먼저 보이게
    // 무엇부터 볼지 순서: 미확인 → 위험 → 하위 영향이 큰 원인 장비 → 최근 발생. 이름순이면 원인 스위치가 서버 행 뒤로 밀려 개별 장애로 오판함
    const sinceMs = (x: Tile) => x.since ? Date.parse(x.since) || 0 : 0
    const open = tiles.filter(x => (x.state === 'crit' || x.state === 'warn') && !x.causedBy)
      .sort((a, b) => Number(!!a.acked) - Number(!!b.acked)
        || Number(a.state !== 'crit') - Number(b.state !== 'crit')
        || (caused.get(b.name)?.length ?? 0) - (caused.get(a.name)?.length ?? 0)
        || sinceMs(b) - sinceMs(a))
    const fresh = open.filter(x => !x.acked)
    const handling = open.length - fresh.length
    const crit = fresh.filter(x => x.state === 'crit').length
    const warn = fresh.filter(x => x.state === 'warn').length
    return { tiles, open, fresh, handling, crit, warn, impactOf, rowAckKeys, topoLookup, topology }
  }, [hosts, hostStatuses, hostMetrics, alerts, maintenance, acks, cfg, snmp, snmpAt, hostAge, hostAgeAt, synth, ssl, dpm, layout, anomaly, assets, alertsUnknown])

  return {
    ...fleet, ...src, layout, synth, dataUpdatedAt, loading, layoutFailed: qLayout.isError && !qLayout.data?.devices.length,
  }
}
