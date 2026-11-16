import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { FolderTree, Pencil, Plus, Trash2, X } from 'lucide-react'
import PageToolbar from '../components/PageToolbar'
import ConfirmDialog from '../components/ConfirmDialog'
import { useFleet, LABEL, DOT } from '../hooks/useFleet'
import { getMe } from '../api/auth'
import { errorText } from '../api/users'
import {
  assignGroup, createGroup, deleteGroup, fetchGroups, indexGroups, memberKey, memberOf, updateGroup, UNGROUPED,
  type DeviceGroup,
} from '../api/groups'
import { cn } from '../utils/cn'

const inputCls = 'w-full px-2.5 py-1.5 rounded-lg bg-base-800 border border-base-700 text-sm text-base-100 focus:outline-none focus:border-brand-500'
const btn = 'inline-flex items-center gap-1.5 h-8 px-3 rounded-lg border border-base-700 text-sm text-base-200 hover:bg-base-800 disabled:opacity-40'
const PAGE = 50

export default function GroupsPage() {
  const qc = useQueryClient()
  const { data: me } = useQuery({ queryKey: ['me'], queryFn: getMe, staleTime: 5 * 60_000 })
  const admin = me?.role === 'admin'
  const q = useQuery({ queryKey: ['groups'], queryFn: fetchGroups })
  const idx = useMemo(() => indexGroups(q.data), [q.data])
  const { tiles } = useFleet()

  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const run = async (fn: () => Promise<unknown>, ok: string, fail: string) => {
    setError(''); setNotice('')
    try {
      await fn()
      setNotice(ok)
      await qc.invalidateQueries({ queryKey: ['groups'] })
      return true
    } catch (e) {
      setError(errorText(e, fail))
      return false
    }
  }

  // 트리 편집
  const [newName, setNewName] = useState('')
  const [newParent, setNewParent] = useState<number | null>(null)
  const [editing, setEditing] = useState<DeviceGroup | null>(null)
  const [removing, setRemoving] = useState<DeviceGroup | null>(null)
  // 옮길 수 없는 위치(자기 자신과 그 아래)는 상위 그룹 목록에서 뺌 — 서버도 거부하지만 고를 수 없게.
  // 이름은 전체 경로 — 본사 › 3층과 부산 › 3층처럼 같은 이름의 하위 그룹을 선택 목록에서 구분
  const parentOptions = (except?: number) => {
    const banned = except != null ? idx.subtree(except) : new Set<number>()
    const out: { id: number; label: string }[] = []
    const walk = (p: number | null) => {
      for (const g of idx.children.get(p) ?? []) {
        if (banned.has(g.id)) continue
        out.push({ id: g.id, label: idx.path(g.id).join(' › ') })
        walk(g.id)
      }
    }
    walk(null)
    return out
  }

  // 장비 지정
  const [scope, setScope] = useState<string>(UNGROUPED)
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(0)
  const [picked, setPicked] = useState<Set<string>>(new Set())
  const [target, setTarget] = useState<string>('')

  const rows = useMemo(() => {
    const scopeSet = scope && scope !== UNGROUPED ? idx.subtree(Number(scope)) : null
    const s = search.trim().toLowerCase()
    return tiles.flatMap(t => {
      const m = memberOf(t.key, t.name)
      if (!m) return []
      const gid = idx.groupOf.get(memberKey(m))
      const g = gid != null && idx.byId.has(gid) ? gid : undefined
      if (scope === UNGROUPED ? g != null : scopeSet ? g == null || !scopeSet.has(g) : false) return []
      if (s && ![t.name, t.sub, t.purpose, t.location].some(v => v?.toLowerCase().includes(s))) return []
      return [{ t, m, mk: memberKey(m), path: g != null ? idx.path(g).join(' › ') : '' }]
    }).sort((a, b) => a.t.name.localeCompare(b.t.name))
  }, [tiles, idx, scope, search])
  const pages = Math.max(1, Math.ceil(rows.length / PAGE))
  const cur = Math.min(page, pages - 1)
  const shown = rows.slice(cur * PAGE, cur * PAGE + PAGE)
  const allShown = shown.length > 0 && shown.every(r => picked.has(r.mk))
  // 검색·범위를 바꿔 지금 안 보이는 장비도 고른 그대로 옮김 — 표시 수와 실제로 옮기는 수가 같게
  const allMembers = useMemo(() => {
    const m = new Map<string, NonNullable<ReturnType<typeof memberOf>>>()
    for (const t of tiles) { const x = memberOf(t.key, t.name); if (x) m.set(memberKey(x), x) }
    return m
  }, [tiles])
  const pickedItems = [...picked].flatMap(mk => allMembers.get(mk) ?? [])

  const togglePick = (mk: string) => setPicked(prev => {
    const next = new Set(prev)
    if (next.has(mk)) next.delete(mk)
    else next.add(mk)
    return next
  })
  const assign = async (to: number | null) => {
    if (!pickedItems.length) return
    const label = to == null ? '그룹에서 뺐습니다' : `${idx.path(to).join(' › ')}(으)로 옮겼습니다`
    if (await run(() => assignGroup(to, pickedItems), `${pickedItems.length}대를 ${label}`, '장비를 옮기지 못했습니다')) setPicked(new Set())
  }

  if (!admin && me) {
    return <p className="p-6 text-sm text-base-400">그룹 관리는 관리자만 쓸 수 있습니다.</p>
  }

  return (
    <div className="space-y-4">
      <PageToolbar icon={FolderTree} title="그룹 관리" description="장비를 위치(본사·지사·층)나 업무 기준으로 묶습니다. 장비는 그룹 하나에만 속합니다." />

      {(error || notice) && (
        <p role={error ? 'alert' : 'status'} className={cn('rounded-lg border px-3 py-2 text-sm',
          error ? 'border-rose-500/30 bg-rose-500/10 text-rose-200' : 'border-base-700 bg-base-900 text-base-200')}>
          {error || notice}
        </p>
      )}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-[22rem_minmax(0,1fr)]">
        <section className="rounded-xl border border-base-800 bg-base-900/50" aria-labelledby="tree-title">
          <h2 id="tree-title" className="border-b border-base-800 px-4 py-2.5 text-sm font-semibold text-base-200">그룹 트리</h2>
          <form className="flex flex-wrap gap-2 border-b border-base-800 p-3"
            onSubmit={async e => {
              e.preventDefault()
              const name = newName.trim()
              if (name && await run(() => createGroup({ name, parent_id: newParent }), `'${name}' 그룹을 만들었습니다`, '그룹을 만들지 못했습니다')) setNewName('')
            }}>
            <input aria-label="새 그룹 이름" className={cn(inputCls, 'min-w-0 flex-1')} placeholder="새 그룹 이름" value={newName} maxLength={60} onChange={e => setNewName(e.target.value)} />
            <select aria-label="새 그룹의 상위 그룹" className={cn(inputCls, 'w-44')} value={newParent ?? ''} onChange={e => setNewParent(e.target.value ? Number(e.target.value) : null)}>
              <option value="">맨 위</option>
              {parentOptions().map(o => <option key={o.id} value={o.id}>{o.label}</option>)}
            </select>
            <button type="submit" className={btn} disabled={!newName.trim()}><Plus size={14} /> 추가</button>
          </form>
          <ul className="max-h-[60vh] overflow-y-auto p-2" aria-label="그룹 목록">
            {parentOptions().map(o => {
              const g = idx.byId.get(o.id)!
              const depth = idx.path(o.id).length - 1
              return (
                <li key={g.id} className="group flex items-center gap-1 rounded-md py-1 pr-1 text-sm text-base-200 hover:bg-base-800/60" style={{ paddingLeft: 8 + depth * 14 }}>
                  <span className="min-w-0 flex-1 truncate">{g.name}</span>
                  <button type="button" className="rounded p-1 text-base-500 hover:bg-base-800 hover:text-base-200" aria-label={`${g.name} 아래에 그룹 추가`} title="하위 그룹 추가"
                    onClick={() => { setNewParent(g.id); document.querySelector<HTMLInputElement>('input[aria-label="새 그룹 이름"]')?.focus() }}>
                    <Plus size={13} />
                  </button>
                  <button type="button" className="rounded p-1 text-base-500 hover:bg-base-800 hover:text-base-200" aria-label={`${g.name} 이름·위치 바꾸기`} title="이름·위치 바꾸기" onClick={() => setEditing(g)}>
                    <Pencil size={13} />
                  </button>
                  <button type="button" className="rounded p-1 text-base-500 hover:bg-rose-500/10 hover:text-rose-300" aria-label={`${g.name} 삭제`} title="삭제" onClick={() => setRemoving(g)}>
                    <Trash2 size={13} />
                  </button>
                </li>
              )
            })}
            {idx.byId.size === 0 && <li className="px-2 py-3 text-xs text-base-500">아직 그룹이 없습니다. 위에서 첫 그룹을 만드세요.</li>}
          </ul>
        </section>

        <section className="min-w-0 rounded-xl border border-base-800 bg-base-900/50" aria-labelledby="dev-title">
          <div className="flex flex-wrap items-center gap-2 border-b border-base-800 px-4 py-2.5">
            <h2 id="dev-title" className="text-sm font-semibold text-base-200">장비 지정</h2>
            <select aria-label="보여 줄 장비" className={cn(inputCls, 'ml-auto w-56')} value={scope} onChange={e => { setScope(e.target.value); setPage(0); setPicked(new Set()) }}>
              <option value={UNGROUPED}>미지정 장비</option>
              <option value="">모든 장비</option>
              {parentOptions().map(o => <option key={o.id} value={o.id}>{o.label}</option>)}
            </select>
            <input aria-label="장비 검색" className={cn(inputCls, 'w-52')} placeholder="이름·IP·용도·위치" value={search} onChange={e => { setSearch(e.target.value); setPage(0) }} />
          </div>

          <div className="flex flex-wrap items-center gap-2 border-b border-base-800 px-4 py-2 text-sm">
            <span className="tabular-nums text-base-400">{pickedItems.length ? `${pickedItems.length}대 선택` : '장비를 고르세요'}</span>
            {pickedItems.length > 0 && <button type="button" className="text-xs text-base-500 hover:text-base-200 underline" onClick={() => setPicked(new Set())}>선택 해제</button>}
            {/* 수백~수천 대를 한 지점으로 옮길 때 페이지를 넘기며 고르지 않게 — 지금 범위·검색에 맞는 장비 전부 */}
            {rows.length > shown.length && rows.some(r => !picked.has(r.mk)) && (
              <button type="button" className="text-xs text-brand-300 hover:underline tabular-nums"
                onClick={() => setPicked(prev => new Set([...prev, ...rows.map(r => r.mk)]))}>
                {search.trim() ? '검색 결과' : '목록'} {rows.length.toLocaleString()}대 모두 선택
              </button>
            )}
            <select aria-label="옮길 그룹" className={cn(inputCls, 'w-56')} value={target} onChange={e => setTarget(e.target.value)}>
              <option value="">옮길 그룹 선택</option>
              {parentOptions().map(o => <option key={o.id} value={o.id}>{o.label}</option>)}
            </select>
            <button type="button" className={btn} disabled={!pickedItems.length || !target} onClick={() => assign(Number(target))}>옮기기</button>
            <button type="button" className={btn} disabled={!pickedItems.length} onClick={() => assign(null)}><X size={14} /> 그룹에서 빼기</button>
          </div>

          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="text-left text-xs text-base-500">
                <tr className="border-b border-base-800">
                  <th className="w-10 px-4 py-2">
                    <input type="checkbox" aria-label="이 페이지 모두 선택" checked={allShown}
                      onChange={() => setPicked(prev => {
                        const next = new Set(prev)
                        for (const r of shown) { if (allShown) next.delete(r.mk); else next.add(r.mk) }
                        return next
                      })} />
                  </th>
                  <th className="px-2 py-2 font-medium">장비</th>
                  <th className="px-2 py-2 font-medium">종류</th>
                  <th className="px-2 py-2 font-medium">그룹</th>
                </tr>
              </thead>
              <tbody>
                {shown.map(({ t, mk, path }) => (
                  <tr key={mk} className="border-b border-base-800/60 hover:bg-base-800/40">
                    <td className="px-4 py-1.5"><input type="checkbox" aria-label={`${t.name} 선택`} checked={picked.has(mk)} onChange={() => togglePick(mk)} /></td>
                    <td className="px-2 py-1.5">
                      <span className="flex items-center gap-2">
                        <span className={cn('h-2 w-2 shrink-0 rounded-full', DOT[t.state])} title={LABEL[t.state]} />
                        <span className="truncate text-base-100">{t.name}</span>
                        {t.sub && <span className="truncate text-xs text-base-500">{t.sub}</span>}
                      </span>
                    </td>
                    <td className="px-2 py-1.5 text-base-400">{t.kind}</td>
                    <td className="px-2 py-1.5 text-base-400">{path || <span className="text-base-600">미지정</span>}</td>
                  </tr>
                ))}
                {rows.length === 0 && (
                  <tr><td colSpan={4} className="px-4 py-6 text-center text-sm text-base-500">{search ? '검색과 일치하는 장비가 없습니다' : '해당하는 장비가 없습니다'}</td></tr>
                )}
              </tbody>
            </table>
          </div>
          {rows.length > PAGE && (
            <div className="flex items-center justify-end gap-2 px-4 py-2 text-xs text-base-400">
              <span className="tabular-nums">{cur * PAGE + 1}–{Math.min(rows.length, (cur + 1) * PAGE)} / {rows.length.toLocaleString()}</span>
              <button type="button" className={btn} disabled={cur === 0} onClick={() => setPage(cur - 1)}>이전</button>
              <button type="button" className={btn} disabled={cur >= pages - 1} onClick={() => setPage(cur + 1)}>다음</button>
            </div>
          )}
        </section>
      </div>

      {editing && (
        <EditDialog g={editing} options={parentOptions(editing.id)} onCancel={() => setEditing(null)}
          onSave={async (name, parent) => {
            if (await run(() => updateGroup({ ...editing, name, parent_id: parent }), '그룹을 바꿨습니다', '그룹을 바꾸지 못했습니다')) setEditing(null)
          }} />
      )}
      <ConfirmDialog open={!!removing} title="그룹 삭제" variant="danger" confirmLabel="삭제"
        message={removing ? `'${removing.name}' 그룹을 지웁니다. 하위 그룹과 장비는 한 단계 위로 올라갑니다${removing.parent_id == null ? '(맨 위 그룹이라 장비는 미지정이 됨)' : ''}.` : ''}
        onCancel={() => setRemoving(null)}
        onConfirm={async () => {
          const g = removing!
          setRemoving(null)
          await run(() => deleteGroup(g.id), `'${g.name}' 그룹을 지웠습니다`, '그룹을 지우지 못했습니다')
        }} />
    </div>
  )
}

