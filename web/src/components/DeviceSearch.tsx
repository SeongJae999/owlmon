import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Search } from 'lucide-react'
import { DOT, LABEL, type Tile } from '../hooks/useFleet'
import { cn } from '../utils/cn'

const LIMIT = 8

// 앞 8건만 보이고 전체 건수는 셈 — 8건에서 끊기면 찾는 장비가 없다고 오인함. tiles가 문제 순이라 문제 장비가 먼저 나옴
function matchTiles(tiles: Tile[], q: string): { hits: Tile[]; total: number } {
  const s = q.trim().toLowerCase()
  if (!s) return { hits: [], total: 0 }
  const hits: Tile[] = []
  let total = 0
  for (const x of tiles) {
    if (![x.name, x.sub, x.purpose, x.location].some(v => v?.toLowerCase().includes(s))) continue
    total++
    if (hits.length < LIMIT) hits.push(x)
  }
  return { hits, total }
}

export default function DeviceSearch({ tiles }: { tiles: Tile[] }) {
  const navigate = useNavigate()
  const [q, setQ] = useState('')
  const [open, setOpen] = useState(false)
  // 2xl 미만에선 아이콘만 두고 누르면 입력칸을 머리줄 위에 펼침 — 입력칸이 늘 있으면 1024~1440px에서 주 메뉴 뒤쪽이 잘림
  const [expanded, setExpanded] = useState(false)
  const [active, setActive] = useState(0)
  const box = useRef<HTMLDivElement>(null)
  const input = useRef<HTMLInputElement>(null)
  const toggle = useRef<HTMLButtonElement>(null)
  const { hits, total } = useMemo(() => matchTiles(tiles, q), [tiles, q])
  const cur = Math.min(active, hits.length - 1)
  const showList = open && q.trim() !== ''

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== '/' || e.ctrlKey || e.metaKey || e.altKey || e.defaultPrevented) return
      const t = e.target as HTMLElement
      if (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName)) return
      // 대화상자가 열려 있으면 포커스를 그 밖으로 빼앗지 않음
      if (t.closest?.('[aria-modal="true"], [role="dialog"]') || document.querySelector('[aria-modal="true"]')) return
      e.preventDefault()
      setExpanded(true)
      input.current?.focus()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])
  // 숨겨져 있던 입력칸은 펼쳐진 뒤에야 포커스를 받음
  useEffect(() => { if (expanded) input.current?.focus() }, [expanded])

  // 지금 상태 화면의 장비 표 검색으로 — 종류·그룹 거름은 풀어 전체에서 찾게
  const showAll = () => {
    navigate(`/?q=${encodeURIComponent(q.trim())}#hosts`)
    setQ('')
    setOpen(false)
    setExpanded(false)
    input.current?.blur()
  }

  const go = (x: Tile) => {
    navigate(x.to)
    setQ('')
    setOpen(false)
    setExpanded(false)
    input.current?.blur()
  }

  return (
    <div ref={box} className="relative" onBlur={e => { if (!box.current?.contains(e.relatedTarget as Node)) { setOpen(false); setExpanded(false) } }}>
      <button ref={toggle} type="button" aria-label="장비 찾기" aria-expanded={expanded} onClick={() => setExpanded(true)}
        className="2xl:hidden p-2 rounded-lg border border-base-700 bg-base-800 text-base-400 hover:text-brand-400 hover:bg-base-700 transition-colors">
        <Search size={18} />
      </button>
      <div className={cn('relative', expanded ? 'max-2xl:fixed max-2xl:right-3 max-2xl:top-2.5 max-2xl:z-50 max-2xl:w-[min(24rem,calc(100vw-1.5rem))]' : 'max-2xl:hidden')}>
        <Search size={15} className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-base-500" />
        <input
          ref={input} type="text" value={q} placeholder="장비 찾기  /" autoComplete="off" spellCheck={false}
          role="combobox" aria-label="장비 찾기" aria-expanded={showList} aria-controls="device-search-list" aria-autocomplete="list"
          aria-activedescendant={showList && cur >= 0 ? `device-search-${cur}` : undefined}
          onChange={e => { setQ(e.target.value); setActive(0); setOpen(true) }}
          onFocus={() => setOpen(true)}
          onKeyDown={e => {
            if (e.key === 'ArrowDown' && hits.length) { e.preventDefault(); setOpen(true); setActive((cur + 1) % hits.length) }
            else if (e.key === 'ArrowUp' && hits.length) { e.preventDefault(); setActive((cur - 1 + hits.length) % hits.length) }
            else if (e.key === 'Enter' && showList && hits[cur]) { e.preventDefault(); go(hits[cur]) }
            else if (e.key === 'Escape') {
              setQ('')
              setOpen(false)
              setExpanded(false)
              // 아이콘이 보이는 폭에서만 아이콘으로 돌려보냄 — 숨은 버튼엔 포커스가 안 가 입력칸에 남음
              if (toggle.current?.offsetParent) toggle.current.focus()
              else input.current?.blur()
            }
          }}
          className="h-9 w-full 2xl:w-64 pl-8 pr-2 rounded-lg border border-base-700 bg-base-800 text-sm text-base-100 placeholder:text-base-500"
        />
        {showList && (hits.length === 0 ? (
          <p className="absolute right-0 top-full mt-1 w-full 2xl:w-80 px-3 py-2.5 rounded-xl border border-base-700 bg-base-900 shadow-xl text-sm text-base-500">일치하는 장비 없음</p>
        ) : (
          <ul id="device-search-list" role="listbox" aria-label="장비 찾기 결과"
            className="absolute right-0 top-full mt-1 w-full 2xl:w-80 py-1 rounded-xl border border-base-700 bg-base-900 shadow-xl">
            {hits.map((x, i) => (
              <li
                key={x.key} id={`device-search-${i}`} role="option" aria-selected={i === cur}
                // 클릭 전에 입력칸이 포커스를 잃으면 목록이 먼저 닫혀 클릭이 안 먹음
                onMouseDown={e => e.preventDefault()} onMouseEnter={() => setActive(i)} onClick={() => go(x)}
                className={cn('flex items-center gap-2.5 px-3 py-2 cursor-pointer', i === cur && 'bg-base-800')}
              >
                <span className={cn('h-2 w-2 shrink-0 rounded-full', (x.acked && x.state !== 'ok') || x.causedBy ? 'bg-base-500' : DOT[x.state])} />
                <span className="sr-only">{LABEL[x.state]}</span>
                <span className="min-w-0">
                  <span className="block truncate text-sm text-base-100">{x.name}</span>
                  <span className="block truncate text-xs text-base-500">{x.kind}{x.sub && ` · ${x.sub}`}</span>
                </span>
              </li>
            ))}
            {total > hits.length && (
              <li role="presentation" className="border-t border-base-800 mt-1">
                <button type="button" onMouseDown={e => e.preventDefault()} onClick={showAll}
                  className="w-full px-3 py-2 text-left text-sm text-brand-300 hover:bg-base-800 tabular-nums">
                  {total.toLocaleString()}건 모두 보기
                </button>
              </li>
            )}
          </ul>
        ))}
      </div>
    </div>
  )
}
