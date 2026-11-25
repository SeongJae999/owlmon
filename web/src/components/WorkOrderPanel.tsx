import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { CalendarClock, Check, ClipboardList, MapPin, Pencil, Play, Plus, RotateCcw, Trash2, X } from 'lucide-react'
import {
  BUCKETS, STATUS_LABEL, WORK_ORDERS_KEY, bucketOf, createWorkOrder, deleteWorkOrder, isOverdue, localDate,
  setWorkOrderStatus, updateWorkOrder, useWorkOrders,
  type Bucket, type HostWork, type WorkOrder, type WorkOrderInput, type WorkStatus,
} from '../api/workOrders'
import { fetchAssets } from '../api/asset'
import { errorText } from '../api/users'
import type { Tile } from '../hooks/useFleet'
import ConfirmDialog from './ConfirmDialog'
import { cn } from '../utils/cn'

const inputCls = 'w-full h-9 px-2.5 rounded-lg bg-base-800 border border-base-700 text-sm text-base-100 placeholder:text-base-500 focus:outline-none focus:border-brand-500'
const NO_ORDERS: WorkOrder[] = []
const EMPTY_FORM: WorkOrderInput = { host_name: '', title: '', description: '', due_date: null, assignee: '' }

// 장비 칸·행에 붙는 끝나지 않은 작업 수. 기한 지난 작업이 하나라도 있으면 붉게
export function WorkBadge({ info, className }: { info?: HostWork; className?: string }) {
  if (!info?.count) return null
  const late = info.overdue > 0
  const label = late ? `작업 ${info.count}건, 기한 지남 ${info.overdue}건` : `작업 ${info.count}건`
  return (
    <span title={label} aria-label={label}
      className={cn('inline-flex items-center gap-0.5 shrink-0 h-4 px-1 rounded text-[10px] font-semibold tabular-nums',
        late ? 'bg-rose-500/20 text-rose-300' : 'bg-brand-500/15 text-brand-300', className)}>
      <ClipboardList size={10} aria-hidden />{info.count}
    </span>
  )
}

// 상단 메뉴 줄의 토글 버튼 — 끝나지 않은 작업 수를 배지로
export function WorkOrderButton({ open, onClick }: { open: boolean; onClick: () => void }) {
  const { data } = useWorkOrders()
  const today = localDate()
  const openList = (data ?? []).filter(w => w.status !== 'done')
  const late = openList.some(w => isOverdue(w, today))
  return (
    <button type="button" onClick={onClick} aria-expanded={open} aria-controls="work-order-panel"
      title="작업 목록" aria-label={`작업 목록${openList.length ? ` (끝나지 않은 작업 ${openList.length}건)` : ''}`}
      className={cn('p-2 rounded-md transition-colors relative', open ? 'text-brand-400 bg-base-700' : 'text-base-400 hover:text-brand-400 hover:bg-base-700')}>
      <ClipboardList size={18} />
      {openList.length > 0 && (
        <span className={cn('absolute -top-0.5 -right-0.5 min-w-4 h-4 px-1 rounded-full text-[10px] font-bold leading-4 text-white tabular-nums',
          late ? 'bg-rose-500' : 'bg-brand-600')}>
          {openList.length > 99 ? '99+' : openList.length}
        </span>
      )}
    </button>
  )
}

