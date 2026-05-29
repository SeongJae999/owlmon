import React, { useState } from 'react'
import {
  getSSLDomains, getSSLStatus, addSSLDomain, deleteSSLDomain, triggerSSLCheck,
  type SSLCertStatus,
} from '../api/ssl'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { ShieldCheck, Globe, Trash2, ShieldAlert, CheckCircle2, AlertTriangle, RefreshCcw, Plus, X, Info, Save, ExternalLink, Clock } from 'lucide-react'
import { cn } from '../utils/cn'
import PageToolbar from './PageToolbar'
import ConfirmDialog from './ConfirmDialog'

const STATUS_CONFIG: Record<string, { bg: string, text: string, label: string, icon: any }> = {
  ok:       { bg: 'bg-emerald-500/15', text: 'text-emerald-300', label: '정상',      icon: CheckCircle2 },
  notice:   { bg: 'bg-sky-500/15',     text: 'text-sky-300',     label: '갱신 준비', icon: Clock },
  warning:  { bg: 'bg-amber-500/15',   text: 'text-amber-300',   label: '갱신 임박', icon: AlertTriangle },
  critical: { bg: 'bg-rose-500/15',    text: 'text-rose-300',    label: '긴급 갱신', icon: ShieldAlert },
  expired:  { bg: 'bg-rose-500/15',    text: 'text-rose-300',    label: '만료됨',    icon: ShieldAlert },
  error:    { bg: 'bg-slate-800',      text: 'text-slate-400',   label: '연결 실패', icon: Info },
}

function CertCard({ cert, onDelete }: { cert: SSLCertStatus & { id?: number; memo?: string }; onDelete?: () => void }) {
  const cfg = STATUS_CONFIG[cert.status] ?? STATUS_CONFIG.error

  return (
    <div className={cn(
      "bg-slate-900 rounded-3xl border p-5 shadow-premium transition-all group relative overflow-hidden",
      cert.status === 'expired' || cert.status === 'critical' ? "border-rose-500/30 ring-1 ring-rose-500/20" :
      cert.status === 'warning' ? "border-amber-500/30 ring-1 ring-amber-500/20" :
      cert.status === 'notice' ? "border-sky-500/30 ring-1 ring-sky-500/20" :
      "border-slate-800"
    )}>
      {/* Header */}
      <div className="flex items-start justify-between gap-2 mb-6">
        <div className="flex items-center gap-3 min-w-0 flex-1">
          <div className={cn(
            "p-2 rounded-lg bg-slate-800 text-slate-400 group-hover:text-indigo-400 transition-colors shrink-0",
            cert.status === 'ok' && "text-emerald-500 bg-emerald-500/10"
          )}>
            <Globe size={18} />
          </div>
          <div className="min-w-0">
            <a
              href={`https://${cert.domain}${cert.port && cert.port !== 443 ? `:${cert.port}` : ''}/`}
              target="_blank"
              rel="noopener noreferrer"
              className="group/link inline-flex items-center gap-1 max-w-full font-bold text-slate-100 leading-tight hover:text-indigo-400 transition-colors"
              title={`${cert.domain} 새 탭에서 열기`}
              onClick={(e) => e.stopPropagation()}
            >
              <span className="truncate">{cert.domain}</span>
              <ExternalLink size={12} className="shrink-0 text-slate-500 group-hover/link:text-indigo-400 transition-colors" />
            </a>
            {cert.memo && <p className="text-[11px] text-slate-500 font-medium mt-0.5 truncate" title={cert.memo}>{cert.memo}</p>}
          </div>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <span className={cn(
            "inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-bold uppercase tracking-tight whitespace-nowrap",
            cfg.bg, cfg.text
          )}>
            <cfg.icon size={10} />
            {cfg.label}
          </span>
          {onDelete && (
            <button 
              className="p-1.5 rounded-lg text-slate-400 hover:text-rose-500 hover:bg-rose-500/10 transition-all"
              onClick={onDelete}
            >
              <Trash2 size={16} />
            </button>
          )}
        </div>
      </div>

      {cert.status === 'error' ? (
        <div className="bg-slate-800 border border-slate-800 rounded-xl p-4 flex items-center gap-3 text-slate-400 text-xs font-bold">
          <Info size={16} className="shrink-0 text-slate-400" />
          <p>{cert.error}</p>
        </div>
      ) : (
        <div className="space-y-4">
          <div className="flex items-baseline gap-1">
            <span className={cn(
              "text-3xl font-bold tracking-tight",
              cert.days_left <= 7 ? "text-rose-400"
                : cert.days_left <= 30 ? "text-amber-400"
                : cert.days_left <= 60 ? "text-sky-400"
                : "text-emerald-400"
            )}>
              {cert.days_left}
            </span>
            <span className="text-sm font-bold text-slate-400">일 남음</span>
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between text-[11px] font-bold">
              <span className="text-slate-400 tracking-wide">만료일</span>
              <span className="text-slate-400 font-mono">{new Date(cert.not_after).toLocaleDateString('ko-KR')}</span>
            </div>
            <div className="flex items-center justify-between text-[11px] font-bold">
              <span className="text-slate-400 tracking-wide" title="SSL 인증서를 발급한 인증기관 (CA)">발급기관</span>
              <span className="text-slate-400 truncate max-w-[140px] text-right" title={cert.issuer}>{cert.issuer}</span>
            </div>
          </div>
        </div>
      )}

      {cert.checked_at && (
        <div className="mt-4 pt-3 border-t border-slate-800 flex justify-end">
          <div className="text-[10px] font-bold text-slate-400 tracking-wide">
            최근 확인: {new Date(cert.checked_at).toLocaleTimeString('ko-KR')}
          </div>
        </div>
      )}
    </div>
  )
}

