import React, { useState } from 'react'
import {
  getSyntheticStatus, addSyntheticMonitor, deleteSyntheticMonitor,
  triggerSyntheticCheck, getSyntheticHistory,
  type SyntheticStatusItem,
} from '../api/synthetic'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Globe, CheckCircle2, XCircle, Clock, Trash2, History, RefreshCcw, ExternalLink, Plus, X, Save, Activity, ShieldCheck } from 'lucide-react'
import { cn } from '../utils/cn'
import PageToolbar from './PageToolbar'

function formatLatency(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

function MonitorCard({
  item, onDelete, onCheck, onShowHistory,
}: {
  item: SyntheticStatusItem
  onDelete: () => void
  onCheck: () => void
  onShowHistory: () => void
}) {
  const m = item.monitor
  const latest = item.latest
  const stats = item.stats
  const isUp = latest?.success ?? false
  const hasChecked = !!latest

  return (
    <div className={cn(
      "bg-slate-900 rounded-3xl border p-5 shadow-premium transition-all group relative overflow-hidden",
      !hasChecked ? "border-slate-800" : isUp ? "border-slate-800" : "border-rose-500/30 ring-1 ring-rose-500/20"
    )}>
      {/* Header */}
      <div className="flex items-start justify-between gap-3 mb-6">
        <div className="flex items-center gap-3 min-w-0 flex-1">
          <div className={cn(
            "p-2 rounded-lg bg-slate-800 text-slate-400 group-hover:text-indigo-400 transition-colors shrink-0",
            isUp && hasChecked && "text-emerald-500 bg-emerald-500/10"
          )}>
            <Globe size={18} />
          </div>
          <div className="min-w-0 flex-1">
            <h4 className="font-bold text-slate-100 leading-tight truncate" title={m.name}>{m.name}</h4>
            <div className="flex items-center gap-1 mt-0.5 group/link min-w-0">
              <span className="text-[10px] font-bold text-slate-400 truncate font-mono" title={m.url}>{m.url}</span>
              <a href={m.url} target="_blank" rel="noreferrer" className="text-slate-400 hover:text-indigo-500 transition-colors shrink-0">
                <ExternalLink size={10} />
              </a>
            </div>
          </div>
        </div>
        
        <div className="flex items-center gap-1 shrink-0">
          <span className={cn(
            "inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-bold uppercase tracking-tight",
            !hasChecked ? "bg-slate-800 text-slate-500" : isUp ? "bg-emerald-500/15 text-emerald-300" : "bg-rose-500/15 text-rose-300"
          )}>
            {!hasChecked ? <Clock size={10} /> : isUp ? <CheckCircle2 size={10} /> : <XCircle size={10} />}
            {!hasChecked ? '대기' : isUp ? '정상' : '실패'}
          </span>
          <div className="flex opacity-0 group-hover:opacity-100 transition-opacity">
            <button onClick={onCheck} className="p-1 rounded-md text-slate-400 hover:bg-slate-800 hover:text-indigo-400 transition-all" title="즉시 체크"><RefreshCcw size={14} /></button>
            <button onClick={onShowHistory} className="p-1 rounded-md text-slate-400 hover:bg-slate-800 hover:text-indigo-400 transition-all" title="히스토리"><History size={14} /></button>
            <button onClick={onDelete} className="p-1 rounded-md text-slate-400 hover:bg-rose-500/10 hover:text-rose-500 transition-all" title="삭제"><Trash2 size={14} /></button>
          </div>
        </div>
      </div>

      <div className="space-y-4">
        {/* Latency */}
        <div className="flex items-baseline gap-1">
          <span className={cn(
            "text-3xl font-bold tracking-tight",
            !hasChecked ? "text-slate-400" : isUp ? "text-slate-100" : "text-rose-400"
          )}>
            {latest ? formatLatency(latest.response_time_ms) : '--'}
          </span>
          <span className="text-xs font-bold text-slate-400 uppercase tracking-widest ml-1">Latency</span>
        </div>

        {latest?.error && (
          <div className="bg-rose-500/10 border border-rose-500/20 rounded-lg p-2.5 text-[11px] font-bold text-rose-300 leading-tight">
            {latest.error}
          </div>
        )}

        {/* Stats Grid */}
        <div className="grid grid-cols-2 gap-3 py-3 border-y border-slate-800">
          <div className="space-y-0.5">
            <span className="text-[9px] font-bold text-slate-400 uppercase tracking-widest">가용성 (24h)</span>
            <div className={cn(
              "text-xs font-bold",
              stats.uptime_pct >= 99 ? "text-emerald-400" : stats.uptime_pct >= 95 ? "text-amber-400" : "text-rose-400"
            )}>
              {stats.uptime_pct.toFixed(2)}%
            </div>
          </div>
          <div className="space-y-0.5">
            <span className="text-[9px] font-bold text-slate-400 uppercase tracking-widest">평균 응답</span>
            <div className="text-xs font-bold text-slate-400">
              {formatLatency(Math.round(stats.avg_latency_ms))}
            </div>
          </div>
        </div>

        <div className="flex items-center justify-between text-[9px] font-bold text-slate-400 uppercase tracking-widest pt-1">
          <div className="flex items-center gap-1.5">
            <Clock size={10} />
            Check every {m.interval_seconds}s
          </div>
          {latest && (
            <div>Last: {new Date(latest.checked_at).toLocaleTimeString('ko-KR')}</div>
          )}
        </div>
      </div>
    </div>
  )
}

function AddMonitorForm({ onAdd }: { onAdd: () => void }) {
  const [form, setForm] = useState({
    name: '', url: '', method: 'GET', expected_status: 200,
    expected_keyword: '', interval_seconds: 60, timeout_seconds: 10, enabled: true,
  })
  const queryClient = useQueryClient()
  
  const mutation = useMutation({
    mutationFn: addSyntheticMonitor,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['syntheticStatus'] })
      onAdd()
    }
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!form.name || !form.url) return
    mutation.mutate(form)
  }

  return (
    <form onSubmit={handleSubmit} className="bg-slate-900 rounded-3xl border border-indigo-500/30 p-6 shadow-xl animate-in fade-in slide-in-from-top-4 duration-300 space-y-6">
      <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
        <FormField label="사이트 이름" value={form.name} onChange={(v: string) => setForm({...form, name: v})} placeholder="예: 학교 홈페이지" />
        <FormField label="사이트 주소 (URL)" value={form.url} onChange={(v: string) => setForm({...form, url: v})} placeholder="https://..." />
      </div>

      <div className="space-y-1.5">
        <label className="text-[10px] font-bold text-slate-400 uppercase tracking-widest ml-1">필수 포함 단어 (선택)</label>
        <input 
          className="w-full bg-slate-800 border border-slate-800 rounded-xl px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 transition-all"
          placeholder="페이지 내용 중 반드시 있어야 하는 단어 (예: 공지사항)"
          value={form.expected_keyword}
          onChange={e => setForm({...form, expected_keyword: e.target.value})}
        />
        <p className="text-[10px] text-slate-400 font-medium ml-1 italic">페이지는 열리지만 내용은 깨졌을 때(데이터베이스 연결 오류 등)를 감지하는 데 유용합니다.</p>
      </div>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 pt-2">
        <FormField label="Method" type="select" value={form.method} onChange={(v: string) => setForm({...form, method: v})} options={['GET', 'POST', 'HEAD']} />
        <FormField label="Status" type="number" value={form.expected_status} onChange={(v: string) => setForm({...form, expected_status: Number(v)})} />
        <FormField label="Interval (s)" type="number" value={form.interval_seconds} onChange={(v: string) => setForm({...form, interval_seconds: Number(v)})} />
        <FormField label="Timeout (s)" type="number" value={form.timeout_seconds} onChange={(v: string) => setForm({...form, timeout_seconds: Number(v)})} />
      </div>

      <div className="flex justify-end pt-2">
        <button 
          type="submit" 
          disabled={mutation.isPending}
          className="flex items-center gap-2 px-10 py-3 bg-indigo-600 text-white rounded-xl text-sm font-bold hover:bg-indigo-700 shadow-lg shadow-indigo-500/20 transition-all disabled:opacity-50"
        >
          {mutation.isPending ? <RefreshCcw size={18} className="animate-spin" /> : <Save size={18} />}
          모니터링 사이트 추가
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
          {options.map((o: string) => <option key={o} value={o}>{o}</option>)}
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

function HistoryModal({ monitorId, name, onClose }: { monitorId: number; name: string; onClose: () => void }) {
  const { data: history = [], isLoading } = useQuery({
    queryKey: ['syntheticHistory', monitorId],
    queryFn: () => getSyntheticHistory(monitorId, 100),
  })

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-slate-900/60 backdrop-blur-sm" onClick={onClose} />
      <div className="bg-slate-900 w-full max-w-3xl max-h-[85vh] rounded-3xl shadow-2xl z-10 overflow-hidden flex flex-col border border-slate-800 animate-in zoom-in-95 duration-200">
        <div className="p-6 border-b border-slate-800 flex items-center justify-between bg-slate-800/50">
          <div className="flex items-center gap-3 text-slate-200">
            <div className="p-2 bg-slate-900 rounded-xl shadow-sm"><History size={20} className="text-indigo-400" /></div>
            <div>
              <h3 className="font-bold text-lg leading-tight">{name}</h3>
              <p className="text-[10px] font-bold text-slate-400 uppercase tracking-widest mt-0.5">Check History · Recent 100 Items</p>
            </div>
          </div>
          <button onClick={onClose} className="p-2 text-slate-400 hover:bg-slate-900 rounded-full hover:text-slate-100 transition-all border border-transparent hover:border-slate-800">
            <X size={20} />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-4 sm:p-6">
          {isLoading ? (
            <div className="flex flex-col items-center justify-center py-20 text-slate-400"><RefreshCcw size={48} className="mb-4 opacity-20 animate-spin" /><p className="font-medium">기록을 불러오는 중...</p></div>
          ) : history.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-20 text-slate-400"><Activity size={48} className="mb-4 opacity-10" /><p className="font-medium">점검 이력이 없습니다.</p></div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-left border-separate border-spacing-y-1.5">
                <thead>
                  <tr className="text-[10px] font-bold text-slate-400 uppercase tracking-widest">
                    <th className="px-4 pb-2">Checked At</th>
                    <th className="px-4 pb-2">Status</th>
                    <th className="px-4 pb-2 text-right">Code</th>
                    <th className="px-4 pb-2 text-right">Latency</th>
                    <th className="px-4 pb-2">Notes</th>
                  </tr>
                </thead>
                <tbody>
                  {history.map((r, i) => (
                    <tr key={i} className="group transition-all">
                      <td className="bg-slate-800 group-hover:bg-slate-800/80 px-4 py-2.5 rounded-l-xl text-xs font-medium text-slate-500 font-mono">
                        {new Date(r.checked_at).toLocaleString('ko-KR', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })}
                      </td>
                      <td className="bg-slate-800 group-hover:bg-slate-800/80 px-4 py-2.5">
                        <span className={cn(
                          "px-2 py-0.5 rounded text-[10px] font-bold uppercase tracking-tight",
                          r.success ? "bg-emerald-500/15 text-emerald-300" : "bg-rose-500/15 text-rose-300"
                        )}>
                          {r.success ? 'Success' : 'Failure'}
                        </span>
                      </td>
                      <td className="bg-slate-800 group-hover:bg-slate-800/80 px-4 py-2.5 text-right text-xs font-bold text-slate-400">
                        {r.status_code || '-'}
                      </td>
                      <td className="bg-slate-800 group-hover:bg-slate-800/80 px-4 py-2.5 text-right text-xs font-bold text-slate-400">
                        {formatLatency(r.response_time_ms)}
                      </td>
                      <td className="bg-slate-800 group-hover:bg-slate-800/80 px-4 py-2.5 rounded-r-xl max-w-[200px] truncate text-[11px] font-medium text-rose-500 italic">
                        {r.error || ''}
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

export default function SyntheticDashboard() {
  const queryClient = useQueryClient()
  const [showAdd, setShowAdd] = useState(false)
  const [historyTarget, setHistoryTarget] = useState<{ id: number; name: string } | null>(null)

  const { data: items = [], isLoading, refetch } = useQuery({
    queryKey: ['syntheticStatus'],
    queryFn: getSyntheticStatus,
    refetchInterval: 30000,
  })

  const checkMutation = useMutation({
    mutationFn: triggerSyntheticCheck,
    onSuccess: () => refetch()
  })

  const deleteMutation = useMutation({
    mutationFn: deleteSyntheticMonitor,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['syntheticStatus'] })
    }
  })

  const handleDelete = (id: number) => {
    if (!confirm('정말 삭제하시겠습니까?')) return
    deleteMutation.mutate(id)
  }

  return (
    <div className="space-y-6">
      <PageToolbar icon={Activity} title="사이트 점검 (Synthetic)" description="학교 홈페이지 및 웹 서비스 상태 주기적 감시">
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
          {showAdd ? '닫기' : '사이트 추가'}
        </button>
      </PageToolbar>

      {/* Add Site Form */}
      {showAdd && (
        <AddMonitorForm onAdd={() => setShowAdd(false)} />
      )}

      {/* Monitor Grid */}
      {isLoading && items.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-32 text-slate-400 animate-pulse">
          <RefreshCcw size={48} className="mb-4 opacity-20 animate-spin" />
          <p className="font-medium">웹 서비스 상태를 확인하는 중...</p>
        </div>
      ) : items.length === 0 && !showAdd ? (
        <div className="bg-slate-900 rounded-3xl border border-slate-800 border-dashed py-32 flex flex-col items-center justify-center text-slate-400 text-center px-6">
          <ShieldCheck size={48} className="mb-4 opacity-20" />
          <p className="font-medium">등록된 점검 대상 사이트가 없습니다.</p>
          <p className="text-xs opacity-70 mt-1">상단의 '사이트 추가' 버튼으로 학교 홈페이지나 업무 시스템 URL을 등록하세요.</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4 gap-4">
          {items.map(item => (
            <MonitorCard
              key={item.monitor.id}
              item={item}
              onDelete={() => handleDelete(item.monitor.id)}
              onCheck={() => checkMutation.mutate(item.monitor.id)}
              onShowHistory={() => setHistoryTarget({ id: item.monitor.id, name: item.monitor.name })}
            />
          ))}
        </div>
      )}

      {historyTarget && (
        <HistoryModal
          monitorId={historyTarget.id}
          name={historyTarget.name}
          onClose={() => setHistoryTarget(null)}
        />
      )}
    </div>
  )
}
