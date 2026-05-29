import React, { useState, useMemo, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import {
  ShieldCheck, Globe, Trash2, Pencil, ShieldAlert, CheckCircle2, AlertTriangle, RefreshCcw,
  Plus, X, Info, Save, ExternalLink, Clock, Activity, Zap,
} from 'lucide-react'
import {
  getSSLDomains, getSSLStatus, addSSLDomain, deleteSSLDomain, triggerSSLCheck, updateSSLDomain,
  type SSLCertStatus, type SSLDomain,
} from '../api/ssl'
import {
  getSyntheticStatus, addSyntheticMonitor, deleteSyntheticMonitor, updateSyntheticMonitor, triggerSyntheticCheck,
  type SyntheticStatusItem,
} from '../api/synthetic'
import { cn } from '../utils/cn'
import PageToolbar from './PageToolbar'
import ConfirmDialog from './ConfirmDialog'

/**
 * 웹사이트 모니터링 — SSL 인증서 + 사이트 가용성 통합 페이지
 *
 * 도메인 한 번 등록하면 SSL + Synthetic 둘 다 자동 등록.
 * 카드 한 장으로 인증서 상태 + 사이트 살아있는지 + 응답 시간 모두 표시.
 */

const STATUS_CONFIG: Record<string, { bg: string, text: string, label: string, icon: any }> = {
  ok:       { bg: 'bg-emerald-500/15', text: 'text-emerald-300', label: '정상',      icon: CheckCircle2 },
  notice:   { bg: 'bg-sky-500/15',     text: 'text-sky-300',     label: '갱신 준비', icon: Clock },
  warning:  { bg: 'bg-amber-500/15',   text: 'text-amber-300',   label: '갱신 임박', icon: AlertTriangle },
  critical: { bg: 'bg-rose-500/15',    text: 'text-rose-300',    label: '긴급 갱신', icon: ShieldAlert },
  expired:  { bg: 'bg-rose-500/15',    text: 'text-rose-300',    label: '만료됨',    icon: ShieldAlert },
  error:    { bg: 'bg-slate-800',      text: 'text-slate-400',   label: '연결 실패', icon: Info },
}

// URL 정규화 (붙여넣기용)
function normalizeDomain(raw: string): { domain: string; port?: number } {
  let s = raw.trim()
  if (!s) return { domain: '' }
  s = s.replace(/^[a-z][a-z0-9+.-]*:\/\//i, '')
  s = s.split('/')[0].split('?')[0].split('#')[0]
  s = s.split('@').pop() || s
  const m = s.match(/^([^:]+)(?::(\d+))?$/)
  if (!m) return { domain: s }
  return { domain: m[1], port: m[2] ? parseInt(m[2], 10) : undefined }
}

// Synthetic URL → 도메인 매칭 (https://school.go.kr/ → school.go.kr)
function urlToDomain(url: string): string {
  try {
    return new URL(url).hostname
  } catch {
    return url
  }
}

export default function WebsiteMonitoring() {
  const queryClient = useQueryClient()
  const [showAdd, setShowAdd] = useState(false)
  const [form, setForm] = useState({ domain: '', port: 443, memo: '', includeSynthetic: true })
  const [deleteTarget, setDeleteTarget] = useState<{ sslId: number; synId?: number; domain: string } | null>(null)
  const [editTarget, setEditTarget] = useState<{ ssl: SSLDomain; syn?: SyntheticStatusItem } | null>(null)

  const { data: sslDomains = [], isLoading: sslLoading } = useQuery({
    queryKey: ['sslDomains'],
    queryFn: getSSLDomains,
    staleTime: 60_000,
    placeholderData: keepPreviousData,
  })
  const { data: sslStatuses = [] } = useQuery({
    queryKey: ['sslStatus'],
    queryFn: getSSLStatus,
    staleTime: 60_000,
    placeholderData: keepPreviousData,
  })
  const { data: synItems = [] } = useQuery({
    queryKey: ['syntheticStatus'],
    queryFn: getSyntheticStatus,
    refetchInterval: 30_000,
    placeholderData: keepPreviousData,
  })

  // 메모이즈 — 다른 쿼리 refetch 시 불필요한 재계산 방지
  const sslStatusMap = useMemo(
    () => new Map(sslStatuses.map(s => [s.domain, s])),
    [sslStatuses],
  )
  const synByDomain = useMemo(() => {
    const m = new Map<string, SyntheticStatusItem>()
    synItems.forEach(item => {
      const dom = urlToDomain(item.monitor.url)
      if (!m.has(dom)) m.set(dom, item)
    })
    return m
  }, [synItems])

  const checkSSLMutation = useMutation({
    mutationFn: triggerSSLCheck,
    onSuccess: (newStatus) => queryClient.setQueryData(['sslStatus'], newStatus),
  })

  // 서버 재시작 후엔 SSL 캐시가 비어있음 — 등록된 도메인은 있는데 status가 비어있으면 자동 체크
  useEffect(() => {
    if (sslDomains.length > 0 && sslStatuses.length === 0 && !checkSSLMutation.isPending) {
      checkSSLMutation.mutate()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sslDomains.length, sslStatuses.length])

  const addMutation = useMutation({
    mutationFn: async (f: typeof form) => {
      const ssl = await addSSLDomain({ domain: f.domain, port: f.port, memo: f.memo })
      if (f.includeSynthetic) {
        try {
          await addSyntheticMonitor({
            name: f.memo || f.domain,
            url: `https://${f.domain}${f.port && f.port !== 443 ? `:${f.port}` : ''}/`,
            method: 'GET',
            expected_status: 200,
            expected_keyword: '',
            interval_seconds: 300,
            timeout_seconds: 10,
            enabled: true,
          })
        } catch (e) {
          console.warn('Synthetic 등록 실패 (SSL은 성공)', e)
        }
      }
      return ssl
    },
    onSuccess: async () => {
      // refetchQueries로 즉시 강제 refetch (invalidate는 stale 표시만)
      await Promise.all([
        queryClient.refetchQueries({ queryKey: ['sslDomains'] }),
        queryClient.refetchQueries({ queryKey: ['syntheticStatus'] }),
      ])
      checkSSLMutation.mutate()
      setShowAdd(false)
      setForm({ domain: '', port: 443, memo: '', includeSynthetic: true })
    },
  })

  const addSynMutation = useMutation({
    mutationFn: async ({ domain, port, memo }: { domain: string; port: number; memo: string }) => {
      const url = `https://${domain}${port && port !== 443 ? `:${port}` : ''}/`
      return await addSyntheticMonitor({
        name: memo || domain,
        url,
        method: 'GET',
        expected_status: 200,
        expected_keyword: '',
        interval_seconds: 300,
        timeout_seconds: 10,
        enabled: true,
      })
    },
    onSuccess: async () => {
      // 캐시 무효화 + 즉시 refetch — 카드 바로 업데이트
      await queryClient.invalidateQueries({ queryKey: ['syntheticStatus'] })
      await queryClient.refetchQueries({ queryKey: ['syntheticStatus'], type: 'all' })
    },
    onError: (err: any) => {
      const msg = err?.response?.data || err?.message || '사이트 점검 추가 실패'
      alert(`점검 추가 실패: ${msg}`)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: async (t: { sslId: number; synId?: number }) => {
      await deleteSSLDomain(t.sslId)
      if (t.synId !== undefined) {
        try { await deleteSyntheticMonitor(t.synId) } catch (e) { console.warn('Synthetic 삭제 실패', e) }
      }
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.refetchQueries({ queryKey: ['sslDomains'] }),
        queryClient.refetchQueries({ queryKey: ['sslStatus'] }),
        queryClient.refetchQueries({ queryKey: ['syntheticStatus'] }),
      ])
    },
  })

  const handleAdd = (e: React.FormEvent) => {
    e.preventDefault()
    if (!form.domain) return
    addMutation.mutate(form)
  }

  const handleDomainChange = (raw: string) => {
    const { domain, port } = normalizeDomain(raw)
    setForm(prev => ({ ...prev, domain, port: port ?? prev.port }))
  }

  return (
    <div className="space-y-6">
      <PageToolbar
        icon={Globe}
        title="웹사이트 모니터링"
        description="SSL 인증서 + 사이트 가용성 통합 감시"
      >
        {sslStatuses.length > 0 && sslStatuses[0].checked_at && (
          <span className="text-[11px] font-medium text-slate-500 mr-2 hidden sm:inline-flex items-center gap-1">
            <Clock size={11} /> 마지막 체크 {new Date(sslStatuses[0].checked_at).toLocaleTimeString('ko-KR')}
          </span>
        )}
        <button
          className="flex items-center gap-2 px-4 py-2.5 bg-slate-900 border border-slate-800 text-slate-400 rounded-xl text-sm font-bold hover:bg-slate-800 transition-all shadow-sm"
          onClick={() => checkSSLMutation.mutate()}
          disabled={checkSSLMutation.isPending}
        >
          <RefreshCcw size={18} className={cn(checkSSLMutation.isPending && "animate-spin")} />
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
          {showAdd ? '닫기' : '웹사이트 추가'}
        </button>
      </PageToolbar>

      {/* Add Form */}
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
                inputMode="numeric"
                value={form.port || ''}
                onChange={e => setForm({...form, port: e.target.value === '' ? 0 : Number(e.target.value)})}
                onBlur={e => {
                  if (!e.target.value.trim() || Number(e.target.value) <= 0) setForm(prev => ({...prev, port: 443}))
                }}
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

          {/* Synthetic 함께 등록 토글 */}
          <label className="flex items-center gap-2 cursor-pointer select-none">
            <input
              type="checkbox"
              checked={form.includeSynthetic}
              onChange={e => setForm({...form, includeSynthetic: e.target.checked})}
              className="w-4 h-4 accent-indigo-500"
            />
            <span className="text-sm text-slate-300">
              사이트 가용성도 함께 점검 <span className="text-slate-500 text-xs">(5분 주기로 사이트가 살아있는지 확인)</span>
            </span>
          </label>

          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={() => setShowAdd(false)}
              disabled={addMutation.isPending}
              className="px-6 py-3 rounded-xl text-sm font-bold bg-slate-800 text-slate-300 border border-slate-700 hover:bg-slate-700 transition-colors disabled:opacity-50"
            >
              취소
            </button>
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
              {addMutation.isPending ? '등록 중...' : '등록하기'}
            </button>
          </div>
        </form>
      )}

      {/* Domain Cards */}
      {sslLoading ? (
        <div className="flex flex-col items-center justify-center py-32 text-slate-400 animate-pulse">
          <RefreshCcw size={48} className="mb-4 opacity-20 animate-spin" />
          <p className="font-medium">불러오는 중...</p>
        </div>
      ) : sslDomains.length === 0 && !showAdd ? (
        <div className="bg-slate-900 rounded-3xl border border-slate-800 border-dashed py-32 flex flex-col items-center justify-center text-slate-400 text-center px-6">
          <Globe size={48} className="mb-4 opacity-20" />
          <p className="font-medium">등록된 웹사이트가 없습니다.</p>
          <p className="text-xs opacity-70 mt-1">상단의 '웹사이트 추가' 버튼을 눌러 등록하세요.</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4 gap-4">
          {sslDomains.map(dom => {
            const ssl = sslStatusMap.get(dom.domain)
            const syn = synByDomain.get(dom.domain)
            return (
              <WebsiteCard
                key={dom.id}
                domain={dom.domain}
                port={dom.port}
                memo={dom.memo}
                ssl={ssl}
                syn={syn}
                addingSyn={addSynMutation.isPending && addSynMutation.variables?.domain === dom.domain}
                onDelete={() => setDeleteTarget({ sslId: dom.id, synId: syn?.monitor.id, domain: dom.domain })}
                onEdit={() => setEditTarget({ ssl: dom, syn })}
                onAddSyn={() => addSynMutation.mutate({ domain: dom.domain, port: dom.port, memo: dom.memo })}
                onCheckSyn={syn ? () => triggerSyntheticCheck(syn.monitor.id).then(() => queryClient.refetchQueries({ queryKey: ['syntheticStatus'] })) : undefined}
              />
            )
          })}
        </div>
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title="웹사이트 삭제"
        message={`'${deleteTarget?.domain}'을(를) 삭제하시겠습니까? SSL 모니터링과 사이트 점검이 모두 중단됩니다.`}
        onConfirm={() => {
          if (deleteTarget) deleteMutation.mutate({ sslId: deleteTarget.sslId, synId: deleteTarget.synId })
          setDeleteTarget(null)
        }}
        onCancel={() => setDeleteTarget(null)}
      />

      {editTarget && (
        <EditModal
          ssl={editTarget.ssl}
          syn={editTarget.syn}
          onClose={() => setEditTarget(null)}
          onSaved={() => {
            queryClient.invalidateQueries({ queryKey: ['sslDomains'] })
            queryClient.invalidateQueries({ queryKey: ['syntheticStatus'] })
            setEditTarget(null)
          }}
        />
      )}
    </div>
  )
}

// ─── 수정 모달 ────────────────────────────────────
function EditModal({
  ssl, syn, onClose, onSaved,
}: {
  ssl: SSLDomain
  syn?: SyntheticStatusItem
  onClose: () => void
  onSaved: () => void
}) {
  const [memo, setMemo] = useState(ssl.memo)
  const initialMin = String((syn?.monitor.interval_seconds ?? 300) / 60)
  const [synIntervalStr, setSynIntervalStr] = useState(initialMin) // 입력 중 빈 칸 허용 위해 문자열
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleSave = async () => {
    setSaving(true)
    setError(null)
    try {
      if (memo !== ssl.memo) {
        await updateSSLDomain(ssl.id, memo)
      }
      if (syn) {
        const parsed = Number(synIntervalStr)
        const minutes = Number.isFinite(parsed) && parsed > 0 ? parsed : 5 // 빈 칸/0 → 기본 5분
        const intervalSec = Math.max(60, Math.round(minutes * 60))
        if (intervalSec !== syn.monitor.interval_seconds || !syn.monitor.enabled) {
          await updateSyntheticMonitor(syn.monitor.id, {
            ...syn.monitor,
            enabled: true,
            interval_seconds: intervalSec,
          })
        }
      }
      onSaved()
    } catch (e: any) {
      console.error(e)
      const msg = e?.response?.data?.toString?.().trim() || e?.message || '저장 실패'
      setError(msg)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm" onClick={onClose}>
      <div
        className="bg-slate-900 border border-slate-800 rounded-2xl shadow-2xl w-full max-w-md mx-4"
        onClick={e => e.stopPropagation()}
      >
        <div className="flex items-center justify-between p-5 pb-3">
          <div className="flex items-center gap-3 min-w-0">
            <div className="p-2 rounded-lg bg-indigo-500/15 text-indigo-400">
              <Pencil size={18} />
            </div>
            <div className="min-w-0">
              <h3 className="text-base font-bold text-slate-100">웹사이트 수정</h3>
              <p className="text-xs text-slate-500 truncate" title={ssl.domain}>{ssl.domain}</p>
            </div>
          </div>
          <button onClick={onClose} className="p-1 rounded-md text-slate-500 hover:text-slate-300 hover:bg-slate-800 transition-colors">
            <X size={16} />
          </button>
        </div>

        <div className="px-5 pb-5 space-y-4">
          <div className="space-y-1.5">
            <label className="text-[10px] font-bold text-slate-400 tracking-wide ml-1">메모</label>
            <input
              className="w-full bg-slate-800 border border-slate-700 rounded-xl px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition-all"
              value={memo}
              onChange={e => setMemo(e.target.value)}
              placeholder="예: 메인 홈페이지"
            />
          </div>

          {syn ? (
            <div className="space-y-1.5">
              <label className="text-[10px] font-bold text-slate-400 tracking-wide ml-1">사이트 점검 주기 (분)</label>
              <input
                type="number"
                min={1}
                inputMode="numeric"
                className="w-full bg-slate-800 border border-slate-700 rounded-xl px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition-all tabular-nums"
                value={synIntervalStr}
                onChange={e => setSynIntervalStr(e.target.value)}
                onBlur={e => {
                  // 빈 칸이면 기본값 5로 복구
                  if (!e.target.value.trim() || Number(e.target.value) <= 0) setSynIntervalStr('5')
                }}
              />
              <p className="text-[10px] text-slate-500 ml-1">권장: 5분 (1분 = 잦은 알림, 30분 = 장애 감지 늦음)</p>
            </div>
          ) : (
            <p className="text-xs text-slate-500 italic">사이트 점검 미등록 — 카드의 '점검 추가' 버튼으로 등록</p>
          )}

          {error && (
            <div className="flex items-start gap-2 px-3 py-2 rounded-lg bg-rose-500/10 border border-rose-500/30 text-xs">
              <AlertTriangle size={14} className="shrink-0 text-rose-400 mt-0.5" />
              <span className="text-rose-300 font-medium">{error}</span>
            </div>
          )}
        </div>

        <div className="flex gap-2 px-5 pb-5 justify-end">
          <button
            onClick={onClose}
            className="px-4 py-2 rounded-lg text-sm font-semibold bg-slate-800 text-slate-300 border border-slate-700 hover:bg-slate-700 transition-colors"
          >
            취소
          </button>
          <button
            onClick={handleSave}
            disabled={saving}
            className={cn(
              "px-4 py-2 rounded-lg text-sm font-semibold text-white transition-colors flex items-center gap-1.5",
              saving ? "bg-indigo-500 cursor-wait" : "bg-indigo-600 hover:bg-indigo-700"
            )}
          >
            {saving ? <RefreshCcw size={14} className="animate-spin" /> : <Save size={14} />}
            저장
          </button>
        </div>
      </div>
    </div>
  )
}

// ─── 통합 카드 ────────────────────────────────────
function WebsiteCard({
  domain, port, memo, ssl, syn, addingSyn, onDelete, onEdit, onAddSyn, onCheckSyn,
}: {
  domain: string
  port: number
  memo: string
  ssl?: SSLCertStatus
  syn?: SyntheticStatusItem
  addingSyn: boolean
  onDelete: () => void
  onEdit: () => void
  onAddSyn: () => void
  onCheckSyn?: () => void
}) {
  const sslCfg = ssl ? (STATUS_CONFIG[ssl.status] ?? STATUS_CONFIG.error) : null
  const synAlive = syn?.latest?.success
  const synUptime = syn?.stats.uptime_pct ?? null
  const synLatency = syn?.latest?.response_time_ms ?? null

  const cardBorder = ssl?.status === 'expired' || ssl?.status === 'critical' || synAlive === false
    ? "border-rose-500/30 ring-1 ring-rose-500/20"
    : ssl?.status === 'warning'
      ? "border-amber-500/30 ring-1 ring-amber-500/20"
      : ssl?.status === 'notice'
        ? "border-sky-500/30 ring-1 ring-sky-500/20"
        : "border-slate-800"

  const siteUrl = `https://${domain}${port && port !== 443 ? `:${port}` : ''}/`

  return (
    <div className={cn(
      "bg-slate-900 rounded-3xl border p-5 shadow-premium transition-all relative overflow-hidden",
      cardBorder
    )}>
      {/* Header */}
      <div className="flex items-start justify-between gap-2 mb-5">
        <div className="flex items-center gap-3 min-w-0 flex-1">
          <div className={cn(
            "p-2 rounded-lg shrink-0",
            ssl?.status === 'ok' ? "bg-emerald-500/10 text-emerald-400" : "bg-slate-800 text-slate-400"
          )}>
            <Globe size={18} />
          </div>
          <div className="min-w-0">
            <a
              href={siteUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="group/link inline-flex items-center gap-1 max-w-full font-bold text-slate-100 leading-tight hover:text-indigo-400 transition-colors"
              title={`${domain} 새 탭에서 열기`}
            >
              <span className="truncate">{domain}</span>
              <ExternalLink size={12} className="shrink-0 text-slate-500 group-hover/link:text-indigo-400 transition-colors" />
            </a>
            {memo && <p className="text-[11px] text-slate-500 font-medium mt-0.5 truncate" title={memo}>{memo}</p>}
          </div>
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          {sslCfg && (
            <span className={cn(
              "inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-bold tracking-tight whitespace-nowrap",
              sslCfg.bg, sslCfg.text
            )}
              title="SSL 인증서 상태"
            >
              <sslCfg.icon size={10} />
              {sslCfg.label}
            </span>
          )}
          {syn && (
            <span className={cn(
              "inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-bold tracking-tight whitespace-nowrap",
              synAlive === true ? "bg-emerald-500/15 text-emerald-300"
                : synAlive === false ? "bg-rose-500/15 text-rose-300"
                : "bg-slate-800 text-slate-400"
            )}
              title="사이트 가용성 상태"
            >
              <Activity size={10} />
              {synAlive === true ? '정상' : synAlive === false ? '응답 없음' : '점검 전'}
            </span>
          )}
          <button
            className="p-1.5 rounded-lg text-slate-400 hover:text-indigo-400 hover:bg-indigo-500/10 transition-all"
            onClick={onEdit}
            title="수정"
          >
            <Pencil size={16} />
          </button>
          <button
            className="p-1.5 rounded-lg text-slate-400 hover:text-rose-500 hover:bg-rose-500/10 transition-all"
            onClick={onDelete}
            title="삭제"
          >
            <Trash2 size={16} />
          </button>
        </div>
      </div>

      {/* SSL 섹션 */}
      {ssl?.status === 'error' ? (
        <div className="bg-slate-800 border border-slate-800 rounded-xl p-3 flex items-center gap-2 text-slate-400 text-xs font-bold mb-4">
          <Info size={14} className="shrink-0" />
          <p className="truncate">{ssl.error || '인증서 확인 실패'}</p>
        </div>
      ) : ssl ? (
        <div className="space-y-2 mb-4 pb-4 border-b border-slate-800">
          <div className="flex items-center justify-between">
            <span className="text-[11px] font-bold text-slate-500 flex items-center gap-1.5">
              <ShieldCheck size={12} /> SSL 인증서
            </span>
            <div className="flex items-baseline gap-1">
              <span className={cn(
                "text-xl font-bold tabular-nums",
                ssl.days_left <= 7 ? "text-rose-400"
                  : ssl.days_left <= 30 ? "text-amber-400"
                  : ssl.days_left <= 60 ? "text-sky-400"
                  : "text-emerald-400"
              )}>{ssl.days_left}</span>
              <span className="text-xs font-bold text-slate-400">일</span>
            </div>
          </div>
          <div className="flex items-center justify-between text-[11px]">
            <span className="text-slate-500" title="SSL 인증서를 발급한 인증기관 (CA)">발급기관</span>
            <span className="text-slate-400 font-medium truncate max-w-[140px] text-right" title={ssl.issuer}>{ssl.issuer}</span>
          </div>
        </div>
      ) : (
        <div className="mb-4 pb-4 border-b border-slate-800 flex items-center gap-2 text-xs text-indigo-300">
          <RefreshCcw size={12} className="animate-spin" /> SSL 인증서 확인 중…
        </div>
      )}

      {/* Synthetic 섹션 */}
      {syn ? (
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-[11px] font-bold text-slate-500 flex items-center gap-1.5">
              <Activity size={12} /> 사이트 가용성
            </span>
            <span className={cn(
              "inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-bold",
              synAlive === true ? "bg-emerald-500/15 text-emerald-300"
                : synAlive === false ? "bg-rose-500/15 text-rose-300"
                : "bg-slate-800 text-slate-400"
            )}>
              {synAlive === true && <CheckCircle2 size={10} />}
              {synAlive === false && <ShieldAlert size={10} />}
              {synAlive === true ? '정상' : synAlive === false ? '응답 없음' : '점검 전'}
            </span>
          </div>
          <div className="grid grid-cols-2 gap-2 text-[11px]">
            <div className="flex items-center justify-between">
              <span className="text-slate-500">가동률</span>
              <span className={cn(
                "font-bold tabular-nums",
                synUptime !== null && synUptime >= 99 ? "text-emerald-400"
                  : synUptime !== null && synUptime >= 95 ? "text-amber-400"
                  : synUptime !== null ? "text-rose-400"
                  : "text-slate-500"
              )}>{synUptime !== null ? `${synUptime.toFixed(1)}%` : '—'}</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-slate-500">응답</span>
              <span className="font-bold text-slate-300 tabular-nums">{synLatency !== null ? `${synLatency}ms` : '—'}</span>
            </div>
          </div>
        </div>
      ) : (
        <div className="flex items-center justify-between gap-2 bg-slate-800/40 rounded-lg p-2.5">
          <div className="flex items-center gap-1.5 text-xs text-slate-400">
            <Activity size={13} /> 사이트 가용성 점검 없음
          </div>
          <button
            onClick={onAddSyn}
            disabled={addingSyn}
            className={cn(
              "inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-bold transition-colors",
              addingSyn
                ? "bg-indigo-500 text-white cursor-wait"
                : "bg-indigo-600 text-white hover:bg-indigo-700"
            )}
            title="이 도메인에 사이트 점검 추가 (5분 주기)"
          >
            {addingSyn ? <RefreshCcw size={12} className="animate-spin" /> : <Plus size={12} />}
            {addingSyn ? '추가 중...' : '점검 추가'}
          </button>
        </div>
      )}

      {/* Footer — 즉시 점검 버튼 (등록된 경우만) */}
      {onCheckSyn && (
        <div className="mt-4 pt-3 border-t border-slate-800 flex items-center justify-end">
          <button
            onClick={onCheckSyn}
            className="text-[10px] font-bold text-indigo-400 hover:text-indigo-300 flex items-center gap-1"
            title="사이트 즉시 점검"
          >
            <Zap size={10} /> 사이트 즉시 점검
          </button>
        </div>
      )}
    </div>
  )
}