export default function SSLDashboard() {
  const queryClient = useQueryClient()
  const [showAdd, setShowAdd] = useState(false)
  const [form, setForm] = useState({ domain: '', port: 443, memo: '' })
  const [deleteTarget, setDeleteTarget] = useState<{ id: number; domain: string } | null>(null)

  const { data: domains = [], isLoading: domainsLoading } = useQuery({
    queryKey: ['sslDomains'],
    queryFn: getSSLDomains,
  })

  const { data: statuses = [] } = useQuery({
    queryKey: ['sslStatus'],
    queryFn: getSSLStatus,
  })

  const checkMutation = useMutation({
    mutationFn: triggerSSLCheck,
    onSuccess: (newStatus) => {
      queryClient.setQueryData(['sslStatus'], newStatus)
    }
  })

  const addMutation = useMutation({
    mutationFn: addSSLDomain,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['sslDomains'] })
      // 등록 직후 즉시 SSL 체크 트리거 — 6시간 주기 대기하지 않음
      checkMutation.mutate()
      setShowAdd(false)
      setForm({ domain: '', port: 443, memo: '' })
    }
  })

  const deleteMutation = useMutation({
    mutationFn: deleteSSLDomain,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['sslDomains'] })
      queryClient.invalidateQueries({ queryKey: ['sslStatus'] })
    }
  })

  // URL 붙여넣기 정규화 — "https://example.com:8443/path" → domain "example.com", port 8443
  const normalizeDomain = (raw: string): { domain: string; port?: number } => {
    let s = raw.trim()
    if (!s) return { domain: '' }
    // 프로토콜 제거
    s = s.replace(/^[a-z][a-z0-9+.-]*:\/\//i, '')
    // 경로/쿼리/프래그먼트 제거
    s = s.split('/')[0].split('?')[0].split('#')[0]
    // user:pass@host 제거
    s = s.split('@').pop() || s
    // 포트 추출
    const m = s.match(/^([^:]+)(?::(\d+))?$/)
    if (!m) return { domain: s }
    const port = m[2] ? parseInt(m[2], 10) : undefined
    return { domain: m[1], port }
  }

  const handleDomainChange = (raw: string) => {
    const { domain, port } = normalizeDomain(raw)
    setForm(prev => ({ ...prev, domain, port: port ?? prev.port }))
  }

  const handleAdd = (e: React.FormEvent) => {
    e.preventDefault()
    if (!form.domain) return
    addMutation.mutate(form)
  }

  const handleDelete = (id: number, domain: string) => {
    setDeleteTarget({ id, domain })
  }

  const statusMap = new Map(statuses.map(s => [s.domain, s]))

  return (
    <div className="space-y-6">
      <PageToolbar icon={ShieldCheck} title="SSL 인증서 모니터링" description="HTTPS 웹사이트 및 서비스 보안 관리">
        <button
          className="flex items-center gap-2 px-4 py-2.5 bg-slate-900 border border-slate-800 text-slate-400 rounded-xl text-sm font-bold hover:bg-slate-800 transition-all shadow-sm"
          onClick={() => checkMutation.mutate()}
          disabled={checkMutation.isPending}
        >
          {checkMutation.isPending ? <RefreshCcw size={18} className="animate-spin" /> : <RefreshCcw size={18} />}
          즉시 체크
        </button>
        <button
          className={cn(
            "flex items-center gap-2 px-6 py-2.5 rounded-xl text-sm font-bold transition-all shadow-lg",
            showAdd ? "bg-slate-800 text-slate-400 shadow-none" : "bg-indigo-600 text-white hover:bg-indigo-700 shadow-indigo-500/20"
          )}
          onClick={() => setShowAdd(!showAdd)}
        >
          {showAdd ? <X size={18} /> : <Plus size={18} />}
          {showAdd ? '닫기' : '도메인 추가'}
        </button>
      </PageToolbar>

      {/* Add Domain Form */}
      {showAdd && (
        <form onSubmit={handleAdd} className="bg-slate-900 rounded-3xl border border-indigo-500/30 p-6 shadow-xl animate-in fade-in slide-in-from-top-4 duration-300 space-y-6">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div className="space-y-1.5">
              <label className="text-[10px] font-bold text-slate-400 tracking-wide ml-1">도메인 주소</label>
              <input
                className="w-full bg-slate-800 border border-slate-800 rounded-xl px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition-all font-medium"
                placeholder="예: school.go.kr"
                value={form.domain}
                onChange={e => handleDomainChange(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-[10px] font-bold text-slate-400 tracking-wide ml-1">포트</label>
              <input
                className="w-full bg-slate-800 border border-slate-800 rounded-xl px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition-all font-medium"
                type="number"
                placeholder="443"
                value={form.port}
                onChange={e => setForm({...form, port: Number(e.target.value)})}
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-[10px] font-bold text-slate-400 tracking-wide ml-1">메모</label>
              <input
                className="w-full bg-slate-800 border border-slate-800 rounded-xl px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition-all font-medium"
                placeholder="예: 메인 홈페이지"
                value={form.memo}
                onChange={e => setForm({...form, memo: e.target.value})}
              />
            </div>
          </div>
          <div className="flex justify-end gap-3 pt-2">
            <button
              type="submit"
              disabled={addMutation.isPending}
              className={cn(
                "flex items-center gap-2 px-10 py-3 rounded-xl text-sm font-bold transition-all shadow-lg shadow-indigo-500/20",
                addMutation.isPending
                  ? "bg-indigo-500 text-white cursor-wait ring-2 ring-indigo-400/50"
                  : "bg-indigo-600 text-white hover:bg-indigo-700"
              )}
            >
              {addMutation.isPending ? <RefreshCcw size={18} className="animate-spin" /> : <Save size={18} />}
              {addMutation.isPending ? '등록 중...' : '도메인 등록하기'}
            </button>
          </div>
        </form>
      )}

      {/* Domain Grid */}
      {domainsLoading ? (
        <div className="flex flex-col items-center justify-center py-32 text-slate-400 animate-pulse">
          <RefreshCcw size={48} className="mb-4 opacity-20 animate-spin" />
          <p className="font-medium">SSL 정보를 불러오는 중...</p>
        </div>
      ) : domains.length === 0 && !showAdd ? (
        <div className="bg-slate-900 rounded-3xl border border-slate-800 border-dashed py-32 flex flex-col items-center justify-center text-slate-400 text-center px-6">
          <ShieldCheck size={48} className="mb-4 opacity-20" />
          <p className="font-medium">모니터링할 도메인이 없습니다.</p>
          <p className="text-xs opacity-70 mt-1">상단의 '도메인 추가' 버튼을 눌러 정보를 입력하세요.</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4 gap-4">
          {domains.map(dom => {
            const cert = statusMap.get(dom.domain)
            if (!cert) return (
              <div key={dom.id} className="bg-slate-900 rounded-3xl border border-indigo-500/30 ring-1 ring-indigo-500/20 p-5 shadow-premium relative overflow-hidden">
                {/* 진행 중 임을 알리는 indigo glow */}
                <div className="absolute inset-x-0 top-0 h-0.5 bg-gradient-to-r from-transparent via-indigo-400 to-transparent animate-pulse" />
                <div className="flex items-center gap-3 mb-4">
                  <div className="p-2 rounded-lg bg-indigo-500/15 text-indigo-300">
                    <RefreshCcw size={18} className="animate-spin" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="font-bold text-slate-100 leading-tight truncate" title={dom.domain}>{dom.domain}</div>
                    <div className="text-[11px] text-indigo-300 font-medium mt-0.5">SSL 인증서 확인 중…</div>
                  </div>
                </div>
                <div className="space-y-2 animate-pulse">
                  <div className="h-2 w-3/4 bg-slate-700 rounded" />
                  <div className="h-2 w-1/2 bg-slate-700 rounded" />
                </div>
              </div>
            )
            return (
              <CertCard
                key={dom.id}
                cert={{ ...cert, id: dom.id, memo: dom.memo }}
                onDelete={() => handleDelete(dom.id, dom.domain)}
              />
            )
          })}
        </div>
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title="도메인 삭제"
        message={`'${deleteTarget?.domain}' 도메인을 삭제하시겠습니까? 모니터링이 즉시 중단됩니다.`}
        onConfirm={() => {
          if (deleteTarget) deleteMutation.mutate(deleteTarget.id)
          setDeleteTarget(null)
        }}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  )
}
