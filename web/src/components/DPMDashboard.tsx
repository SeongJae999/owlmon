import React, { useState } from 'react'
import {
  getDPMStatus, addDPMInstance, deleteDPMInstance, getDPMQueries, triggerDPMCheck,
  type DPMStatusItem,
} from '../api/dpm'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Database, Clock, Trash2, RefreshCcw, Plus, X, Save, AlertTriangle, Zap, Info } from 'lucide-react'
import { Link } from 'react-router-dom'
import { cn } from '../utils/cn'
import PageToolbar from './PageToolbar'
import ConfirmDialog from './ConfirmDialog'

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 ** 2) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 ** 3) return `${(bytes / 1024 ** 2).toFixed(1)} MB`
  return `${(bytes / 1024 ** 3).toFixed(2)} GB`
}

function formatMs(ms: number): string {
  if (ms < 1) return `${ms.toFixed(2)}ms`
  if (ms < 1000) return `${ms.toFixed(1)}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

function InstanceCard({
  item, onDelete, onCheck, onShowQueries,
}: {
  item: DPMStatusItem
  onDelete: () => void
  onCheck: () => void
  onShowQueries: () => void
}) {
  const inst = item.instance
  const m = item.metrics
  const isError = m?.error && m.error !== ''
  const hasNotice = m?.notice && m.notice !== ''
  const connRatio = m && m.connections_max > 0
    ? ((m.connections_active + m.connections_idle) / m.connections_max) * 100
    : 0
  const cacheHitPct = m ? m.cache_hit_ratio * 100 : 0

  return (
    <div className={cn(
      "bg-slate-900 rounded-3xl border p-5 shadow-premium transition-all group relative overflow-hidden",
      isError ? "border-rose-500/30 ring-1 ring-rose-500/20" : "border-slate-800"
    )}>
      {/* Header */}
      <div className="flex items-start justify-between gap-3 mb-6">
        <div className="flex items-center gap-3 min-w-0 flex-1">
          <div className={cn(
            "p-2 rounded-lg bg-slate-800 text-slate-400 group-hover:text-indigo-400 transition-colors shrink-0",
            m && !isError && "text-indigo-400 bg-indigo-500/10"
          )}>
            <Database size={18} />
          </div>
          <div className="min-w-0 flex-1">
            <h4 className="font-bold text-slate-100 leading-snug line-clamp-2" title={inst.name}>{inst.name}</h4>
            <div className="flex items-center gap-1.5 mt-0.5 min-w-0">
              <span className="text-[10px] font-bold text-slate-400 uppercase tracking-tight shrink-0">{inst.db_type}</span>
              <span className="text-slate-200 shrink-0">•</span>
              <span className="text-[10px] font-bold text-slate-400 truncate font-mono">{inst.host}:{inst.port}</span>
            </div>
          </div>
        </div>
        
        <div className="flex items-center gap-1 shrink-0">
          <span className={cn(
            "inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-bold tracking-tight",
            isError ? "bg-rose-500/15 text-rose-300" : m ? "bg-emerald-500/15 text-emerald-300" : "bg-slate-800 text-slate-500"
          )}>
            {isError ? '연결 오류' : m ? '정상' : '대기'}
          </span>
          <div className="flex opacity-0 group-hover:opacity-100 transition-opacity">
            <button onClick={onCheck} className="p-1 rounded-md text-slate-400 hover:bg-slate-800 hover:text-indigo-400 transition-all" title="즉시 점검"><RefreshCcw size={14} /></button>
            <button onClick={onShowQueries} className="p-1 rounded-md text-slate-400 hover:bg-slate-800 hover:text-indigo-400 transition-all" title="슬로우 쿼리"><Zap size={14} /></button>
            <button onClick={onDelete} className="p-1 rounded-md text-slate-400 hover:bg-rose-500/10 hover:text-rose-500 transition-all" title="삭제"><Trash2 size={14} /></button>
          </div>
        </div>
      </div>

      {isError ? (
        <div className="bg-rose-500/10 border border-rose-500/20 rounded-xl p-4 flex items-center gap-3 text-rose-300 text-xs font-bold">
          <AlertTriangle size={16} className="shrink-0" />
          <p>{m!.error}</p>
        </div>
      ) : !m ? (
        <div className="bg-slate-800 border border-slate-800 rounded-xl p-8 flex flex-col items-center justify-center text-slate-400 gap-2">
          <Clock size={24} className="opacity-20" />
          <p className="text-[11px] font-bold">점검 대기 중...</p>
        </div>
      ) : (
        <div className="space-y-5">
          {/* Main Metrics */}
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-1.5">
              <div className="flex justify-between items-end">
                <span className="text-[10px] font-bold text-slate-400 tracking-wide leading-none">연결 수</span>
                <span className={cn(
                  "text-[10px] font-bold leading-none",
                  connRatio >= 80 ? "text-rose-500" : connRatio >= 60 ? "text-amber-500" : "text-emerald-500"
                )}>{m.connections_active + m.connections_idle}/{m.connections_max}</span>
              </div>
              <div className="h-1.5 w-full bg-slate-800 rounded-full overflow-hidden">
                <div className={cn(
                  "h-full transition-all duration-500",
                  connRatio >= 80 ? "bg-rose-500" : connRatio >= 60 ? "bg-amber-500" : "bg-emerald-500"
                )} style={{ width: `${Math.min(connRatio, 100)}%` }} />
              </div>
            </div>
            <div className="space-y-1.5">
              <div className="flex justify-between items-end">
                <span className="text-[10px] font-bold text-slate-400 tracking-wide leading-none">캐시 적중률</span>
                <span className={cn(
                  "text-[10px] font-bold leading-none",
                  cacheHitPct >= 95 ? "text-emerald-500" : cacheHitPct >= 90 ? "text-amber-500" : "text-rose-500"
                )}>{cacheHitPct.toFixed(1)}%</span>
              </div>
              <div className="h-1.5 w-full bg-slate-800 rounded-full overflow-hidden">
                <div className={cn(
                  "h-full transition-all duration-500",
                  cacheHitPct >= 95 ? "bg-emerald-500" : cacheHitPct >= 90 ? "bg-amber-500" : "bg-rose-500"
                )} style={{ width: `${Math.min(cacheHitPct, 100)}%` }} />
              </div>
            </div>
          </div>

          {/* Details */}
          <div className="space-y-2 py-3 border-y border-slate-800">
            <div className="flex items-center justify-between text-[11px] font-bold">
              <span className="text-slate-400 tracking-wide">DB 크기</span>
              <span className="text-slate-400">{formatBytes(m.db_size_bytes)}</span>
            </div>
            <div className="flex items-center justify-between text-[11px] font-bold">
              <span className="text-slate-400 tracking-wide">활성 / 대기</span>
              <span className="text-slate-400">{m.connections_active} / {m.connections_idle}</span>
            </div>
          </div>

          {hasNotice && m!.notice === 'pg_stat_statements_missing' ? (
            <div className="bg-sky-500/10 border border-sky-500/20 rounded-lg p-2.5 flex items-start gap-2 text-[11px]">
              <Info size={12} className="shrink-0 text-sky-400 mt-0.5" />
              <div className="space-y-0.5 min-w-0">
                <div className="text-sky-300 font-bold">쿼리 분석 기능 OFF</div>
                <div className="text-sky-300/80 leading-relaxed">
                  기본 모니터링은 정상. 슬로우 쿼리 찾기 기능을 쓰려면 <code className="bg-slate-900 px-1 py-0.5 rounded text-[10px]">pg_stat_statements</code> 확장 설치가 필요합니다.{' '}
                  <Link to="/support#troubleshoot" className="text-indigo-300 hover:text-indigo-200 underline underline-offset-2">설치 방법 →</Link>
                </div>
              </div>
            </div>
          ) : hasNotice && (
            <div className="bg-sky-500/10 border border-sky-500/20 rounded-lg p-2.5 flex items-start gap-2 text-[11px]">
              <AlertTriangle size={12} className="shrink-0 text-sky-400 mt-0.5" />
              <span className="text-sky-300 font-medium">{m!.notice}</span>
            </div>
          )}

          <div className="flex items-center justify-between text-[10px] font-bold text-slate-400 tracking-wide">
            <div className="flex items-center gap-1.5">
              <Clock size={10} />
              점검 주기 {inst.poll_interval_sec}초
            </div>
            <div>최근 확인: {new Date(m.collected_at).toLocaleTimeString('ko-KR')}</div>
          </div>
        </div>
      )}
    </div>
  )
}

function AddInstanceForm({ onAdd }: { onAdd: () => void }) {
  const queryClient = useQueryClient()
  const [form, setForm] = useState({
    name: '', db_type: 'postgres', host: '', port: 5432,
    username: '', password: '', database: '',
    poll_interval_sec: 60, enabled: true,
  })

  const [error, setError] = useState<string | null>(null)
  const mutation = useMutation({
    mutationFn: addDPMInstance,
    onSuccess: async () => {
      await queryClient.refetchQueries({ queryKey: ['dpmStatus'] })
      onAdd()
    },
    onError: (err: any) => {
      const msg = err?.response?.data?.toString?.().trim() || err?.message || '등록 실패'
      setError(msg)
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    mutation.mutate(form)
  }

  return (
    <form onSubmit={handleSubmit} className="bg-slate-900 rounded-3xl border border-indigo-500/30 p-6 shadow-xl animate-in fade-in slide-in-from-top-4 duration-300 space-y-6">
      <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
        <FormField label="표시 이름" value={form.name} onChange={(v: string) => setForm({...form, name: v})} placeholder="예: 학사시스템 DB" />
        <FormField label="DB 종류" type="select" value={form.db_type} onChange={(v: string) => setForm({...form, db_type: v})} options={[{value: 'postgres', label: 'PostgreSQL'}]} />
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-5">
        <FormField label="호스트 주소" value={form.host} onChange={(v: string) => setForm({...form, host: v})} placeholder="db.school.local" />
        <FormField label="포트" type="number" value={form.port} onChange={(v: string) => setForm({...form, port: Number(v)})} />
        <FormField label="데이터베이스 이름" value={form.database} onChange={(v: string) => setForm({...form, database: v})} placeholder="school" />
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-5">
        <FormField label="사용자명" value={form.username} onChange={(v: string) => setForm({...form, username: v})} placeholder="owlmon_readonly" />
        <FormField label="비밀번호" type="password" value={form.password} onChange={(v: string) => setForm({...form, password: v})} />
        <FormField label="점검 주기 (초)" type="number" value={form.poll_interval_sec} onChange={(v: string) => setForm({...form, poll_interval_sec: Number(v)})} />
      </div>

      <div className="bg-amber-500/10 border border-amber-500/20 rounded-xl p-4 flex items-start gap-3">
        <Zap size={18} className="text-amber-400 mt-0.5 shrink-0" />
        <div className="space-y-1">
          <p className="text-xs font-bold text-amber-200">성능 분석 권장 설정</p>
          <p className="text-[11px] text-amber-300 leading-relaxed font-medium">슬로우 쿼리 수집을 위해 대상 DB에 <code className="bg-slate-900 px-1 rounded border border-amber-500/30">pg_stat_statements</code> 확장이 활성화되어야 합니다. 또한 읽기 전용 계정 사용을 권장합니다.</p>
        </div>
      </div>

      {error && (
        <div className="flex items-start gap-2 px-3 py-2 rounded-lg bg-rose-500/10 border border-rose-500/30 text-xs">
          <AlertTriangle size={14} className="shrink-0 text-rose-400 mt-0.5" />
          <span className="text-rose-300 font-medium">{error}</span>
        </div>
      )}

      <div className="flex justify-end gap-2 pt-2">
        <button
          type="button"
          onClick={onAdd}
          disabled={mutation.isPending}
          className="px-6 py-3 rounded-xl text-sm font-bold bg-slate-800 text-slate-300 border border-slate-700 hover:bg-slate-700 transition-colors disabled:opacity-50"
        >
          취소
        </button>
        <button
          type="submit"
          disabled={mutation.isPending}
          className={cn(
            "flex items-center gap-2 px-10 py-3 rounded-xl text-sm font-bold transition-all shadow-lg shadow-indigo-500/20",
            mutation.isPending
              ? "bg-indigo-500 text-white cursor-wait ring-2 ring-indigo-400/50"
              : "bg-indigo-600 text-white hover:bg-indigo-700"
          )}
        >
          {mutation.isPending ? <RefreshCcw size={18} className="animate-spin" /> : <Save size={18} />}
          {mutation.isPending ? '등록 중...' : '데이터베이스 등록하기'}
        </button>
      </div>
    </form>
  )
}

function FormField({ label, value, onChange, type = "text", placeholder = "", options = [] }: any) {
  return (
    <div className="space-y-1.5">
      <label className="text-[10px] font-bold text-slate-400 uppercase tracking-widest ml-1">{label}</label>
      {type === 'select' ? (
        <select 
          className="w-full bg-slate-800 border border-slate-800 rounded-xl px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 transition-all font-bold"
          value={value}
          onChange={e => onChange(e.target.value)}
        >
          {options.map((o: any) => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
      ) : (
        <input
          type={type}
          className="w-full bg-slate-800 border border-slate-800 rounded-xl px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 transition-all font-bold"
          value={value}
          onChange={e => onChange(e.target.value)}
          placeholder={placeholder}
        />
      )}
    </div>
  )
}

function QueriesModal({ instanceId, name, onClose }: { instanceId: number; name: string; onClose: () => void }) {
  const { data: queries = [], isLoading } = useQuery({
    queryKey: ['dpmQueries', instanceId],
    queryFn: () => getDPMQueries(instanceId),
  })

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-slate-900/60 backdrop-blur-sm" onClick={onClose} />
      <div className="bg-slate-900 w-full max-w-5xl max-h-[85vh] rounded-3xl shadow-2xl z-10 overflow-hidden flex flex-col border border-slate-800 animate-in zoom-in-95 duration-200">
        <div className="p-6 border-b border-slate-800 flex items-center justify-between bg-slate-800/50">
          <div className="flex items-center gap-3 text-slate-200">
            <div className="p-2 bg-slate-900 rounded-xl shadow-sm"><Zap size={20} className="text-amber-500" /></div>
            <div>
              <h3 className="font-bold text-lg leading-tight">{name}</h3>
              <p className="text-[10px] font-bold text-slate-400 tracking-wide mt-0.5">슬로우 쿼리 분석 · 상위 {queries.length}건</p>
            </div>
          </div>
          <button onClick={onClose} className="p-2 text-slate-400 hover:bg-slate-900 rounded-full hover:text-slate-100 transition-all border border-transparent hover:border-slate-800">
            <X size={20} />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-4 sm:p-6">
          {isLoading ? (
            <div className="flex flex-col items-center justify-center py-20 text-slate-400"><RefreshCcw size={48} className="mb-4 opacity-20 animate-spin" /><p className="font-medium">분석 데이터를 불러오는 중...</p></div>
          ) : queries.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-20 text-slate-400 border border-slate-800 border-dashed rounded-3xl">
              <Zap size={48} className="mb-4 opacity-10" />
              <p className="font-bold text-slate-500">슬로우 쿼리 내역이 없습니다.</p>
              <p className="text-[10px] font-bold text-slate-400 tracking-wide mt-1">pg_stat_statements 확장 설치 필요</p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-left border-separate border-spacing-y-1.5">
                <thead>
                  <tr className="text-[10px] font-bold text-slate-400 uppercase tracking-widest">
                    <th className="px-4 pb-2">Query Statement</th>
                    <th className="px-4 pb-2 text-right">Calls</th>
                    <th className="px-4 pb-2 text-right">Avg Time</th>
                    <th className="px-4 pb-2 text-right">Max Time</th>
                    <th className="px-4 pb-2 text-right">Total Time</th>
                  </tr>
                </thead>
                <tbody>
                  {queries.map((q, i) => (
                    <tr key={i} className="group transition-all">
                      <td className="bg-slate-800 group-hover:bg-slate-800/80 px-4 py-3 rounded-l-xl">
                        <code className="block text-[11px] font-mono text-slate-400 max-w-[500px] truncate group-hover:whitespace-normal group-hover:break-all transition-all" title={q.query_text}>
                          {q.query_text}
                        </code>
                      </td>
                      <td className="bg-slate-800 group-hover:bg-slate-800/80 px-4 py-3 text-right text-xs font-bold text-slate-400 font-mono">
                        {q.calls.toLocaleString()}
                      </td>
                      <td className={cn(
                        "bg-slate-800 group-hover:bg-slate-800/80 px-4 py-3 text-right text-xs font-bold font-mono",
                        q.mean_time_ms > 100 ? "text-rose-400" : "text-slate-400"
                      )}>
                        {formatMs(q.mean_time_ms)}
                      </td>
                      <td className="bg-slate-800 group-hover:bg-slate-800/80 px-4 py-3 text-right text-xs font-bold text-slate-500 font-mono">
                        {formatMs(q.max_time_ms)}
                      </td>
                      <td className="bg-slate-800 group-hover:bg-slate-800/80 px-4 py-3 rounded-r-xl text-right text-[11px] font-bold text-slate-400 font-mono">
                        {formatMs(q.total_time_ms)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

export default function DPMDashboard() {
  const queryClient = useQueryClient()
  const [showAdd, setShowAdd] = useState(false)
  const [queriesTarget, setQueriesTarget] = useState<{ id: number; name: string } | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<{ id: number; name: string } | null>(null)

  const { data: items = [], isLoading, refetch } = useQuery({
    queryKey: ['dpmStatus'],
    queryFn: getDPMStatus,
    refetchInterval: 30000,
  })

  const checkMutation = useMutation({
    mutationFn: triggerDPMCheck,
    onSuccess: () => refetch()
  })

  const deleteMutation = useMutation({
    mutationFn: deleteDPMInstance,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['dpmStatus'] })
    }
  })

  const handleDelete = (id: number, name: string) => {
    setDeleteTarget({ id, name })
  }

  return (
    <div className="space-y-6">
      <PageToolbar icon={Database} title="데이터베이스 성능 (DPM)" description="PostgreSQL 실시간 상태 및 슬로우 쿼리 분석">
        <button
          className="flex items-center gap-2 px-4 py-2.5 bg-slate-900 border border-slate-800 text-slate-400 rounded-xl text-sm font-bold hover:bg-slate-800 transition-all shadow-sm"
          onClick={() => refetch()}
          disabled={isLoading}
        >
          <RefreshCcw size={18} className={cn(isLoading && "animate-spin")} />
          새로고침
        </button>
        <button
          className={cn(
            "flex items-center gap-2 px-6 py-2.5 rounded-xl text-sm font-bold transition-all shadow-lg",
            showAdd ? "bg-slate-800 text-slate-400 shadow-none" : "bg-indigo-600 text-white hover:bg-indigo-700 shadow-indigo-500/20"
          )}
          onClick={() => setShowAdd(!showAdd)}
        >
          {showAdd ? <X size={18} /> : <Plus size={18} />}
          {showAdd ? '닫기' : 'DB 추가'}
        </button>
      </PageToolbar>

      {/* Add Instance Form */}
      {showAdd && (
        <AddInstanceForm onAdd={() => setShowAdd(false)} />
      )}

      {/* Instance Grid */}
      {isLoading && items.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-32 text-slate-400 animate-pulse">
          <RefreshCcw size={48} className="mb-4 opacity-20 animate-spin" />
          <p className="font-medium">데이터베이스 상태를 수집하는 중...</p>
        </div>
      ) : items.length === 0 && !showAdd ? (
        <div className="bg-slate-900 rounded-3xl border border-slate-800 border-dashed py-32 flex flex-col items-center justify-center text-slate-400 text-center px-6">
          <Database size={48} className="mb-4 opacity-20" />
          <p className="font-medium">등록된 데이터베이스가 없습니다.</p>
          <p className="text-xs opacity-70 mt-1">상단의 'DB 추가' 버튼으로 모니터링할 PostgreSQL 인스턴스를 등록하세요.</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4 gap-4">
          {items.map(item => (
            <InstanceCard
              key={item.instance.id}
              item={item}
              onDelete={() => handleDelete(item.instance.id, item.instance.name)}
              onCheck={() => checkMutation.mutate(item.instance.id)}
              onShowQueries={() => setQueriesTarget({ id: item.instance.id, name: item.instance.name })}
            />
          ))}
        </div>
      )}

      {queriesTarget && (
        <QueriesModal
          instanceId={queriesTarget.id}
          name={queriesTarget.name}
          onClose={() => setQueriesTarget(null)}
        />
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title="DB 인스턴스 삭제"
        message={`'${deleteTarget?.name}' 인스턴스를 삭제하시겠습니까? 수집된 통계도 함께 삭제됩니다.`}
        onConfirm={() => {
          if (deleteTarget) deleteMutation.mutate(deleteTarget.id)
          setDeleteTarget(null)
        }}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  )
}
