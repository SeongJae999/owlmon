import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { X, Tag, Trash2, FileText, Server, AlertCircle, Loader2 } from 'lucide-react'
import {
  annotateLog,
  getLogAnnotations,
  deleteAnnotation,
  type LogRecord,
  type AnnotationCategory,
  type LogAnnotation,
} from '../api/logs'
import { cn } from '../utils/cn'

// 카테고리 라벨 (한국어 표시용 + 색상)
const CATEGORY_OPTIONS: Array<{ value: AnnotationCategory; label: string; color: string }> = [
  { value: 'root_cause',     label: '원인 (Root Cause)',          color: 'text-rose-300 bg-rose-500/15' },
  { value: 'action_taken',   label: '조치 (Action Taken)',        color: 'text-emerald-300 bg-emerald-500/15' },
  { value: 'false_positive', label: '오탐 (False Positive)',      color: 'text-amber-300 bg-amber-500/15' },
]

function categoryStyle(c?: string) {
  return CATEGORY_OPTIONS.find(o => o.value === c)?.color ?? 'text-slate-400 bg-slate-800'
}

function categoryLabel(c?: string) {
  return CATEGORY_OPTIONS.find(o => o.value === c)?.label ?? '미분류'
}

interface Props {
  log: LogRecord
  onClose: () => void
}

