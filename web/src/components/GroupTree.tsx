import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ChevronRight, FolderTree, Settings2 } from 'lucide-react'
import { UNGROUPED, type GroupIndex } from '../api/groups'
import type { GroupCount } from '../hooks/useGroupScope'
import { cn } from '../utils/cn'

interface Props {
  idx: GroupIndex
  counts: Map<number | typeof UNGROUPED, GroupCount>
  total: GroupCount
  selected: string
  onSelect: (g: string) => void
  canEdit: boolean
  className?: string
}

const FOLD_KEY = 'owlmon.groupTree.folded'
const loadFolded = (): Set<number> => {
  try { return new Set(JSON.parse(localStorage.getItem(FOLD_KEY) ?? '[]')) } catch { return new Set() }
}

// 수는 미확인 문제만 — 확인한 문제까지 세면 처리 중인 그룹이 계속 빨갛게 보여 새 장애가 묻힘
function Counts({ c }: { c?: GroupCount }) {
  return (
    <span className="ml-auto flex shrink-0 items-center gap-1.5 text-xs tabular-nums">
      {!!c?.crit && <span className="flex items-center gap-1 text-rose-300"><span className="h-1.5 w-1.5 rounded-full bg-rose-500" />{c.crit}</span>}
      {!!c?.warn && <span className="flex items-center gap-1 text-amber-300"><span className="h-1.5 w-1.5 rounded-full bg-amber-400" />{c.warn}</span>}
      <span className="w-8 text-right text-base-500">{c?.total ?? 0}</span>
    </span>
  )
}

export default function GroupTree({ idx, counts, total, selected, onSelect, canEdit, className }: Props) {
  const [folded, setFolded] = useState(loadFolded)
  const toggle = (id: number) => setFolded(prev => {
    const next = new Set(prev)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    try { localStorage.setItem(FOLD_KEY, JSON.stringify([...next])) } catch { /* 저장 못 해도 화면은 동작 */ }
    return next
  })

  const row = (key: string, label: string, depth: number, c: GroupCount | undefined, hasKids: boolean, id?: number) => {
    const active = selected === key
    return (
      <div key={key} role="treeitem" aria-selected={active} aria-expanded={hasKids ? !folded.has(id!) : undefined}
        className={cn('group flex items-center gap-1 rounded-md pr-2 text-sm',
          active ? 'bg-brand-500/15 text-base-100' : 'text-base-300 hover:bg-base-800/70')}
        style={{ paddingLeft: 4 + depth * 14 }}>
        {hasKids ? (
          <button type="button" onClick={() => toggle(id!)} aria-label={folded.has(id!) ? `${label} 펼치기` : `${label} 접기`}
            className="grid h-6 w-5 shrink-0 place-items-center rounded text-base-500 hover:text-base-200">
            <ChevronRight size={13} className={cn('transition-transform', !folded.has(id!) && 'rotate-90')} />
          </button>
        ) : <span className="w-5 shrink-0" />}
        <button type="button" onClick={() => onSelect(key)} className="flex min-w-0 flex-1 items-center gap-2 py-1.5 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 rounded">
          <span className={cn('truncate', active && 'font-semibold')}>{label}</span>
          <Counts c={c} />
        </button>
      </div>
    )
  }

  const walk = (parent: number | null, depth: number): React.ReactNode[] =>
    (idx.children.get(parent) ?? []).flatMap(g => {
      const kids = (idx.children.get(g.id) ?? []).length > 0
      return [row(String(g.id), g.name, depth, counts.get(g.id), kids, g.id), ...(kids && !folded.has(g.id) ? walk(g.id, depth + 1) : [])]
    })

  return (
    <nav aria-label="장비 그룹" className={cn('flex min-h-0 flex-col rounded-xl border border-base-800 bg-base-900/50', className)}>
      <div className="flex items-center gap-2 border-b border-base-800 px-3 py-2">
        <FolderTree size={14} className="text-base-500" />
        <h2 className="text-sm font-semibold text-base-200">그룹</h2>
        <span className="ml-auto text-xs text-base-500">미확인 · 장비</span>
        {canEdit && (
          <Link to="/groups" aria-label="그룹 관리" title="그룹 관리" className="rounded p-1 text-base-500 hover:bg-base-800 hover:text-base-200">
            <Settings2 size={14} />
          </Link>
        )}
      </div>
      <div role="tree" className="min-h-0 flex-1 overflow-y-auto p-1.5">
        {row('', '전체', 0, total, false)}
        {walk(null, 0)}
        {!!counts.get(UNGROUPED)?.total && row(UNGROUPED, '미지정', 0, counts.get(UNGROUPED), false)}
        {idx.byId.size === 0 && (
          <p className="px-2 py-3 text-xs leading-relaxed text-base-500">
            그룹이 없습니다. 위치(본사·지사·층)나 업무(홈페이지·인사 시스템) 기준으로 장비를 묶으면 그룹별 상태를 볼 수 있습니다.
            {canEdit && <> <Link to="/groups" className="text-brand-300 hover:underline">그룹 만들기</Link></>}
          </p>
        )}
      </div>
    </nav>
  )
}
