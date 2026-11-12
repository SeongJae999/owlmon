import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Maximize, Minimize, Pause, Repeat, X } from 'lucide-react'
import { getAlertHistory } from '../api/alert'
import { heartbeat } from '../api/auth'
import { severityLabel } from '../api/rules'
import { cn } from '../utils/cn'
import { useFleet, GROUPS, LABEL, type State } from '../hooks/useFleet'
import TopologyMap from '../components/TopologyMap'
import StatusCounts from '../components/StatusCounts'
import { ago, lasting } from '../utils/time'
import { useClock } from '../hooks/useClock'
import ProblemList from '../components/ProblemList'
import GroupCards from '../components/GroupCards'
import { TONES, type Tone } from '../utils/severity'

const ROTATE_MS = 30_000
const AUTO_KEY = 'owlmon_wall_auto'

const TONE: Record<State, Tone> = { crit: 'rose', warn: 'amber', maint: 'slate', ok: 'emerald', wait: 'slate' }
const badge = (tone: Tone) => cn('shrink-0 rounded-md border px-1.5 py-0.5 text-xs font-bold', TONES[tone].bg, TONES[tone].text, TONES[tone].border)
const PANEL = 'rounded-xl border border-base-800 bg-base-900/50'
const BTN = 'flex items-center gap-2 px-3 py-1.5 rounded-lg bg-base-800 hover:bg-base-700 text-base-200 text-sm font-semibold border border-base-700'


