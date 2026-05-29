import { useEffect } from 'react'
import { AlertTriangle, X } from 'lucide-react'
import { cn } from '../utils/cn'

interface Props {
  open: boolean
  title?: string
  message: string
  confirmLabel?: string
  cancelLabel?: string
  variant?: 'danger' | 'default'
  onConfirm: () => void
  onCancel: () => void
}

/**
 * 공용 확인 모달 — 브라우저 native confirm() 대체.
 * 다크 테마, ESC/배경 클릭으로 취소.
 */
export default function ConfirmDialog({
  open,
  title = '확인',
  message,
  confirmLabel = '삭제',
  cancelLabel = '취소',
  variant = 'danger',
  onConfirm,
  onCancel,
}: Props) {
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onCancel()
      if (e.key === 'Enter') onConfirm()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [open, onConfirm, onCancel])

  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm animate-in fade-in duration-150"
      onClick={onCancel}
    >
      <div
        className="bg-slate-900 border border-slate-800 rounded-2xl shadow-2xl w-full max-w-sm mx-4 animate-in zoom-in-95 duration-150"
        onClick={e => e.stopPropagation()}
      >
        <div className="flex items-start justify-between p-5 pb-3">
          <div className="flex items-center gap-3">
            <div className={cn(
              "p-2 rounded-lg",
              variant === 'danger' ? "bg-rose-500/15 text-rose-400" : "bg-indigo-500/15 text-indigo-400"
            )}>
              <AlertTriangle size={18} />
            </div>
            <h3 className="text-base font-bold text-slate-100">{title}</h3>
          </div>
          <button
            onClick={onCancel}
            className="p-1 rounded-md text-slate-500 hover:text-slate-300 hover:bg-slate-800 transition-colors"
            aria-label="닫기"
          >
            <X size={16} />
          </button>
        </div>
        <div className="px-5 pb-5">
          <p className="text-sm text-slate-300 leading-relaxed">{message}</p>
        </div>
        <div className="flex gap-2 px-5 pb-5 justify-end">
          <button
            onClick={onCancel}
            className="px-4 py-2 rounded-lg text-sm font-semibold bg-slate-800 text-slate-300 border border-slate-700 hover:bg-slate-700 transition-colors"
          >
            {cancelLabel}
          </button>
          <button
            onClick={onConfirm}
            className={cn(
              "px-4 py-2 rounded-lg text-sm font-semibold text-white transition-colors",
              variant === 'danger'
                ? "bg-rose-600 hover:bg-rose-700"
                : "bg-indigo-600 hover:bg-indigo-700"
            )}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
