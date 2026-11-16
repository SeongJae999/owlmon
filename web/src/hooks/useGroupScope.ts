import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import { fetchGroups, indexGroups, tileGroup, UNGROUPED, type GroupIndex } from '../api/groups'

interface ScopeTile { key: string; name: string; ip?: string; state: string; acked?: unknown; causedBy?: string }
export interface GroupCount { total: number; crit: number; warn: number }

// 선택한 그룹(주소 ?g=)으로 장비·문제를 거르고, 트리에 보일 그룹별 수(하위 그룹 합산)를 셈.
// 그룹 안 장비가 그룹 밖 장비(코어 스위치 등) 장애로 끊겼으면 그 원인 장비를 그룹의 문제로 함께 셈 — 아니면 그룹 전체가 끊겨도 0건으로 보임
export function useGroupScope<T extends ScopeTile>(tiles: T[], open: T[], lookup: (name: string) => T | undefined) {
  const [params, setParams] = useSearchParams()
  const sel = params.get('g') ?? ''
  // 상황판은 하루 종일 켜 둠 — 관리 화면에서 바꾼 소속이 새로고침 없이 반영되게
  const q = useQuery({ queryKey: ['groups'], queryFn: fetchGroups, refetchInterval: 60_000, refetchIntervalInBackground: true })
  const idx = useMemo<GroupIndex>(() => indexGroups(q.data), [q.data])
  const groupsPending = q.isPending

  return useMemo(() => {
    // 지워진 그룹을 가리키는 소속은 미지정(또는 IP 대역)으로 — 화면에서 장비가 사라지지 않게
    const gid = (t: ScopeTile) => tileGroup(idx, t)?.id
    const chain = (g: number | undefined): (number | typeof UNGROUPED)[] => {
      if (g == null) return [UNGROUPED]
      const out: number[] = []
      for (let p: number | null | undefined = g, hops = 0; p != null && hops < 100; p = idx.byId.get(p)?.parent_id, hops++) out.push(p)
      return out
    }
    const openKeys = new Set(open.map(t => t.key))
    // 그룹 안 장비를 끊기게 한 원인 장비 (처리할 문제에 있는 것만)
    const causeOf = (t: ScopeTile) => {
      if (!t.causedBy) return undefined
      const c = lookup(t.causedBy)
      return c && openKeys.has(c.key) ? c : undefined
    }

    const counts = new Map<number | typeof UNGROUPED, GroupCount>()
    const problems = new Map<number | typeof UNGROUPED, Map<string, T>>()
    const bump = (k: number | typeof UNGROUPED) => {
      let c = counts.get(k)
      if (!c) counts.set(k, (c = { total: 0, crit: 0, warn: 0 }))
      return c
    }
    const addProblem = (k: number | typeof UNGROUPED, t: T) => {
      let m = problems.get(k)
      if (!m) problems.set(k, (m = new Map()))
      m.set(t.key, t)
    }
    for (const t of tiles) {
      const ch = chain(gid(t))
      for (const k of ch) bump(k).total++
      const c = causeOf(t)
      if (c && !c.acked) for (const k of ch) addProblem(k, c)
    }
    for (const t of open) {
      if (t.acked) continue
      for (const k of chain(gid(t))) addProblem(k, t)
    }
    for (const [k, m] of problems) {
      const c = bump(k)
      for (const t of m.values()) { if (t.state === 'crit') c.crit++; else c.warn++ }
    }

    const selId = sel && sel !== UNGROUPED ? Number(sel) : NaN
    const valid = sel === UNGROUPED || idx.byId.has(selId)
    const inScope = !valid ? () => true
      : sel === UNGROUPED ? (t: ScopeTile) => gid(t) == null
      : ((s: Set<number>) => (t: ScopeTile) => { const g = gid(t); return g != null && s.has(g) })(idx.subtree(selId))
    const scopedTiles = valid ? tiles.filter(inScope) : tiles
    let scopedOpen = open
    if (valid) {
      const seen = new Set<string>()
      const list: T[] = []
      const add = (t: T) => { if (!seen.has(t.key)) { seen.add(t.key); list.push(t) } }
      for (const t of open) if (inScope(t)) add(t)
      for (const t of scopedTiles) { const c = causeOf(t); if (c) add(c) }
      // 전체 목록(useFleet)의 우선순위를 그대로 — 그룹 밖 원인 장비도 전체 목록에 있음
      const rank = new Map(open.map((t, i) => [t.key, i]))
      scopedOpen = list.sort((a, b) => (rank.get(a.key) ?? Infinity) - (rank.get(b.key) ?? Infinity))
    }
    const fresh = scopedOpen.filter(x => !x.acked)

    const select = (g: string) => {
      const next = new URLSearchParams(params)
      next.delete('p') // 장비 표 페이지 — 다른 그룹에선 의미 없음
      if (g) next.set('g', g)
      else next.delete('g')
      setParams(next, { replace: true })
    }
    const label = !valid ? '전체' : sel === UNGROUPED ? '미지정' : idx.path(selId).join(' › ')
    // 선택 목록은 전체 경로로 — 본사 › 3층과 부산 › 3층처럼 같은 이름의 하위 그룹을 구분
    const options: { id: string; label: string }[] = []
    const walk = (p: number | null) => {
      for (const g of idx.children.get(p) ?? []) { options.push({ id: String(g.id), label: idx.path(g.id).join(' › ') }); walk(g.id) }
    }
    walk(null)

    return {
      idx, counts, selected: valid ? sel : '', label, options, select, groupOf: gid,
      // 주소에 그룹이 있는데 그룹 정보를 못 받았거나 지워진 그룹이면 전체를 보이므로 화면에 알려야 함 — 지점 상황판이 조용히 전사 숫자를 보이지 않게
      unresolved: !!sel && !valid ? (groupsPending ? 'loading' : 'missing') : '',
      tiles: scopedTiles, open: scopedOpen, fresh,
      handling: scopedOpen.length - fresh.length,
      crit: fresh.filter(x => x.state === 'crit').length,
      warn: fresh.filter(x => x.state === 'warn').length,
    }
  }, [tiles, open, lookup, idx, sel, params, setParams, groupsPending])
}
