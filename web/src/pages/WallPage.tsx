import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Maximize, Minimize, X } from 'lucide-react'
import { heartbeat } from '../api/auth'
import { alertTitle, getAlertHistory } from '../api/alert'
import { cn } from '../utils/cn'
import { useFleet, type Tile } from '../hooks/useFleet'
import DataAge from '../components/DataAge'
import { useGroupScope } from '../hooks/useGroupScope'
import { UNGROUPED } from '../api/groups'
import TopologyMap from '../components/TopologyMap'
import StatusCounts from '../components/StatusCounts'
import { ago } from '../utils/time'
import { useClock } from '../hooks/useClock'
import ProblemList from '../components/ProblemList'
import GroupCards from '../components/GroupCards'

const PANEL = 'rounded-xl border border-base-800 bg-base-900/50'
// 장비 상태(타일)로 이미 처리할 문제에 나오는 알림 — 나머지(로그 규칙·서비스·이상탐지·감시 시스템·인증서 30일 전)는 상황판에 다른 자리가 없음
const TILE_CATEGORIES = new Set(['cpu', 'memory', 'disk', 'down', 'snmp', 'synthetic', 'dpm'])
const isTileAlert = (category: string, severity: string) => TILE_CATEGORIES.has(category) || (category === 'ssl' && severity === 'critical')
const BTN = 'flex items-center gap-2 px-3 py-1.5 rounded-lg bg-base-800 hover:bg-base-700 text-base-200 text-sm font-semibold border border-base-700'
// 1초 시계만 따로 그려서 매초 화면 전체(구성도·목록)를 다시 그리지 않게
// 갱신 표시는 화면을 받아 온 시각이 아니라 장비에서 실제 수집된 시각 — 수집이 밀려도 '방금 갱신'으로 보이지 않게
function Clock({ tiles }: { tiles: Tile[] }) {
  const now = useClock(1000)
  return (
    <div className="text-right tabular-nums mr-2">
      <div className="text-xl font-bold leading-none">{now.toTimeString().slice(0, 8)}</div>
      <div className="mt-1 flex items-center justify-end gap-2 text-xs text-base-500">
        {now.toLocaleDateString('ko-KR', { month: 'long', day: 'numeric', weekday: 'short' })}
        <DataAge tiles={tiles} now={now.getTime()} className="text-base-500" />
      </div>
    </div>
  )
}

