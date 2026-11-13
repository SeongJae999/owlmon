import { Link } from 'react-router-dom'
import { Database, Globe, Network, Server } from 'lucide-react'
import { GROUPS, type Group, type Tile } from '../hooks/useFleet'
import { TONES } from '../utils/severity'
import { cn } from '../utils/cn'

const ICON = { 서버: Server, '네트워크 장비': Network, 웹사이트: Globe, 데이터베이스: Database } as const

interface Props {
  tiles: Tile[]
  groupFailed: (g: Group) => boolean
  groupPending: (g: Group) => boolean
  large?: boolean // 상황판(벽 모니터)용 큰 글씨
  // 메인에선 아래 장비 표를 그 종류로 거르는 필터 — 없으면(상황판) 종류별 화면으로 이동
  selected?: Group | null
  onSelect?: (g: Group | null) => void
}

function Card({ to, onClick, pressed, className, children }: { to: string; onClick?: () => void; pressed?: boolean; className: string; children: React.ReactNode }) {
  return onClick
    ? <button type="button" onClick={onClick} aria-pressed={pressed} className={className}>{children}</button>
    : <Link to={to} className={className}>{children}</Link>
}

export default function GroupCards({ tiles, groupFailed, groupPending, large, selected, onSelect }: Props) {
  return (
    <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
      {GROUPS.map(g => {
        const list = tiles.filter(x => x.group === g.name)
        // 위쪽 장비 장애로 함께 끊긴 장비는 원인 쪽에서 한 번만 셈 — 머리글·처리할 문제 숫자와 맞추기 위해
        const crit = list.filter(x => x.state === 'crit' && !x.acked && !x.causedBy).length
        const warn = list.filter(x => x.state === 'warn' && !x.acked && !x.causedBy).length
        const caused = list.filter(x => x.causedBy).length
        const handling = list.filter(x => (x.state === 'crit' || x.state === 'warn') && x.acked && !x.causedBy).length
        const maint = list.filter(x => x.state === 'maint').length
        const ok = list.filter(x => x.state === 'ok').length
        const wait = list.filter(x => x.state === 'wait').length
        // 조회 실패를 "등록 없음"·"모두 정상"으로 두면 장애를 놓침 — 이전 값이 남아 있어도 실패를 우선 표시
        const fail = groupFailed(g.name)
        const waiting = !fail && groupPending(g.name)
        const tone = TONES[fail ? 'amber' : waiting || list.length === 0 ? 'slate' : crit ? 'rose' : warn ? 'amber' : handling || wait || caused ? 'slate' : 'emerald']
        const Icon = ICON[g.name]
        const num = cn('font-bold tabular-nums', large ? 'text-xl' : 'text-base')
        return (
          <Card
            key={g.name}
            to={g.to}
            onClick={onSelect && (() => onSelect(selected === g.name ? null : g.name))}
            pressed={onSelect ? selected === g.name : undefined}
            className={cn('rounded-xl border bg-base-900/50 flex items-center gap-3 text-left hover:border-base-700', large ? 'px-5 py-4 gap-4' : 'px-4 py-3',
              selected === g.name ? 'border-brand-500/70 ring-1 ring-brand-500/40' : 'border-base-800')}
          >
            <Icon size={large ? 28 : 20} className={cn('shrink-0', tone.text)} />
            <div className="min-w-0">
              <div className={cn('font-semibold text-base-400', large ? 'text-sm' : 'text-xs')}>{g.name}</div>
              <div className={cn('text-base-300 flex flex-wrap items-baseline gap-x-2', large ? 'text-base' : 'text-sm')}>
                {fail ? <span className="font-semibold text-base-200">조회 실패</span>
                  : waiting ? <span>확인 중…</span>
                  : list.length === 0 ? <span>등록 없음</span>
                  : crit || warn ? (
                    <>
                      {crit > 0 && <span>위험 <span className={cn(num, TONES.rose.text)}>{crit}</span></span>}
                      {warn > 0 && <span>주의 <span className={cn(num, TONES.amber.text)}>{warn}</span></span>}
                      <span className="text-base-500 tabular-nums">/ {list.length}{g.unit}</span>
                    </>
                  ) : (
                    <span>정상 <span className={cn(num, wait || caused ? 'text-base-300' : TONES.emerald.text)}>{ok}</span><span className="ml-1 text-base-500">/ {list.length}{g.unit}</span></span>
                  )}
                {!fail && !waiting && caused > 0 && <span className="text-base-500 tabular-nums">상위 영향 {caused}</span>}
                {!fail && !waiting && handling > 0 && <span className="text-base-500 tabular-nums">처리 중 {handling}</span>}
                {!fail && !waiting && maint > 0 && <span className="text-base-500 tabular-nums">점검 {maint}</span>}
                {!fail && !waiting && wait > 0 && <span className="text-base-500 tabular-nums">확인 중 {wait}</span>}
              </div>
            </div>
          </Card>
        )
      })}
    </div>
  )
}
