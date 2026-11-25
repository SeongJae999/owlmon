import axios from 'axios'
import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'

export type WorkStatus = 'todo' | 'doing' | 'done'

export interface WorkOrder {
  id: number
  host_name: string
  title: string
  description: string
  due_date: string | null // "YYYY-MM-DD", null이면 기한 없음
  assignee: string
  status: WorkStatus
  alert_ref: string | null
  created_by: string
  created_at: string
  updated_at: string
  done_note: string
  done_at: string | null
}

export type WorkOrderInput = Pick<WorkOrder, 'host_name' | 'title' | 'description' | 'due_date' | 'assignee'> & { alert_ref?: string | null }

export const STATUS_LABEL: Record<WorkStatus, string> = { todo: '할 일', doing: '하는 중', done: '끝남' }

export async function fetchWorkOrders(params?: { status?: WorkStatus; host?: string }): Promise<WorkOrder[]> {
  const res = await axios.get<WorkOrder[]>('/api/work-orders', { params })
  return res.data ?? []
}

export async function createWorkOrder(w: WorkOrderInput): Promise<WorkOrder> {
  const res = await axios.post<WorkOrder>('/api/work-orders', w)
  return res.data
}

export async function updateWorkOrder(id: number, w: WorkOrderInput): Promise<WorkOrder> {
  const res = await axios.put<WorkOrder>(`/api/work-orders/${id}`, w)
  return res.data
}

export async function setWorkOrderStatus(id: number, status: WorkStatus, done_note: string): Promise<WorkOrder> {
  const res = await axios.post<WorkOrder>(`/api/work-orders/${id}/status`, { status, done_note })
  return res.data
}

export async function deleteWorkOrder(id: number): Promise<void> {
  await axios.delete(`/api/work-orders/${id}`)
}

export const WORK_ORDERS_KEY = ['workOrders'] as const

// 패널·배지·랙·구성도가 같은 목록을 나눠 씀 — 한 번만 불러 오고 1분마다 새로 받음
export function useWorkOrders() {
  return useQuery({ queryKey: WORK_ORDERS_KEY, queryFn: () => fetchWorkOrders(), refetchInterval: 60_000 })
}

// 브라우저 기준 오늘 날짜 — 기한은 날짜만 있어 현장 사람의 달력과 맞춤
export function localDate(d = new Date()): string {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

export type Bucket = 'overdue' | 'today' | 'week' | 'later' | 'none'
export const BUCKETS: { id: Bucket; label: string }[] = [
  { id: 'overdue', label: '기한 지남' },
  { id: 'today', label: '오늘' },
  { id: 'week', label: '이번 주' },
  { id: 'later', label: '나중' },
  { id: 'none', label: '기한 없음' },
]

// 이번 주는 일요일까지 (월요일 시작)
function weekEnd(today: string): string {
  const [y, m, d] = today.split('-').map(Number)
  const t = new Date(y, m - 1, d)
  t.setDate(t.getDate() + ((7 - t.getDay()) % 7))
  return localDate(t)
}

export function bucketOf(due: string | null, today: string): Bucket {
  if (!due) return 'none'
  if (due < today) return 'overdue'
  if (due === today) return 'today'
  return due <= weekEnd(today) ? 'week' : 'later'
}

export const isOverdue = (w: WorkOrder, today: string) => w.status !== 'done' && !!w.due_date && w.due_date < today

export interface HostWork { count: number; overdue: number }

// 장비별 끝나지 않은 작업 수 — 랙·구성도 배지용
export function useOpenWorkByHost(): Map<string, HostWork> {
  const { data } = useWorkOrders()
  // 목록이 그대로여도 날짜가 바뀌면 다시 셈 (자정 넘어 기한 지남으로 바뀜)
  const today = localDate()
  return useMemo(() => {
    const m = new Map<string, HostWork>()
    for (const w of data ?? []) {
      if (w.status === 'done') continue
      const e = m.get(w.host_name) ?? { count: 0, overdue: 0 }
      e.count++
      if (isOverdue(w, today)) e.overdue++
      m.set(w.host_name, e)
    }
    return m
  }, [data, today])
}
