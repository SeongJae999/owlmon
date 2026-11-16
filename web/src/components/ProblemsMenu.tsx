import { useEffect, useRef, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { ChevronDown } from 'lucide-react'
import ProblemList from './ProblemList'
import { useClock } from '../hooks/useClock'
import type { useFleet } from '../hooks/useFleet'
import { cn } from '../utils/cn'

const W = 384
// 대형 환경에서 문제가 수백 건이어도 펼칠 때 멈추지 않게 — 나머지는 지금 상태 화면에서
const MAX_ROWS = 30

export default function ProblemsMenu({ fleet, canAck }: { fleet: ReturnType<typeof useFleet>; canAck: boolean }) {
  const location = useLocation()
  const now = useClock(60_000)
  // 연 시점의 이동 키를 기억 — 목록 안 링크·뒤로 가기로 다른 화면에 가면 저절로 닫히게 (같은 경로의 ?focus 이동 포함)
  const [state, setState] = useState<{ left: number; top: number; key: string } | null>(null)
  const pos = state?.key === location.key ? state : null
  const isOpen = !!pos
  const button = useRef<HTMLButtonElement>(null)
  const panel = useRef<HTMLDivElement>(null)
  const n = fleet.fresh.length
  const crit = fleet.crit > 0
  // 첫 응답 전·조회 실패를 '문제 0'으로 보이면 장애를 정상으로 오인함
  const unknown = fleet.failed.length > 0 || fleet.stale(now)
  const count = fleet.pending ? '…' : unknown && !n ? '?' : String(n)

  // 다른 화면으로 가면 연 상태를 지움 — 키만 비교하면 뒤로 가기로 같은 키가 돌아왔을 때 저절로 다시 열림
  const [lastKey, setLastKey] = useState(location.key)
  if (lastKey !== location.key) { setLastKey(location.key); if (state) setState(null) }
  useEffect(() => { if (isOpen) panel.current?.querySelector<HTMLElement>('a, button')?.focus() }, [isOpen])
  useEffect(() => {
    if (!isOpen) return
    const onDown = (e: MouseEvent) => { if (!panel.current?.contains(e.target as Node) && !button.current?.contains(e.target as Node)) setState(null) }
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') { setState(null); button.current?.focus() } }
    const close = () => setState(null)
    window.addEventListener('mousedown', onDown)
    window.addEventListener('keydown', onKey)
    window.addEventListener('resize', close)
    return () => { window.removeEventListener('mousedown', onDown); window.removeEventListener('keydown', onKey); window.removeEventListener('resize', close) }
  }, [isOpen])

  return (
    <>
      <button
        ref={button} type="button" aria-expanded={isOpen} aria-controls="problems-menu"
        aria-label={fleet.pending ? '처리할 문제 불러오는 중' : unknown && !n ? '처리할 문제 확인 불가' : `처리할 문제 ${n}건${crit ? `, 위험 ${fleet.crit}건` : ''}`}
        onClick={e => {
          // 지금 상태 화면엔 같은 목록이 이미 있음 — 같은 내용을 겹쳐 펼치지 않고 그 목록으로 내림.
          // 그룹을 골랐으면 그 목록은 그룹 것뿐이라 버튼 숫자(전체)와 다름 — 그땐 전체 목록을 펼침
          const onPage = location.pathname === '/' && !new URLSearchParams(location.search).get('g') && document.getElementById('problems')
          if (onPage) { onPage.scrollIntoView({ block: 'start' }); onPage.querySelector<HTMLElement>('a, button')?.focus({ preventScroll: true }); return }
          const r = e.currentTarget.getBoundingClientRect()
          setState(isOpen ? null : { left: Math.max(8, Math.min(r.right - W, window.innerWidth - W - 8)), top: r.bottom, key: location.key })
        }}
        className="flex items-center gap-1.5 h-9 px-2.5 rounded-lg border border-base-700 bg-base-800 text-sm font-semibold text-base-200 hover:bg-base-700 transition-colors"
      >
        {crit && <span className="h-2 w-2 rounded-full bg-rose-500" />}
        <span className="lg:max-xl:hidden">문제</span>
        <span className={cn('tabular-nums', crit ? 'text-rose-300' : 'text-base-100')}>{count}</span>
        <ChevronDown size={14} className={cn('text-base-500 transition-transform', isOpen && 'rotate-180')} />
      </button>
      {/* 버튼 바로 다음에 둬 Tab 순서가 이어지게 */}
      {pos && (
        <div ref={panel} id="problems-menu" style={{ left: pos.left, top: pos.top + 4 }}
          className="fixed z-50 w-96 max-w-[calc(100vw-1rem)] max-h-[70vh] flex flex-col rounded-xl border border-base-700 bg-base-900 shadow-xl overflow-hidden">
          <ProblemList
            open={fleet.open.slice(0, MAX_ROWS)}
            impactOf={fleet.impactOf} rowAckKeys={fleet.rowAckKeys} failed={fleet.failed} stale={fleet.stale(now)} pending={fleet.pending} now={now}
            canAck={canAck} compact className="min-h-0 flex-1 rounded-none border-0 bg-transparent"
          />
          <Link to="/" className="shrink-0 flex items-center justify-between gap-2 px-4 py-2.5 border-t border-base-800 text-sm text-brand-300 hover:bg-base-800/40 focus-visible:outline-offset-[-2px]">
            지금 상태에서 전체 보기
            {fleet.open.length > MAX_ROWS && <span className="text-xs text-base-500 tabular-nums">{MAX_ROWS}/{fleet.open.length}건 표시</span>}
          </Link>
        </div>
      )}
    </>
  )
}
