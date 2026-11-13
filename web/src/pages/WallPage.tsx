import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Maximize, Minimize, X } from 'lucide-react'
import { getAlertHistory } from '../api/alert'
import { heartbeat } from '../api/auth'
import { severityLabel } from '../api/rules'
import { cn } from '../utils/cn'
import { useFleet } from '../hooks/useFleet'
import TopologyMap from '../components/TopologyMap'
import StatusCounts from '../components/StatusCounts'
import { ago } from '../utils/time'
import { useClock } from '../hooks/useClock'
import ProblemList from '../components/ProblemList'
import GroupCards from '../components/GroupCards'
import { TONES, type Tone } from '../utils/severity'

const badge = (tone: Tone) => cn('shrink-0 rounded-md border px-1.5 py-0.5 text-xs font-bold', TONES[tone].bg, TONES[tone].text, TONES[tone].border)
const PANEL = 'rounded-xl border border-base-800 bg-base-900/50'
const BTN = 'flex items-center gap-2 px-3 py-1.5 rounded-lg bg-base-800 hover:bg-base-700 text-base-200 text-sm font-semibold border border-base-700'
export default function WallPage() {
  const now = useClock(1000)
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
    queryKey: ['wallHistory'], queryFn: () => getAlertHistory({ limit: 12 }), refetchInterval: 30_000,
  })
  const { tiles, open, fresh, handling, crit, warn, impactOf, rowAckKeys, topoLookup, topology, failed, groupFailed, oldestUpdate, stale: staleAt, pending, groupPending, layoutFailed, layout, groupStale } = useFleet()
  // 멈춘 화면을 실시간 상태로 믿지 않게: 조회 실패나 오래된 데이터는 머리글에 크게 표시
  const stale = staleAt(now)

  // 구성도가 칸보다 크면 맨 왼쪽이 아니라 가운데(보통 인터넷·코어 장비)부터 보이게
  const mapRef = useRef<HTMLElement>(null)
  // 장비 수가 바뀔 때만 다시 맞춤 — 기본 배치에서 저장된 구성도로 바뀔 때는 맞추고, 30초 재조회 때는 사용자가 옮긴 스크롤을 건드리지 않음
  const mapKey = `${layout?.devices.length ? 'saved' : 'auto'}-${topology.devices.length}`
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
        <div className="flex items-baseline gap-2 min-w-0">
          <span className="text-lg font-bold text-brand-300">OWLmon</span>
          <span className="text-sm text-base-500">상황판</span>
        </div>
        <StatusCounts crit={crit} warn={warn} handling={handling} loading={pending} unknown={stale} className="shrink-0" />
        <div className="min-w-0 flex-1">
          {stale && (
            <p role="alert" className="truncate text-base font-semibold text-base-100" title={failed.join(', ')}>
              <span className="mr-2 inline-block h-2.5 w-2.5 rounded-full bg-amber-400 animate-pulse align-middle" />
              {failed.length > 0 ? `데이터 갱신 중단 — ${failed.join('·')} 조회 실패` : '데이터 갱신 지연'}
              {oldestUpdate > 0 && ` (마지막 갱신 ${ago(new Date(oldestUpdate).toISOString(), now)})`}
            </p>
          )}
        </div>
        <div className="flex shrink-0 items-center gap-3">
          <div className="text-right tabular-nums mr-2">
            <div className="text-xl font-bold leading-none">{now.toTimeString().slice(0, 8)}</div>
            <div className="mt-1 text-xs text-base-500">
              {now.toLocaleDateString('ko-KR', { month: 'long', day: 'numeric', weekday: 'short' })}
              {oldestUpdate > 0 && `, ${ago(new Date(oldestUpdate).toISOString(), now)} 갱신`}
            </div>
          </div>
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

      <GroupCards tiles={tiles} groupFailed={groupFailed} groupPending={groupPending} />

      <div className="flex-1 min-h-0 grid grid-cols-1 xl:grid-cols-[minmax(0,1fr)_minmax(340px,26%)] gap-3">
        <main ref={mapRef} className={cn(PANEL, 'min-h-0 max-xl:h-[65vh] overflow-auto p-4')} aria-label="구성도">
          {layoutFailed && <p className="mb-2 text-sm text-base-400">구성도 설정을 불러오지 못해 자동 배치로 표시 중입니다 — 실제 배선과 다를 수 있습니다</p>}
          <TopologyMap layout={topology} lookup={name => {
            const t = topoLookup(name)
            // 조회 실패·갱신 지연 중엔 지난 "정상"을 초록으로 그리지 않음 — 위험·주의는 지난 값이라도 계속 보여 줌
            return t && t.state === 'ok' && (groupFailed(t.group) || groupStale(t.group, now))
              ? { ...t, state: 'wait', detail: groupFailed(t.group) ? '조회 실패 — 확인 불가' : '갱신 지연 — 확인 불가' } : t
          }} />
        </main>

        <aside className="min-h-0 flex flex-col gap-3 max-xl:min-h-[60vh]">
          <ProblemList
            open={open} fresh={fresh} handling={handling} crit={crit} warn={warn}
            impactOf={impactOf} rowAckKeys={rowAckKeys} failed={failed} stale={stale} pending={pending} now={now}
            canAck={false} compact className="min-h-0 max-h-[65%] shrink-0"
          />

          <section className={cn(PANEL, 'min-h-0 flex-1 flex flex-col overflow-hidden')}>
            <h2 className="px-4 py-2.5 border-b border-base-800 text-sm font-semibold text-base-200">최근 알림</h2>
            {history.length === 0 ? (
              <p className="px-4 py-3 text-sm text-base-500">최근 알림이 없습니다</p>
            ) : (
              <ol className="min-h-0 overflow-y-auto px-4 py-2.5 space-y-2">
                {history.map(r => (
                  <li key={r.id} className="flex items-center gap-2.5 text-sm">
                    <span className="w-14 shrink-0 whitespace-nowrap tabular-nums text-base-500">{ago(r.sent_at, now)}</span>
                    <span className={badge(r.severity === 'critical' ? 'rose' : r.severity === 'warning' ? 'amber' : 'slate')}>
                      {severityLabel(r.severity)}
                    </span>
                    <span className="min-w-0 text-base-300 truncate">{r.subject.replace(/^\[OWLmon\]\s*/, '')}</span>
                  </li>
                ))}
              </ol>
            )}
          </section>
        </aside>
      </div>
    </div>
  )
}