export default function WorkOrderPanel({ open, onClose, isAdmin, tiles }: {
  open: boolean; onClose: () => void; isAdmin: boolean; tiles: Tile[]
}) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const { data, isPending, isError } = useWorkOrders()
  const { data: assets } = useQuery({ queryKey: ['assets'], queryFn: fetchAssets, enabled: open })
  const [showDone, setShowDone] = useState(false)
  const [form, setForm] = useState<{ id?: number; value: WorkOrderInput } | null>(null)
  const [finishing, setFinishing] = useState<{ id: number; note: string } | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<WorkOrder | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const panelRef = useRef<HTMLElement>(null)

  useEffect(() => {
    if (!open) return
    panelRef.current?.focus()
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape' && !confirmDelete) onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, onClose, confirmDelete])

  // 랙에 그려지는 장비면 랙으로, 아니면 구성도로
  const rackPlaced = useMemo(() => new Set((assets ?? []).filter(a => a.rack && a.rack_unit > 0).map(a => a.host_name)), [assets])
  const deviceNames = useMemo(() => {
    const s = new Set<string>()
    for (const t of tiles) if (t.kind === '서버' || t.kind === '네트워크 장비' || t.kind === '데이터베이스') s.add(t.name)
    for (const a of assets ?? []) if (a.host_name) s.add(a.host_name)
    return [...s].sort((a, b) => a.localeCompare(b, 'ko'))
  }, [tiles, assets])

  const today = localDate()
  const list = data ?? NO_ORDERS
  const groups = useMemo(() => {
    const g = new Map<Bucket, WorkOrder[]>(BUCKETS.map(b => [b.id, []]))
    for (const w of list) if (w.status !== 'done') g.get(bucketOf(w.due_date, today))!.push(w)
    return g
  }, [list, today])
  const done = useMemo(() => list.filter(w => w.status === 'done').sort((a, b) => (b.done_at ?? '').localeCompare(a.done_at ?? '')), [list])
  const openCount = list.length - done.length

  const run = async (fn: () => Promise<unknown>, after?: () => void) => {
    setBusy(true)
    setError('')
    try {
      await fn()
      after?.()
      qc.invalidateQueries({ queryKey: WORK_ORDERS_KEY })
    } catch (e) {
      setError(errorText(e, '저장하지 못했습니다'))
    } finally {
      setBusy(false)
    }
  }

  const goTo = (host: string) => {
    onClose()
    navigate(rackPlaced.has(host) ? `/racks?focus=${encodeURIComponent(host)}` : `/topology?focus=${encodeURIComponent(host)}`)
  }

  const changeStatus = (w: WorkOrder, status: WorkStatus, note = w.done_note) =>
    run(() => setWorkOrderStatus(w.id, status, note), () => setFinishing(null))

  if (!open) return null

  return (
    <aside ref={panelRef} id="work-order-panel" tabIndex={-1} aria-label="작업 목록"
      className="fixed z-50 top-0 right-0 h-full w-full sm:w-[420px] flex flex-col bg-base-900 border-l border-base-700 shadow-2xl focus:outline-none">
      <div className="flex items-center gap-2 px-4 h-14 shrink-0 border-b border-base-800">
        <ClipboardList size={18} className="text-brand-400" />
        <h2 className="font-bold text-base-50">작업</h2>
        <span className="text-sm text-base-500 tabular-nums">남은 일 {openCount}건</span>
        <button type="button" onClick={onClose} aria-label="작업 목록 닫기" className="ml-auto p-1.5 rounded-lg text-base-400 hover:bg-base-800 hover:text-base-100"><X size={18} /></button>
      </div>

      <div className="flex items-center gap-2 px-4 py-2.5 shrink-0 border-b border-base-800">
        {isAdmin && !form && (
          <button type="button" onClick={() => { setError(''); setForm({ value: EMPTY_FORM }) }}
            className="flex items-center gap-1.5 h-8 px-3 rounded-lg bg-brand-600 hover:bg-brand-500 text-white text-sm font-semibold">
            <Plus size={15} /> 작업 추가
          </button>
        )}
        <label className="ml-auto flex items-center gap-1.5 text-sm text-base-400 cursor-pointer select-none">
          <input type="checkbox" checked={showDone} onChange={e => setShowDone(e.target.checked)} className="accent-brand-500" />
          끝난 작업도 보기
        </label>
      </div>

      <div className="flex-1 overflow-y-auto">
        {form && (
          <WorkForm
            initial={form.value} editing={!!form.id} names={deviceNames} busy={busy}
            onCancel={() => { setForm(null); setError('') }}
            onSubmit={v => run(() => form.id ? updateWorkOrder(form.id, v) : createWorkOrder(v), () => setForm(null))}
          />
        )}
        {error && <p role="alert" className="mx-4 my-2 px-3 py-2 rounded-lg bg-rose-500/10 text-sm text-rose-300">{error}</p>}

        {isError ? (
          <p role="alert" className="px-4 py-10 text-center text-sm text-rose-300">작업 목록을 불러오지 못했습니다</p>
        ) : isPending ? (
          <p className="px-4 py-10 text-center text-sm text-base-500">불러오는 중...</p>
        ) : openCount === 0 && !showDone ? (
          <div className="px-4 py-14 text-center space-y-1.5">
            <p className="text-sm font-semibold text-base-300">남은 작업이 없습니다</p>
            <p className="text-sm text-base-500">{isAdmin ? '장비마다 언제까지 무엇을 할지 적어 두면 현장 엔지니어가 여기서 봅니다' : '관리자가 작업을 남기면 여기에 보입니다'}</p>
          </div>
        ) : (
          <>
            {BUCKETS.map(b => {
              const items = groups.get(b.id) ?? []
              if (!items.length) return null
              return (
                <Section key={b.id} label={b.label} count={items.length} tone={b.id === 'overdue' ? 'late' : b.id === 'today' ? 'today' : undefined}>
                  {items.map(w => (
                    <Item key={w.id} w={w} today={today} isAdmin={isAdmin} busy={busy} finishing={finishing?.id === w.id ? finishing : null}
                      onGo={() => goTo(w.host_name)} onStatus={s => changeStatus(w, s)}
                      onFinishStart={() => setFinishing({ id: w.id, note: w.done_note })}
                      onFinishNote={note => setFinishing({ id: w.id, note })}
                      onFinish={() => changeStatus(w, 'done', finishing?.note.trim() ?? '')}
                      onFinishCancel={() => setFinishing(null)}
                      onEdit={() => { setError(''); setForm({ id: w.id, value: { host_name: w.host_name, title: w.title, description: w.description, due_date: w.due_date, assignee: w.assignee, alert_ref: w.alert_ref } }) }}
                      onDelete={() => setConfirmDelete(w)} />
                  ))}
                </Section>
              )
            })}
            {showDone && (
              <Section label="끝남" count={done.length}>
                {done.length === 0 && <p className="px-4 py-3 text-sm text-base-500">끝난 작업이 없습니다</p>}
                {done.map(w => (
                  <Item key={w.id} w={w} today={today} isAdmin={isAdmin} busy={busy} finishing={null}
                    onGo={() => goTo(w.host_name)} onStatus={s => changeStatus(w, s)}
                    onFinishStart={() => {}} onFinishNote={() => {}} onFinish={() => {}} onFinishCancel={() => {}}
                    onEdit={() => {}} onDelete={() => setConfirmDelete(w)} />
                ))}
              </Section>
            )}
          </>
        )}
      </div>

      <ConfirmDialog
        open={!!confirmDelete} title="작업 삭제"
        message={confirmDelete ? `"${confirmDelete.title}" 작업을 지웁니다. 되돌릴 수 없습니다.` : ''}
        onConfirm={() => { const w = confirmDelete; setConfirmDelete(null); if (w) run(() => deleteWorkOrder(w.id)) }}
        onCancel={() => setConfirmDelete(null)}
      />
    </aside>
  )
}

function Section({ label, count, tone, children }: { label: string; count: number; tone?: 'late' | 'today'; children: React.ReactNode }) {
  return (
    <section aria-label={label} className="border-b border-base-800">
      <h3 className={cn('sticky top-0 z-10 flex items-center gap-2 px-4 py-2 bg-base-900 text-xs font-semibold',
        tone === 'late' ? 'text-rose-300' : tone === 'today' ? 'text-amber-300' : 'text-base-400')}>
        {label} <span className="tabular-nums text-base-500">{count}</span>
      </h3>
      <ul className="divide-y divide-base-800/70">{children}</ul>
    </section>
  )
}

function dueText(due: string | null, today: string): string {
  if (!due) return ''
  if (due === today) return '오늘까지'
  const [y, m, d] = due.split('-').map(Number)
  const [ty, tm, td] = today.split('-').map(Number)
  const days = Math.round((Date.UTC(y, m - 1, d) - Date.UTC(ty, tm - 1, td)) / 86_400_000)
  const date = `${m}월 ${d}일`
  return days < 0 ? `${date} (${-days}일 지남)` : days === 1 ? `${date} (내일)` : `${date}까지`
}

function Item({ w, today, isAdmin, busy, finishing, onGo, onStatus, onFinishStart, onFinishNote, onFinish, onFinishCancel, onEdit, onDelete }: {
  w: WorkOrder; today: string; isAdmin: boolean; busy: boolean; finishing: { note: string } | null
  onGo: () => void; onStatus: (s: WorkStatus) => void
  onFinishStart: () => void; onFinishNote: (note: string) => void; onFinish: () => void; onFinishCancel: () => void
  onEdit: () => void; onDelete: () => void
}) {
  const late = isOverdue(w, today)
  const isDone = w.status === 'done'
  const btn = 'flex items-center gap-1 h-7 px-2.5 rounded-md border border-base-700 text-xs font-semibold text-base-200 hover:bg-base-800 disabled:opacity-50'
  return (
    <li className={cn('px-4 py-3 space-y-2', late && 'shadow-[inset_3px_0_0_var(--color-rose-500)]')}>
      {/* 누르면 그 장비 위치로 — 랙에 있으면 랙 실장도, 없으면 구성도 */}
      <button type="button" onClick={onGo} className="group block w-full text-left space-y-1" title={`${w.host_name} 위치로 가기`}>
        <span className={cn('block text-sm font-semibold leading-snug group-hover:underline', isDone ? 'text-base-400 line-through' : 'text-base-50')}>{w.title}</span>
        <span className="flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs">
          <span className="inline-flex items-center gap-1 text-brand-300"><MapPin size={12} aria-hidden />{w.host_name}</span>
          {w.due_date && (
            <span className={cn('inline-flex items-center gap-1', late ? 'text-rose-300 font-semibold' : 'text-base-400')}>
              <CalendarClock size={12} aria-hidden />{dueText(w.due_date, today)}
            </span>
          )}
          {w.assignee && <span className="text-base-400">담당 {w.assignee}</span>}
          {w.status === 'doing' && <span className="text-amber-300 font-semibold">하는 중</span>}
        </span>
      </button>
      {w.description && <p className="text-xs text-base-400 whitespace-pre-wrap break-words">{w.description}</p>}
      {isDone && (
        <p className="text-xs text-emerald-300/90 whitespace-pre-wrap break-words">
          {w.done_at && `${new Date(w.done_at).toLocaleString('ko-KR', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' })} 끝냄`}
          {w.done_note && ` — ${w.done_note}`}
        </p>
      )}

      {finishing ? (
        <form onSubmit={e => { e.preventDefault(); onFinish() }} className="space-y-2">
          <textarea autoFocus value={finishing.note} onChange={e => onFinishNote(e.target.value)} rows={2} maxLength={2000}
            placeholder="한 일을 짧게 적어 주세요 (예: 디스크 2번 교체, 재부팅 확인)" aria-label="처리 메모"
            className={cn(inputCls, 'h-auto py-2 resize-y')} />
          <div className="flex gap-1.5">
            <button type="submit" disabled={busy} className="flex items-center gap-1 h-7 px-2.5 rounded-md bg-emerald-600 hover:bg-emerald-500 text-xs font-semibold text-white disabled:opacity-50">
              <Check size={13} /> 끝냄으로 표시
            </button>
            <button type="button" onClick={onFinishCancel} className={btn}>취소</button>
          </div>
        </form>
      ) : (
        <div className="flex flex-wrap items-center gap-1.5">
          {w.status === 'todo' && <button type="button" disabled={busy} onClick={() => onStatus('doing')} className={btn}><Play size={12} /> 시작</button>}
          {!isDone && <button type="button" disabled={busy} onClick={onFinishStart} className={btn}><Check size={13} /> 끝냄</button>}
          {isDone && <button type="button" disabled={busy} onClick={() => onStatus('todo')} className={btn}><RotateCcw size={12} /> 다시 열기</button>}
          {w.status === 'doing' && <button type="button" disabled={busy} onClick={() => onStatus('todo')} className="h-7 px-2 text-xs text-base-500 hover:text-base-200">{STATUS_LABEL.todo}으로</button>}
          {isAdmin && (
            <span className="ml-auto flex gap-0.5">
              {!isDone && <button type="button" onClick={onEdit} aria-label={`${w.title} 수정`} className="p-1.5 rounded text-base-500 hover:text-base-100 hover:bg-base-800"><Pencil size={14} /></button>}
              <button type="button" onClick={onDelete} aria-label={`${w.title} 삭제`} className="p-1.5 rounded text-base-500 hover:text-rose-300 hover:bg-base-800"><Trash2 size={14} /></button>
            </span>
          )}
        </div>
      )}
    </li>
  )
}

function WorkForm({ initial, editing, names, busy, onSubmit, onCancel }: {
  initial: WorkOrderInput; editing: boolean; names: string[]; busy: boolean
  onSubmit: (v: WorkOrderInput) => void; onCancel: () => void
}) {
  const [v, setV] = useState(initial)
  const set = (patch: Partial<WorkOrderInput>) => setV(x => ({ ...x, ...patch }))
  return (
    <form onSubmit={e => { e.preventDefault(); onSubmit({ ...v, due_date: v.due_date || null }) }}
      className="mx-4 my-3 p-3 rounded-lg border border-base-700 bg-base-800/40 space-y-2.5" aria-label={editing ? '작업 수정' : '작업 추가'}>
      <label className="block space-y-1">
        <span className="text-xs font-semibold text-base-400">장비</span>
        <input required autoFocus value={v.host_name} list="work-order-devices" onChange={e => set({ host_name: e.target.value })}
          placeholder="장비 이름 (예: web-01)" className={inputCls} autoComplete="off" />
        <datalist id="work-order-devices">{names.map(n => <option key={n} value={n} />)}</datalist>
      </label>
      <label className="block space-y-1">
        <span className="text-xs font-semibold text-base-400">할 일</span>
        <input required maxLength={100} value={v.title} onChange={e => set({ title: e.target.value })} placeholder="예: 2번 디스크 교체" className={inputCls} />
      </label>
      <label className="block space-y-1">
        <span className="text-xs font-semibold text-base-400">자세히 <span className="font-normal text-base-500">(선택)</span></span>
        <textarea rows={2} maxLength={2000} value={v.description} onChange={e => set({ description: e.target.value })}
          placeholder="현장에서 알아야 할 내용" className={cn(inputCls, 'h-auto py-2 resize-y')} />
      </label>
      <div className="grid grid-cols-2 gap-2">
        <label className="block space-y-1">
          <span className="text-xs font-semibold text-base-400">기한 <span className="font-normal text-base-500">(선택)</span></span>
          <input type="date" value={v.due_date ?? ''} onChange={e => set({ due_date: e.target.value || null })} className={inputCls} />
        </label>
        <label className="block space-y-1">
          <span className="text-xs font-semibold text-base-400">담당자 <span className="font-normal text-base-500">(선택)</span></span>
          <input maxLength={60} value={v.assignee} onChange={e => set({ assignee: e.target.value })} placeholder="예: ○○정보 김기사" className={inputCls} />
        </label>
      </div>
      <div className="flex gap-2 pt-1">
        <button type="submit" disabled={busy} className="h-8 px-4 rounded-lg bg-brand-600 hover:bg-brand-500 text-white text-sm font-semibold disabled:opacity-50">
          {busy ? '저장 중' : editing ? '고치기' : '추가'}
        </button>
        <button type="button" onClick={onCancel} className="h-8 px-3 rounded-lg border border-base-700 text-sm text-base-300 hover:bg-base-800">취소</button>
      </div>
    </form>
  )
}