export default function AnnotateModal({ log, onClose }: Props) {
  const queryClient = useQueryClient()
  const [category, setCategory] = useState<AnnotationCategory | ''>('')
  const [problem, setProblem] = useState('')
  const [solution, setSolution] = useState('')
  const [error, setError] = useState<string | null>(null)

  // 기존 라벨 목록
  const { data: existing = [], isLoading: existingLoading } = useQuery({
    queryKey: ['logAnnotations', log.id],
    queryFn: () => getLogAnnotations(log.id),
  })

  const saveMutation = useMutation({
    mutationFn: () => annotateLog(log.id, { category, problem, solution }),
    onSuccess: () => {
      // 입력 초기화 + 목록 갱신
      setCategory('')
      setProblem('')
      setSolution('')
      setError(null)
      queryClient.invalidateQueries({ queryKey: ['logAnnotations', log.id] })
    },
    onError: (err: any) => {
      setError(err?.response?.data ?? '저장 실패')
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: number) => deleteAnnotation(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['logAnnotations', log.id] })
    },
  })

  const handleSave = () => {
    setError(null)
    if (!problem.trim() && !solution.trim()) {
      setError('원인 또는 조치 중 최소 하나는 입력해야 합니다')
      return
    }
    saveMutation.mutate()
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-slate-900/60 backdrop-blur-sm" onClick={onClose} />
      <div className="bg-slate-900 w-full max-w-3xl max-h-[88vh] rounded-3xl shadow-2xl z-10 overflow-hidden flex flex-col border border-slate-800 animate-in zoom-in-95 duration-200">
        {/* Header */}
        <div className="p-5 border-b border-slate-800 flex items-center justify-between bg-slate-800/50">
          <div className="flex items-center gap-3 text-slate-200">
            <div className="p-2 bg-slate-900 rounded-xl shadow-sm"><Tag size={18} className="text-indigo-400" /></div>
            <div>
              <h3 className="font-bold text-base leading-tight">로그 라벨링</h3>
              <p className="text-[10px] font-bold text-slate-400 uppercase tracking-widest mt-0.5">
                Log #{log.id} · {new Date(log.timestamp).toLocaleString('ko-KR')}
              </p>
            </div>
          </div>
          <button onClick={onClose} className="p-2 text-slate-400 hover:bg-slate-900 rounded-full hover:text-slate-100 transition-all border border-transparent hover:border-slate-800">
            <X size={18} />
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto p-5 space-y-5">
          {/* 원본 로그 미리보기 */}
          <div className="bg-slate-800/40 border border-slate-800 rounded-2xl p-3 space-y-2">
            <div className="flex flex-wrap items-center gap-3 text-[11px] font-bold text-slate-400">
              <span className="flex items-center gap-1.5"><Server size={11} className="opacity-60" /> {log.host}</span>
              <span className="flex items-center gap-1.5"><FileText size={11} className="opacity-60" /> {log.source}</span>
              {log.level && (
                <span className="px-2 py-0.5 rounded text-[10px] uppercase tracking-tight bg-slate-700 text-slate-300">{log.level}</span>
              )}
            </div>
            <code className="block text-[11px] leading-relaxed text-slate-300 whitespace-pre-wrap break-all font-mono">
              {log.line}
            </code>
          </div>

          {/* 입력 폼 */}
          <div className="space-y-3">
            <div className="space-y-1">
              <label className="text-[10px] font-bold text-slate-400 uppercase ml-1 tracking-wider">카테고리</label>
              <select
                className="w-full bg-slate-800 border border-slate-800 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition-all cursor-pointer"
                value={category}
                onChange={e => setCategory(e.target.value as AnnotationCategory | '')}
              >
                <option value="">미분류</option>
                {CATEGORY_OPTIONS.map(o => (
                  <option key={o.value} value={o.value}>{o.label}</option>
                ))}
              </select>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              <div className="space-y-1">
                <label className="text-[10px] font-bold text-slate-400 uppercase ml-1 tracking-wider">원인 (왜 발생)</label>
                <textarea
                  className="w-full bg-slate-800 border border-slate-800 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition-all min-h-[100px] resize-y"
                  value={problem}
                  onChange={e => setProblem(e.target.value)}
                  placeholder="예: 디스크 90% 초과로 인한 쓰기 실패"
                />
              </div>
              <div className="space-y-1">
                <label className="text-[10px] font-bold text-slate-400 uppercase ml-1 tracking-wider">조치 (어떻게 처리)</label>
                <textarea
                  className="w-full bg-slate-800 border border-slate-800 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition-all min-h-[100px] resize-y"
                  value={solution}
                  onChange={e => setSolution(e.target.value)}
                  placeholder="예: 오래된 로그 삭제 + 디스크 retention 정책 강화"
                />
              </div>
            </div>

            {error && (
              <div className="flex items-center gap-2 text-xs text-rose-300 bg-rose-500/10 border border-rose-500/30 rounded-lg px-3 py-2">
                <AlertCircle size={14} /> {error}
              </div>
            )}

            <div className="flex justify-end gap-2">
              <button
                onClick={onClose}
                className="px-4 py-2 rounded-lg text-sm font-bold text-slate-300 bg-slate-800 hover:bg-slate-700 transition-colors"
              >
                닫기
              </button>
              <button
                onClick={handleSave}
                disabled={saveMutation.isPending}
                className="px-4 py-2 rounded-lg text-sm font-bold text-white bg-indigo-600 hover:bg-indigo-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors shadow-sm shadow-indigo-500/20 flex items-center gap-2"
              >
                {saveMutation.isPending && <Loader2 size={14} className="animate-spin" />}
                라벨 저장
              </button>
            </div>
          </div>

          {/* 기존 라벨 목록 */}
          <div className="pt-4 border-t border-slate-800">
            <h4 className="text-[10px] font-bold text-slate-400 uppercase tracking-widest mb-2">
              이 로그의 기존 라벨 ({existing.length})
            </h4>
            {existingLoading ? (
              <div className="text-xs text-slate-500 py-3">불러오는 중...</div>
            ) : existing.length === 0 ? (
              <div className="text-xs text-slate-500 py-3 italic">아직 부여된 라벨이 없습니다.</div>
            ) : (
              <ul className="space-y-2">
                {existing.map((a: LogAnnotation) => (
                  <li key={a.id} className="bg-slate-800/40 border border-slate-800 rounded-2xl p-3">
                    <div className="flex items-center justify-between gap-2 mb-2">
                      <div className="flex items-center gap-2 text-[11px]">
                        <span className={cn('px-2 py-0.5 rounded font-bold', categoryStyle(a.category))}>
                          {categoryLabel(a.category)}
                        </span>
                        <span className="text-slate-500">by <span className="text-slate-300 font-bold">{a.annotator}</span></span>
                        <span className="text-slate-600">{new Date(a.created_at).toLocaleString('ko-KR')}</span>
                      </div>
                      <button
                        onClick={() => deleteMutation.mutate(a.id)}
                        disabled={deleteMutation.isPending}
                        className="text-slate-500 hover:text-rose-400 transition-colors p-1 rounded"
                        title="라벨 삭제"
                      >
                        <Trash2 size={13} />
                      </button>
                    </div>
                    {a.problem && (
                      <div className="text-xs text-slate-300 mb-1">
                        <span className="text-slate-500 font-bold">원인:</span> {a.problem}
                      </div>
                    )}
                    {a.solution && (
                      <div className="text-xs text-slate-300">
                        <span className="text-slate-500 font-bold">조치:</span> {a.solution}
                      </div>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