export default function WallPage() {
  const now = useClock(1000)
  const [params, setParams] = useSearchParams()
  const view = params.get('view') === 'map' ? 'map' : 'tiles'
  const [full, setFull] = useState(false)
  const [auto, setAuto] = useState(() => {
    try { return localStorage.getItem(AUTO_KEY) !== 'off' } catch { return true }
  })

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
  const { tiles, open, fresh, handling, crit, warn, impactOf, rowAckKeys, topoLookup, topology, layout, synth, failed, groupFailed, oldestUpdate, stale: staleAt, pending, groupPending } = useFleet()
  // 멈춘 화면을 실시간 상태로 믿지 않게: 조회 실패나 오래된 데이터는 머리글에 크게 표시
  const stale = staleAt(now)

  // 상황판은 레이아웃 밖이라 세션 유지 신호를 따로 보냄 — 조회 전용 계정은 이걸로 토큰이 연장돼 벽 모니터가 로그아웃되지 않음
  useEffect(() => {
    const beat = () => heartbeat(false).catch(() => {}) // 일시 오류로 화면을 끊지 않음 (만료되면 401 처리)
    beat()
    const id = setInterval(beat, 60_000)
    return () => clearInterval(id)
  }, [])

  // AI 문장 대신 정해진 틀로 만든 한 줄 요약: 숫자와 상태가 화면 다른 곳과 어긋나지 않게
  const zoneOf = new Map((layout?.devices ?? []).map(d => [d.name, d.zone]))
  const summary = fresh.length
    ? fresh.slice(0, 3).map(x => {
        const n = impactOf(x.name).length
        const zone = zoneOf.get(x.name)
        // 용도가 있으면 구역보다 뜻이 잘 통하고 문장도 짧아진다
        const where = x.purpose ? `${x.name}(${x.purpose})` : `${zone ? `${zone} ` : ''}${x.name}`
        return `${where} ${x.detail.split(' · ')[0] || LABEL[x.state]}${n ? `, 하위 ${n}대 영향` : ''}`
      }).join(', ') + (fresh.length > 3 ? ` 외 ${fresh.length - 3}건` : '')
    : handling > 0 ? `${handling}건은 담당자가 확인하고 처리하고 있습니다` : ''

  // 벽 모니터 자동 전환: 아무도 확인하지 않은 위험이 있으면 구성도에 고정
  const pinMap = auto && crit > 0
  useEffect(() => {
    try { localStorage.setItem(AUTO_KEY, auto ? 'on' : 'off') } catch { /* 저장 못 해도 이번 화면에선 동작 */ }
  }, [auto])
  useEffect(() => {
    if (!auto) return
    if (pinMap) {
      if (view !== 'map') setParams({ view: 'map' }, { replace: true })
      return
    }
    const id = setInterval(() => setParams(p => (p.get('view') === 'map' ? {} : { view: 'map' }) as Record<string, string>, { replace: true }), ROTATE_MS)
    return () => clearInterval(id)
  }, [auto, pinMap, view, setParams])

  const problems = tiles.filter(x => x.state === 'crit' || x.state === 'warn')
  const calm = tiles.filter(x => x.state !== 'crit' && x.state !== 'warn')
  const services = (synth ?? []).filter(s => s.monitor.enabled && s.stats.total_checks > 0)

  return (
    <div className="h-dvh overflow-hidden bg-base-950 text-base-100 flex flex-col gap-4 p-4">
      <header className={cn(PANEL, 'flex items-center gap-5 px-5 py-3')}>
        <div className="flex items-baseline gap-2 min-w-0">
          <span className="text-lg font-bold text-brand-300">OWLmon</span>
          <span className="text-sm text-base-500">상황판</span>
        </div>
        <StatusCounts crit={crit} warn={warn} handling={handling} loading={pending} unknown={stale} className="shrink-0 text-lg" />
        {stale ? (
          <p role="alert" className="min-w-0 flex-1 truncate text-base font-semibold text-base-100" title={failed.join(', ')}>
            <span className="mr-2 inline-block h-2.5 w-2.5 rounded-full bg-amber-400 animate-pulse align-middle" />
            {failed.length > 0 ? `데이터 갱신 중단 — ${failed.join('·')} 조회 실패` : '데이터 갱신 지연'}
            {oldestUpdate > 0 && ` (마지막 갱신 ${ago(new Date(oldestUpdate).toISOString(), now)})`}
          </p>
        ) : (
          <p className="min-w-0 flex-1 truncate text-base text-base-300" title={summary}>{summary}</p>
        )}
        <div className="ml-auto flex shrink-0 items-center gap-3">
          <div className="text-right tabular-nums mr-2">
            <div className="text-2xl font-bold leading-none">{now.toTimeString().slice(0, 8)}</div>
            <div className="mt-1 text-xs text-base-500">
              {now.toLocaleDateString('ko-KR', { month: 'long', day: 'numeric', weekday: 'short' })}
              {oldestUpdate > 0 && `, ${ago(new Date(oldestUpdate).toISOString(), now)} 갱신`}
            </div>
          </div>
          <button
            type="button" onClick={() => setAuto(a => !a)} aria-pressed={auto}
            title={pinMap ? '확인하지 않은 위험이 있어 구성도에 고정했습니다' : `${ROTATE_MS / 1000}초마다 장비 목록과 구성도를 번갈아 보여줍니다`}
            className={BTN}
          >
            {auto ? <Repeat size={15} /> : <Pause size={15} />}
            {auto ? (pinMap ? '구성도 고정' : '자동 전환') : '전환 멈춤'}
          </button>
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

      <GroupCards tiles={tiles} groupFailed={groupFailed} groupPending={groupPending} large />

      <div className="flex-1 min-h-0 grid grid-cols-[minmax(0,1fr)_400px] gap-4">
        <main className={cn(PANEL, 'min-h-0 flex flex-col overflow-hidden')}>
          <div className="px-4 py-3 border-b border-base-800 flex items-center gap-3">
            <div role="tablist" aria-label="보기 방식" className="flex gap-1 rounded-lg bg-base-800/60 p-1">
              {([['tiles', '장비 목록'], ['map', '구성도']] as const).map(([v, label]) => (
                <button
                  key={v} type="button" role="tab" aria-selected={view === v}
                  onClick={() => { setAuto(false); setParams(v === 'map' ? { view: 'map' } : {}, { replace: true }) }}
                  className={cn('rounded-md px-3 py-1 text-sm font-semibold',
                    view === v ? 'bg-base-700 text-base-50' : 'text-base-400 hover:text-base-200')}
                >
                  {label}
                </button>
              ))}
            </div>
          </div>
          {view === 'map' ? (
            <div className="flex-1 min-h-0 p-4">
              <TopologyMap
                layout={topology}
                lookup={topoLookup}
              />
            </div>
          ) : (
          <div className="flex-1 min-h-0 overflow-y-auto p-4 space-y-5">
            {/* 문제 장비만 크게, 정상 장비는 이름만 접어서 훑는 양을 줄인다 */}
            {problems.length === 0 && stale ? (
              // 조회 실패·갱신 중단인데 초록 "문제 없음"을 띄우면 장애를 정상으로 오인함 (머리글이 이미 알리므로 role=alert는 두지 않음)
              <p className="rounded-xl border border-amber-500/40 px-4 py-6 text-lg font-semibold text-base-200">
                상태 확인 불가 — {failed.length > 0 ? `${failed.join('·')} 조회 실패` : '데이터 갱신 지연'}
              </p>
            ) : problems.length === 0 && pending ? (
              <p className="rounded-xl border border-base-800 px-4 py-6 text-lg font-semibold text-base-400">상태 확인 중…</p>
            ) : problems.length === 0 ? (
              <p className={cn('rounded-xl border px-4 py-6 text-lg font-semibold', TONES.emerald.bg, TONES.emerald.border, TONES.emerald.text)}>문제 장비 없음</p>
            ) : (
              <ul className="grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-3">
                {problems.map(x => {
                  const tone = x.acked ? 'slate' : TONE[x.state]
                  return (
                    <li key={x.key}>
                      <Link to={x.to} className={cn('block h-full rounded-xl border bg-base-900 px-4 py-3 hover:brightness-125', TONES[tone].border, x.acked && 'opacity-60')}>
                        <div className="flex items-center justify-between gap-2">
                          <span className="text-xs text-base-500">{x.group}</span>
                          <span className={cn(badge(tone), x.state === 'crit' && !x.acked && 'motion-safe:animate-pulse')}>
                            {x.acked ? '확인함' : LABEL[x.state]}
                          </span>
                        </div>
                        <div className="mt-1 text-lg font-semibold truncate">{x.name}</div>
                        {x.purpose && <div className="text-sm text-base-400 truncate">{x.purpose}</div>}
                        <div className="mt-2 text-base text-base-200">{x.detail}</div>
                        {x.since && <div className="mt-1 text-sm text-base-400">{lasting(x.since, now)}</div>}
                        {x.bars && (
                          <div className="mt-3 space-y-1.5">
                            {x.bars.map(b => {
                              const v = b.value
                              const fill = v === null ? '' : v >= b.crit ? 'bg-rose-500' : v >= b.warn ? 'bg-amber-500' : 'bg-emerald-500'
                              return (
                                <div key={b.label} className="flex items-center gap-2 text-xs">
                                  <span className="w-14 text-base-500">{b.label}</span>
                                  <div className="flex-1 h-1.5 rounded-full bg-base-800 overflow-hidden">
                                    {v !== null && <div className={cn('h-full rounded-full', fill)} style={{ width: `${Math.min(100, v)}%` }} />}
                                  </div>
                                  <span className="w-9 text-right tabular-nums text-base-300">{v === null ? '-' : `${Math.round(v)}%`}</span>
                                </div>
                              )
                            })}
                          </div>
                        )}
                      </Link>
                    </li>
                  )
                })}
              </ul>
            )}

            {calm.length > 0 && (
              <section aria-label="정상 장비" className="space-y-2">
                {GROUPS.map(g => {
                  const list = calm.filter(x => x.group === g.name)
                  if (!list.length) return null
                  return (
                    <div key={g.name} className="flex gap-4 text-sm leading-7">
                      <span className="w-40 shrink-0 text-base-400">{g.name} <span className="text-emerald-300 font-semibold">정상 {list.filter(x => x.state === 'ok').length}{g.unit}</span></span>
                      <p className="min-w-0 text-base-400">
                        {list.map((x, i) => (
                          <span key={x.key} className="whitespace-nowrap">
                            {i > 0 && <span className="mx-1.5 text-base-700">·</span>}
                            <Link to={x.to} title={x.purpose} className="hover:text-base-100">{x.name}</Link>
                            {x.state !== 'ok' && <span className={cn('ml-1', x.state === 'maint' ? 'text-purple-300' : 'text-base-500')}>({LABEL[x.state]})</span>}
                          </span>
                        ))}
                      </p>
                    </div>
                  )
                })}
              </section>
            )}
          </div>
          )}

          {/* 공무원에게 의미 있는 건 서버 CPU보다 "서비스가 잘 열렸나" */}
          {view === 'tiles' && services.length > 0 && (
            <section className="shrink-0 border-t border-base-800 px-4 py-4">
              <h2 className="text-sm font-semibold text-base-200">서비스 가동률 <span className="font-normal text-base-500">최근 24시간, 정상 접속 비율</span></h2>
              <ul className="mt-3 grid grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-x-8 gap-y-3">
                {services.map(s => {
                  const pct = s.stats.uptime_pct
                  return (
                    <li key={s.monitor.id}>
                      <div className="flex items-baseline justify-between gap-3">
                        <span className="truncate text-base-200">{s.monitor.name}</span>
                        <span className={cn('text-xl font-bold tabular-nums', pct < 99 ? 'text-amber-300' : 'text-emerald-300')}>{pct.toFixed(1)}%</span>
                      </div>
                      <div className="mt-1 h-1.5 rounded-full bg-base-800 overflow-hidden">
                        <div className={cn('h-full rounded-full', pct < 99 ? 'bg-amber-500' : 'bg-emerald-500')} style={{ width: `${pct}%` }} />
                      </div>
                    </li>
                  )
                })}
              </ul>
            </section>
          )}
        </main>

        <aside className="min-h-0 flex flex-col gap-4">
          <ProblemList
            open={open} fresh={fresh} handling={handling} crit={crit} warn={warn}
            impactOf={impactOf} rowAckKeys={rowAckKeys} failed={failed} stale={stale} pending={pending} now={now}
            canAck={false} className="min-h-0 max-h-[65%] shrink-0"
          />

          <section className={cn(PANEL, 'min-h-0 flex-1 flex flex-col overflow-hidden')}>
            <h2 className="px-4 py-3 border-b border-base-800 text-sm font-semibold text-base-200">최근 알림</h2>
            {history.length === 0 ? (
              <p className="px-4 py-3 text-sm text-base-500">최근 알림이 없습니다</p>
            ) : (
              <ol className="min-h-0 overflow-y-auto px-4 py-3 space-y-2.5">
                {history.map(r => (
                  <li key={r.id} className="flex items-center gap-3 text-sm">
                    <span className="w-16 shrink-0 whitespace-nowrap tabular-nums text-base-500">{ago(r.sent_at, now)}</span>
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
