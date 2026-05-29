import { useState, useMemo } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Pencil, Trash2, Power, X, AlertTriangle, AlertCircle, Info, CheckCircle2, XCircle } from 'lucide-react'
import {
  listRules, createRule, updateRule, toggleRule, deleteRule, getRuleStats,
  severityLabel, categoryLabel,
  type LogRule, type LogRuleInput,
} from '../api/rules'
import { cn } from '../utils/cn'
import ConfirmDialog from '../components/ConfirmDialog'
import { RuleStatsContent } from './RuleStatsPage'

const EMPTY_INPUT: LogRuleInput = {
  name: '',
  pattern: '',
  severity: 'warning',
  threshold_count: null,
  threshold_window: null,
  cooldown_seconds: 300,
  enabled: true,
  description: null,
  category: 'system',
}

export default function RulesPage() {
  const qc = useQueryClient()
  const { data: rules = [], isLoading } = useQuery({ queryKey: ['log-rules'], queryFn: listRules })
  const { data: stats = {} } = useQuery({
    queryKey: ['log-rule-stats'],
    queryFn: getRuleStats,
    refetchInterval: 30000,
  })

  const [tab, setTab] = useState<'rules' | 'stats'>('rules')
  const [filterCat, setFilterCat] = useState<string>('all')
  const [filterSev, setFilterSev] = useState<string>('all')
  const [search, setSearch] = useState('')
  const [editing, setEditing] = useState<LogRule | null>(null)
  const [creating, setCreating] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<{ id: number; name: string } | null>(null)

  const filtered = useMemo(() => {
    return rules.filter(r => {
      if (filterCat !== 'all' && (r.category ?? '') !== filterCat) return false
      if (filterSev !== 'all' && r.severity !== filterSev) return false
      if (search && !r.name.includes(search) && !r.pattern.includes(search)) return false
      return true
    })
  }, [rules, filterCat, filterSev, search])

  const invalidate = () => qc.invalidateQueries({ queryKey: ['log-rules'] })

  const toggleMut = useMutation({ mutationFn: toggleRule, onSuccess: invalidate })
  const deleteMut = useMutation({ mutationFn: deleteRule, onSuccess: invalidate })

  if (isLoading) {
    return <div className="text-slate-500 text-sm">룰 불러오는 중…</div>
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between gap-4 border-b border-slate-800 pb-4">
        <div>
          <h1 className="text-2xl font-bold text-slate-100">로그 룰</h1>
          <p className="text-sm text-slate-500 mt-1">
            정규식 기반 로그 패턴 매칭 — {rules.length}개 등록 / {rules.filter(r => r.enabled).length}개 활성
          </p>
        </div>
        {tab === 'rules' && (
          <button
            onClick={() => setCreating(true)}
            className="flex items-center gap-2 px-4 py-2 rounded-lg bg-indigo-500 hover:bg-indigo-600 text-white text-sm font-semibold transition-colors"
          >
            <Plus size={16} /> 룰 추가
          </button>
        )}
      </div>

      {/* Tabs */}
      <div className="flex gap-1 bg-slate-900 border border-slate-800 rounded-lg p-1 w-fit">
        {([
          { id: 'rules', label: '룰 관리' },
          { id: 'stats', label: '매칭 통계' },
        ] as const).map(t => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            className={cn(
              "px-4 py-1.5 rounded-md text-sm font-bold transition-colors",
              tab === t.id ? "bg-indigo-600 text-white" : "text-slate-400 hover:text-slate-200"
            )}
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === 'stats' && <RuleStatsContent />}

      {tab === 'rules' && <>
      {/* Filters */}
      <div className="flex flex-wrap items-center gap-3">
        <FilterSelect label="카테고리" value={filterCat} onChange={setFilterCat}
          options={[
            { v: 'all', l: '전체' },
            { v: 'security', l: '보안' },
            { v: 'system', l: '시스템' },
            { v: 'network', l: '네트워크' },
            { v: 'app', l: '애플리케이션' },
          ]} />
        <FilterSelect label="심각도" value={filterSev} onChange={setFilterSev}
          options={[
            { v: 'all', l: '전체' },
            { v: 'critical', l: '심각' },
            { v: 'warning', l: '주의' },
            { v: 'info', l: '정보' },
          ]} />
        <input
          value={search}
          onChange={e => setSearch(e.target.value)}
          placeholder="이름·정규식 검색…"
          className="flex-1 max-w-xs px-3 py-2 rounded-lg bg-slate-900 border border-slate-800 text-sm text-slate-200 placeholder-slate-600"
        />
        <span className="ml-auto text-xs text-slate-500">
          {filtered.length}개 표시 / 최근 24h 매칭 {Object.values(stats).reduce((a, b) => a + b, 0).toLocaleString()}건
        </span>
      </div>

      {/* Rules List */}
      <div className="space-y-2">
        {filtered.map(rule => (
          <RuleRow
            key={rule.id}
            rule={rule}
            matchCount={stats[rule.id] ?? 0}
            onToggle={() => toggleMut.mutate(rule.id)}
            onEdit={() => setEditing(rule)}
            onDelete={() => setDeleteTarget({ id: rule.id, name: rule.name })}
          />
        ))}
        {filtered.length === 0 && (
          <div className="text-center text-slate-500 text-sm py-12">조건에 맞는 룰이 없습니다.</div>
        )}
      </div>
      </>}

      {/* Modal */}
      {(editing || creating) && (
        <RuleEditor
          initial={editing ?? null}
          onClose={() => { setEditing(null); setCreating(false) }}
          onSaved={() => { invalidate(); setEditing(null); setCreating(false) }}
        />
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title="룰 삭제"
        message={`'${deleteTarget?.name}' 룰을 삭제하시겠습니까?`}
        onConfirm={() => {
          if (deleteTarget) deleteMut.mutate(deleteTarget.id)
          setDeleteTarget(null)
        }}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  )
}

// ─── 룰 한 줄 카드 ────────────────────────────────────

function RuleRow({ rule, matchCount, onToggle, onEdit, onDelete }: {
  rule: LogRule, matchCount: number,
  onToggle: () => void, onEdit: () => void, onDelete: () => void
}) {
  const sevIcon = rule.severity === 'critical' ? <AlertCircle size={14} />
    : rule.severity === 'warning' ? <AlertTriangle size={14} /> : <Info size={14} />
  const sevColor = rule.severity === 'critical' ? 'text-rose-400 bg-rose-500/10 border-rose-500/30'
    : rule.severity === 'warning' ? 'text-amber-400 bg-amber-500/10 border-amber-500/30'
    : 'text-slate-400 bg-slate-500/10 border-slate-500/30'

  return (
    <div className={cn(
      "flex items-start gap-4 p-4 rounded-lg border transition-colors",
      rule.enabled ? "bg-slate-900/50 border-slate-800 hover:border-slate-700"
                   : "bg-slate-900/20 border-slate-800/40 opacity-60"
    )}>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 flex-wrap">
          <span className="font-semibold text-slate-100">{rule.name}</span>
          <span className={cn("flex items-center gap-1 px-2 py-0.5 rounded text-[11px] font-semibold border", sevColor)}>
            {sevIcon} {severityLabel(rule.severity)}
          </span>
          <span className="text-[11px] text-slate-500">{categoryLabel(rule.category)}</span>
          {rule.enabled ? (
            matchCount > 0 ? (
              <span className={cn(
                "inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-bold tabular-nums",
                matchCount >= 50 ? "bg-amber-500/15 text-amber-300"
                  : "bg-indigo-500/15 text-indigo-300"
              )}
                title={matchCount >= 50 ? "지난 24시간 매칭 50회 이상 — 패턴이 너무 광범위할 수 있음" : "지난 24시간 매칭 카운트"}
              >
                24h · {matchCount.toLocaleString()}건
              </span>
            ) : (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-semibold bg-slate-800 text-slate-500"
                title="지난 24시간 매칭 없음 — 시스템 조용(정상) 또는 패턴 검토">
                24h · 0건
              </span>
            )
          ) : (
            <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-semibold bg-slate-800 text-slate-500"
              title="이 룰은 비활성화 상태 — 매칭/알림 동작 안 함">
              비활성
            </span>
          )}
        </div>
        <code className="block mt-1.5 text-xs text-slate-400 font-mono truncate">{rule.pattern}</code>
        {rule.description && <p className="text-xs text-slate-500 mt-1">{rule.description}</p>}
      </div>
      <div className="flex gap-1 shrink-0">
        <IconBtn onClick={onToggle} title={rule.enabled ? '비활성화' : '활성화'}
          className={rule.enabled ? "text-emerald-400" : "text-slate-500"}>
          <Power size={15} />
        </IconBtn>
        <IconBtn onClick={onEdit} title="편집"><Pencil size={15} /></IconBtn>
        <IconBtn onClick={onDelete} title="삭제" className="hover:text-rose-400"><Trash2 size={15} /></IconBtn>
      </div>
    </div>
  )
}

function IconBtn({ children, onClick, title, className }: {
  children: React.ReactNode, onClick: () => void, title: string, className?: string
}) {
  return (
    <button onClick={onClick} title={title}
      className={cn("p-2 rounded-md text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition-colors", className)}>
      {children}
    </button>
  )
}

// ─── 편집/추가 모달 ──────────────────────────────────

// 자주 쓰는 룰 프리셋 — 학교/공공기관 시스템 흔한 패턴
const PRESETS = [
  { label: '메모리 부족',  pattern: '(?i)(out of memory|outofmemory|OOM)', category: 'system',   severity: 'critical' as const },
  { label: 'DB 연결 실패', pattern: '(?i)(connection refused|connection timeout|too many connections|conn.*failed)', category: 'app', severity: 'critical' as const },
  { label: '인증 실패',    pattern: '(?i)(failed password|authentication fail|invalid credentials|unauthorized)', category: 'security', severity: 'warning' as const },
  { label: '디스크 가득',  pattern: '(?i)(no space left|disk full|disk is full)', category: 'system', severity: 'critical' as const },
  { label: '서비스 다운',  pattern: '(?i)(service.*(down|stopped|failed)|panic|fatal)', category: 'system', severity: 'critical' as const },
  { label: '권한 거부',    pattern: '(?i)(permission denied|access denied|forbidden)', category: 'security', severity: 'warning' as const },
  { label: 'SSL/TLS 오류', pattern: '(?i)(ssl.*error|tls.*error|certificate.*expired|handshake fail)', category: 'network', severity: 'warning' as const },
  { label: '500 에러',     pattern: '(?i)(HTTP/[\\d.]+ 5\\d{2}|status.*5\\d{2}|internal server error)', category: 'app', severity: 'warning' as const },
]

// 쉬운 모드: 키워드 → 안전한 정규식 변환 (특수문자 escape + 대소문자 무시)
function keywordToPattern(keyword: string): string {
  const kw = keyword.trim()
  if (!kw) return ''
  // 사용자가 입력한 키워드들을 OR로 — 공백 또는 ',' 또는 '|' 구분
  const tokens = kw.split(/[\s,|]+/).filter(Boolean)
  // 각 토큰의 정규식 특수문자 escape
  const escaped = tokens.map(t => t.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))
  return tokens.length > 1 ? `(?i)(${escaped.join('|')})` : `(?i)${escaped[0]}`
}

function RuleEditor({ initial, onClose, onSaved }: {
  initial: LogRule | null, onClose: () => void, onSaved: () => void
}) {
  const isEdit = !!initial
  const [form, setForm] = useState<LogRuleInput>(initial
    ? { ...initial }
    : { ...EMPTY_INPUT })
  // 신규 추가는 쉬운 모드 기본, 편집은 정규식 모드 기본
  const [mode, setMode] = useState<'simple' | 'regex'>(isEdit ? 'regex' : 'simple')
  const [keyword, setKeyword] = useState('')
  const [error, setError] = useState('')
  const [testInput, setTestInput] = useState('')

  // 쉬운 모드: 키워드 입력 → 자동으로 form.pattern 업데이트
  const handleKeywordChange = (v: string) => {
    setKeyword(v)
    setForm(prev => ({ ...prev, pattern: keywordToPattern(v) }))
  }

  // 프리셋 적용
  const applyPreset = (p: typeof PRESETS[number]) => {
    setForm(prev => ({
      ...prev,
      name: prev.name || p.label,
      pattern: p.pattern,
      category: p.category,
      severity: p.severity,
    }))
    setMode('regex') // 프리셋은 직접 정규식 — regex 모드로 전환
  }

  // 즉시 정규식 테스트
  const testResult = useMemo(() => {
    if (!testInput || !form.pattern) return null
    try {
      const re = new RegExp(form.pattern)
      return re.test(testInput) ? 'match' : 'nomatch'
    } catch {
      return 'invalid'
    }
  }, [testInput, form.pattern])

  const save = async () => {
    setError('')
    try {
      if (isEdit && initial) {
        await updateRule(initial.id, form)
      } else {
        await createRule(form)
      }
      onSaved()
    } catch (e: any) {
      setError(e?.response?.data ?? e?.message ?? '저장 실패')
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 backdrop-blur-sm z-50 flex items-center justify-center p-4" onClick={onClose}>
      <div onClick={e => e.stopPropagation()}
        className="bg-slate-900 border border-slate-800 rounded-xl w-full max-w-2xl max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between px-6 py-4 border-b border-slate-800">
          <h2 className="text-lg font-bold text-slate-100">{isEdit ? '룰 편집' : '룰 추가'}</h2>
          <button onClick={onClose} className="p-1 rounded hover:bg-slate-800 text-slate-400"><X size={18} /></button>
        </div>

        <div className="p-6 space-y-4">
          {!isEdit && (
            <div>
              <label className="block text-xs font-semibold text-slate-400 mb-1.5">자주 쓰는 룰 (클릭 시 자동 채움)</label>
              <div className="flex flex-wrap gap-1.5">
                {PRESETS.map(p => (
                  <button
                    key={p.label}
                    type="button"
                    onClick={() => applyPreset(p)}
                    className="px-2.5 py-1 rounded-md text-[11px] font-bold bg-indigo-500/10 text-indigo-300 border border-indigo-500/30 hover:bg-indigo-500/20 transition-colors"
                  >
                    {p.label}
                  </button>
                ))}
              </div>
            </div>
          )}

          <Field label="이름 *">
            <input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })}
              placeholder="예: DB 연결 실패 알림"
              className="w-full px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-sm text-slate-200" />
          </Field>

          {/* 입력 모드 토글 */}
          <div>
            <label className="block text-xs font-semibold text-slate-400 mb-1.5">매칭 조건 입력 방식</label>
            <div className="inline-flex bg-slate-800 rounded-lg p-0.5">
              <button
                type="button"
                onClick={() => setMode('simple')}
                className={cn(
                  "px-3 py-1.5 rounded text-xs font-bold transition-colors",
                  mode === 'simple' ? "bg-indigo-600 text-white" : "text-slate-400 hover:text-slate-200"
                )}
              >
                쉬운 모드
              </button>
              <button
                type="button"
                onClick={() => setMode('regex')}
                className={cn(
                  "px-3 py-1.5 rounded text-xs font-bold transition-colors",
                  mode === 'regex' ? "bg-indigo-600 text-white" : "text-slate-400 hover:text-slate-200"
                )}
              >
                정규식 모드 (고급)
              </button>
            </div>
            {!isEdit && mode === 'regex' && form.pattern && !keyword && (
              <p className="flex items-start gap-1 text-[11px] text-indigo-300 mt-1.5">
                <Info size={11} className="shrink-0 mt-0.5" />
                <span>프리셋이 적용되었습니다 — 정규식을 직접 수정하거나 '쉬운 모드'로 전환 가능</span>
              </p>
            )}
          </div>

          {mode === 'simple' ? (
            <Field label="키워드 *" hint="공백 / 쉼표 / | 로 여러 개 입력 가능 (대소문자 무시). 예: OutOfMemory, OOM">
              <input
                value={keyword}
                onChange={e => handleKeywordChange(e.target.value)}
                placeholder="예: OutOfMemory"
                className="w-full px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-sm text-slate-200"
              />
              {form.pattern && (
                <div className="mt-1.5 bg-slate-800/60 border border-slate-700 rounded px-2.5 py-1.5 text-xs text-slate-400 font-mono">
                  <span className="text-slate-500">→ 변환된 정규식:</span>{' '}
                  <span className="text-slate-200">{form.pattern}</span>
                </div>
              )}
            </Field>
          ) : (
            <Field label="정규식 패턴 *" hint="Go RE2 문법. 예: Failed password.*from / (?i)error">
              <input value={form.pattern} onChange={e => setForm({ ...form, pattern: e.target.value })}
                placeholder="(?i)(out of memory|OOM)"
                className="w-full px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-sm font-mono text-slate-200" />
            </Field>
          )}

          <Field label="정규식 테스트" hint="샘플 로그 한 줄 — 매칭 여부 즉시 확인">
            <input value={testInput} onChange={e => setTestInput(e.target.value)} placeholder="예: 2026-05-19 sshd Failed password..."
              className="w-full px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-sm text-slate-200" />
            {testResult && (
              <div className={cn("flex items-center gap-1.5 text-xs mt-1.5 font-semibold",
                testResult === 'match' ? "text-emerald-400"
                : testResult === 'nomatch' ? "text-slate-500"
                : "text-rose-400")}>
                {testResult === 'match' && <CheckCircle2 size={12} />}
                {testResult === 'nomatch' && <XCircle size={12} />}
                {testResult === 'invalid' && <AlertTriangle size={12} />}
                {testResult === 'match' ? '매칭'
                  : testResult === 'nomatch' ? '매칭 안 됨'
                  : '정규식 오류'}
              </div>
            )}
          </Field>

          <div className="grid grid-cols-2 gap-4">
            <Field label="심각도">
              <select value={form.severity} onChange={e => setForm({ ...form, severity: e.target.value as any })}
                className="w-full px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-sm text-slate-200">
                <option value="info">정보</option>
                <option value="warning">주의</option>
                <option value="critical">심각</option>
              </select>
            </Field>
            <Field label="카테고리">
              <select value={form.category ?? 'system'} onChange={e => setForm({ ...form, category: e.target.value })}
                className="w-full px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-sm text-slate-200">
                <option value="security">보안</option>
                <option value="system">시스템</option>
                <option value="network">네트워크</option>
                <option value="app">애플리케이션</option>
              </select>
            </Field>
          </div>

          <Field label="알림 재발송 최소 간격 (초)" hint="같은 룰이 N초 안에 또 매칭돼도 알림 한 번만">
            <input type="number" value={form.cooldown_seconds}
              onChange={e => setForm({ ...form, cooldown_seconds: parseInt(e.target.value) || 300 })}
              className="w-full px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-sm text-slate-200" />
          </Field>

          <Field label="설명/대응 가이드" hint="운영자가 알림 받고 어떻게 대응할지 메모">
            <textarea value={form.description ?? ''} onChange={e => setForm({ ...form, description: e.target.value || null })}
              rows={3} className="w-full px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-sm text-slate-200" />
          </Field>

          <label className="flex items-center gap-2 text-sm text-slate-300">
            <input type="checkbox" checked={form.enabled} onChange={e => setForm({ ...form, enabled: e.target.checked })} />
            활성화
          </label>

          {error && <div className="text-sm text-rose-400 bg-rose-500/10 border border-rose-500/30 rounded p-3">{error}</div>}
        </div>

        <div className="flex justify-end gap-2 px-6 py-4 border-t border-slate-800">
          <button onClick={onClose} className="px-4 py-2 rounded-lg text-slate-400 hover:bg-slate-800 text-sm">취소</button>
          <button onClick={save} className="px-4 py-2 rounded-lg bg-indigo-500 hover:bg-indigo-600 text-white text-sm font-semibold">
            저장
          </button>
        </div>
      </div>
    </div>
  )
}

function Field({ label, hint, children }: { label: string, hint?: string, children: React.ReactNode }) {
  return (
    <div>
      <label className="block text-xs font-semibold text-slate-400 mb-1.5">{label}</label>
      {children}
      {hint && <p className="text-[11px] text-slate-500 mt-1">{hint}</p>}
    </div>
  )
}

function FilterSelect({ label, value, onChange, options }: {
  label: string, value: string, onChange: (v: string) => void,
  options: { v: string, l: string }[]
}) {
  return (
    <label className="text-xs text-slate-500 flex items-center gap-1.5">
      {label}
      <select value={value} onChange={e => onChange(e.target.value)}
        className="px-2 py-1.5 rounded bg-slate-900 border border-slate-800 text-sm text-slate-200">
        {options.map(o => <option key={o.v} value={o.v}>{o.l}</option>)}
      </select>
    </label>
  )
}