function EditDialog({ g, options, onSave, onCancel }: { g: DeviceGroup; options: { id: number; label: string }[]; onSave: (name: string, parent: number | null) => void; onCancel: () => void }) {
  const [name, setName] = useState(g.name)
  const [parent, setParent] = useState<number | null>(g.parent_id)
  return (
    <div role="dialog" aria-modal="true" aria-labelledby="edit-title" className="fixed inset-0 z-50 grid place-items-center bg-black/50 p-4"
      onKeyDown={e => e.key === 'Escape' && onCancel()}>
      <form className="w-full max-w-sm space-y-3 rounded-xl border border-base-700 bg-base-900 p-4"
        onSubmit={e => { e.preventDefault(); if (name.trim()) onSave(name.trim(), parent) }}>
        <h3 id="edit-title" className="text-sm font-semibold text-base-100">그룹 바꾸기</h3>
        <label className="block space-y-1 text-xs text-base-400">이름
          <input autoFocus className={inputCls} value={name} maxLength={60} onChange={e => setName(e.target.value)} />
        </label>
        <label className="block space-y-1 text-xs text-base-400">상위 그룹
          <select className={inputCls} value={parent ?? ''} onChange={e => setParent(e.target.value ? Number(e.target.value) : null)}>
            <option value="">맨 위</option>
            {options.map(o => <option key={o.id} value={o.id}>{o.label}</option>)}
          </select>
        </label>
        <div className="flex justify-end gap-2 pt-1">
          <button type="button" className={btn} onClick={onCancel}>취소</button>
          <button type="submit" className={cn(btn, 'border-brand-500/40 bg-brand-500/15 text-brand-200')} disabled={!name.trim()}>저장</button>
        </div>
      </form>
    </div>
  )
}