export default function WallPage() {
  const now = useClock(10_000) // 갱신 지연(2분) 표시가 늦게 켜지지 않을 만큼만 — 1초면 구성도까지 매초 다시 그림
  const [full, setFull] = useState(false)

  // 상황판은 벽 모니터용이라 사용자 테마와 상관없이 어둡게
  useEffect(() => {
    const root = document.documentElement
    const prev = root.dataset.theme
    root.dataset.theme = 'dark'
    const onFs = () => setFull(!!document.fullscreenElement)
    document.addEventListener('fullscreenchange', onFs)
    return () => {
      if (prev) root.dataset.theme = prev
      document.removeEventListener('fullscreenchange', onFs)
    }
  }, [])

  const toggleFull = () => {
    if (document.fullscreenElement) document.exitFullscreen()
    else document.documentElement.requestFullscreen?.()
  }

  const { data: history = [] } = useQuery({
    queryKey: ['wallOtherAlerts'],
    queryFn: () => getAlertHistory({ from: new Date(Date.now() - 86_400_000).toISOString(), limit: 200 }),
    refetchInterval: 30_000,
  })
  const fleet = useFleet()
  const { impactOf, rowAckKeys, topoLookup, failed, kindFailed, oldestUpdate, stale: staleAt, pending, kindPending, layoutFailed, layout, kindStale } = fleet
  // 그룹(?g=)을 고르면 지점·층 전용 상황판 — 숫자·문제·구성도를 그 그룹으로
  const scope = useGroupScope(fleet.tiles, fleet.open, fleet.topoLookup)
  const { tiles, open, handling, crit, warn } = scope
  // 구성도는 그룹 장비와 그 위쪽 연결 장비만 — 끊긴 원인 장비가 그룹 밖(코어 스위치 등)이어도 선이 이어져 보이게
  const topology = useMemo(() => {
    if (!scope.selected) return fleet.topology
    const parentOf = new Map(fleet.topology.devices.map(d => [d.name, d.parent]))
    const keep = new Set<string>()
    for (const t of scope.tiles) {
      for (let n: string | undefined = t.name, hops = 0; n && !keep.has(n) && hops < 50; n = parentOf.get(n), hops++) keep.add(n)
    }
    const devices = fleet.topology.devices.filter(d => keep.has(d.name))
    const zones = fleet.topology.zones.filter(z => devices.some(d => d.zone === z))
    return { ...fleet.topology, zones, devices }
  }, [fleet.topology, scope.selected, scope.tiles])
  // 그룹을 고르면 그 그룹 장비 알림만. 감시 시스템 자체 알림(host owlmon)은 모든 상황판에
  const otherAlerts = useMemo(() => {
    const names = scope.selected ? new Set(tiles.map(t => t.name)) : null
    // 복구 통보는 빨간 점으로 보이면 새 장애로 읽힘
    return history.filter(r => r.severity !== 'info' && !r.subject.startsWith('[복구]') && !isTileAlert(r.category, r.severity)
      && (!names || r.host === 'owlmon' || names.has(r.host))).slice(0, 5)
  }, [history, tiles, scope.selected])
  // 멈춘 화면을 실시간 상태로 믿지 않게: 조회 실패나 오래된 데이터는 머리글에 크게 표시
  const stale = staleAt(now)


  const lookup = useCallback((name: string) => {
    const t = topoLookup(name)
    // 조회 실패·갱신 지연 중엔 지난 "정상"을 초록으로 그리지 않음 — 위험·주의는 지난 값이라도 계속 보여 줌
    return t && t.state === 'ok' && (kindFailed(t.kind) || kindStale(t.kind, now))
      ? { ...t, state: 'wait' as const, detail: kindFailed(t.kind) ? '조회 실패 — 확인 불가' : '갱신 지연 — 확인 불가' } : t
  }, [topoLookup, kindFailed, kindStale, now])

  // 벽 모니터는 아무도 스크롤하지 않음 — 정상 장비 수백 대를 늘어놓으면 아래쪽 위험 장비가 화면 밖에 묻힘.
  // 문제 장비(확인 불가 포함)와 그 위쪽 연결 경로, 이중화 짝만 그리고 나머지 정상 장비는 대수로만 알림
  const shown = useMemo(() => {
    const parentOf = new Map(topology.devices.map(d => [d.name, d.parent]))
    const pairOf = new Map<string, string>()
    for (const d of topology.devices) if (d.pair) { pairOf.set(d.name, d.pair); if (!pairOf.has(d.pair)) pairOf.set(d.pair, d.name) }
    const keep = new Set<string>()
    const up = (from: string | undefined) => {
      for (let n = from, hops = 0; n && !keep.has(n) && hops < 50; n = parentOf.get(n), hops++) keep.add(n)
    }
    for (const d of topology.devices) {
      const t = topoLookup(d.name)
      // 확인 불가는 그 장비만 응답이 없을 때만 — 첫 조회 중이거나 종류 전체 조회가 실패하면 모든 장비가 확인 불가라 구성도 전체가 다시 깔림(실패는 머리글에 표시)
      const unknown = t?.state === 'wait' && !kindPending(t.kind) && !kindFailed(t.kind)
      if (t && (t.state === 'crit' || t.state === 'warn' || unknown)) { up(d.name); up(pairOf.get(d.name)) }
    }
    const devices = topology.devices.filter(d => keep.has(d.name))
    const zones = topology.zones.filter(z => devices.some(d => d.zone === z))
    const hidden = topology.devices.filter(d => d.kind !== 'internet' && !keep.has(d.name)).length
    return { layout: { ...topology, zones, devices }, hidden }
  }, [topology, topoLookup, kindPending, kindFailed])

  // 구성도가 칸보다 크면 맨 왼쪽이 아니라 가운데(보통 인터넷·코어 장비)부터 보이게
  const mapRef = useRef<HTMLElement>(null)
  // 장비 수가 바뀔 때만 다시 맞춤 — 기본 배치에서 저장된 구성도로 바뀔 때는 맞추고, 30초 재조회 때는 사용자가 옮긴 스크롤을 건드리지 않음
  const mapKey = `${layout?.devices.length ? 'saved' : 'auto'}-${shown.layout.devices.length}`
  useEffect(() => {
    const el = mapRef.current
    if (el) el.scrollLeft = (el.scrollWidth - el.clientWidth) / 2
  }, [mapKey])

  // 상황판은 레이아웃 밖이라 세션 유지 신호를 따로 보냄 — 조회 전용 계정은 이걸로 토큰이 연장돼 벽 모니터가 로그아웃되지 않음
  useEffect(() => {
    const beat = () => heartbeat(false).catch(() => {}) // 일시 오류로 화면을 끊지 않음 (만료되면 401 처리)
    beat()
    const id = setInterval(beat, 60_000)
    return () => clearInterval(id)
  }, [])

  return (
    // 벽 모니터(넓은 화면)에선 한 화면 고정, 좁은 화면에선 문제 목록을 구성도 아래로 내리고 페이지 스크롤
    <div className="min-h-dvh xl:h-dvh xl:overflow-hidden bg-base-950 text-base-100 flex flex-col gap-3 p-3">
      <header className={cn(PANEL, 'flex flex-wrap items-center gap-x-5 gap-y-2 px-4 py-2')}>
        {/* 지점마다 벽 모니터를 두면 어느 그룹 숫자인지가 먼저 보여야 함 — 그룹 이름을 제목으로 크게 */}
        <div className="flex items-center gap-3 min-w-0">
          <div className="min-w-0">
            <div className="text-xs text-base-500"><span className="font-bold text-brand-300">OWLmon</span> 상황판</div>
            <h1 className="truncate text-2xl font-bold leading-tight text-base-50">{scope.label}</h1>
          </div>
          {/* 전체 화면(벽 모니터)에선 고를 일이 없음 — 설정할 때만 */}
          {scope.idx.byId.size > 0 && !full && (
            <select aria-label="그룹" value={scope.selected} onChange={e => scope.select(e.target.value)}
              className="h-8 max-w-[16rem] rounded-lg border border-base-700 bg-base-800 px-2 text-sm text-base-100">
              <option value="">전체</option>
              {scope.options.map(o => <option key={o.id} value={o.id}>{o.label}</option>)}
              <option value={UNGROUPED}>미지정</option>
            </select>
          )}
        </div>
        <StatusCounts crit={crit} warn={warn} handling={handling} loading={pending} unknown={stale} className="shrink-0 text-xl" />
        <div className="min-w-0 flex-1">
          {scope.unresolved && (
            <p role="status" className="truncate text-sm font-semibold text-base-200">
              <span className="mr-2 inline-block h-2.5 w-2.5 rounded-full bg-amber-400 align-middle" />
              {scope.unresolved === 'loading' ? '그룹 정보를 불러오는 중 — 잠시 전체를 표시합니다' : '그룹 정보를 불러오지 못했거나 없는 그룹 — 전체를 표시 중'}
            </p>
          )}
          {stale && (
            <p role="alert" className="truncate text-base font-semibold text-base-100" title={failed.join(', ')}>
              <span className="mr-2 inline-block h-2.5 w-2.5 rounded-full bg-amber-400 animate-pulse align-middle" />
              {failed.length > 0 ? `데이터 갱신 중단 — ${failed.join('·')} 조회 실패` : '데이터 갱신 지연'}
              {oldestUpdate > 0 && ` (마지막 갱신 ${ago(new Date(oldestUpdate).toISOString(), now)})`}
            </p>
          )}
        </div>
        <div className="flex shrink-0 items-center gap-3">
          <Clock tiles={tiles} />
          <button type="button" onClick={toggleFull} className={BTN}>
            {full ? <Minimize size={15} /> : <Maximize size={15} />}
            {full ? '전체 화면 끝내기' : '전체 화면'}
          </button>
          {!full && (
            <Link to="/" aria-label="상황판 나가기" className="p-2 rounded-lg text-base-400 hover:text-base-100 hover:bg-base-800">
              <X size={18} />
            </Link>
          )}
        </div>
      </header>

      <GroupCards tiles={tiles} kindFailed={kindFailed} kindPending={kindPending} large now={now.getTime()} />

      <div className="flex-1 min-h-0 grid grid-cols-1 xl:grid-cols-[minmax(0,1fr)_minmax(340px,26%)] gap-3">
        <main ref={mapRef} className={cn(PANEL, 'min-h-0 max-xl:h-[65vh] overflow-auto p-4')} aria-label="구성도">
          {layoutFailed ? <p className="mb-2 text-sm text-base-400">구성도 설정을 불러오지 못해 자동 배치로 표시 중입니다 — 실제 배선과 다를 수 있습니다</p>
            // 저장된 구성도가 없으면 상위 장비를 몰라 하위 장비 장애가 원인 장비 아래로 묶이지 않음 — 수십 건이 따로 보이는 이유를 알려 줌
            : !layout?.devices.length && <p className="mb-2 text-sm text-base-400">구성도 미저장 — 장비 연결을 몰라 원인 장비 묶음이 꺼져 있습니다</p>}
          <p className="mb-2 text-sm text-base-500 tabular-nums">
            {/* 숨긴 장비를 '정상'이라 단정하지 않음 — 조회 실패·점검 중·구성도에 없는 장비가 섞여 있음 */}
            {pending ? '불러오는 중…'
              : shown.layout.devices.length > 0 ? `문제 장비와 연결 경로만 표시 · 그 밖의 ${shown.hidden}대 숨김`
              : stale ? `확인 불가 — 구성도 장비 ${shown.hidden}대 숨김`
              : `구성도에 문제 장비 없음 · ${shown.hidden}대 숨김`}
          </p>
          {shown.layout.devices.length > 0 && <TopologyMap layout={shown.layout} lookup={lookup} />}
        </main>

        <aside className="min-h-0 flex flex-col gap-3 max-xl:min-h-[60vh]">
          <ProblemList
            open={open}
            impactOf={impactOf} rowAckKeys={rowAckKeys} failed={failed} stale={stale} pending={pending} now={now}
            canAck={false} compact large className="min-h-0 flex-1"
          />
          {/* 장비 상태로 잡히지 않는 알림만 — 장비 알림까지 두면 처리할 문제와 같은 장애가 두 번 보임 */}
          {otherAlerts.length > 0 && (
            <section className={cn(PANEL, 'shrink-0')} aria-label="그 밖의 알림">
              <h2 className="px-4 py-2 border-b border-base-800 text-sm font-semibold text-base-200">그 밖의 알림 <span className="font-normal text-base-500">· 24시간</span></h2>
              <ol className="px-4 py-2 space-y-1.5">
                {otherAlerts.map(r => (
                  <li key={r.id} className="flex items-center gap-2.5 text-sm">
                    <span className={cn('h-2.5 w-2.5 shrink-0 rounded-full', r.severity === 'critical' ? 'bg-rose-500' : 'bg-amber-400')} />
                    <span className="min-w-0 flex-1 truncate text-base-200" title={r.subject}>{alertTitle(r.subject)}</span>
                    <span className="shrink-0 tabular-nums text-base-500">{ago(r.sent_at, now)}</span>
                  </li>
                ))}
              </ol>
            </section>
          )}
        </aside>
      </div>
    </div>
  )
}
