import { Suspense, lazy, useEffect, useState } from 'react'
import { MessageCircle, X } from 'lucide-react'
import { cn } from '../utils/cn'

// 처음 열 때만 로드 — 안 쓰는 사용자에게 챗봇 코드를 내려보내지 않음
const ChatPage = lazy(() => import('../pages/ChatPage'))

export default function ChatWidget() {
  const [open, setOpen] = useState(false)
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false)
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open])

  return (
    <>
      {/* 닫아도 언마운트하지 않음 — 응답 대기 중에 닫았다 열어도 대화가 이어지게 */}
      {loaded && (
        <div
          role="dialog"
          aria-label="OWL AI"
          className={cn(
            'fixed z-50 bg-base-900 border border-base-700 shadow-2xl flex flex-col',
            'inset-0 sm:inset-auto sm:bottom-24 sm:right-6 sm:w-[400px] sm:h-[min(600px,calc(100dvh-8rem))] sm:rounded-xl',
            !open && 'hidden',
          )}
        >
          <header className="flex items-center justify-between px-4 py-3 border-b border-base-800 shrink-0">
            <div className="flex items-center gap-2">
              <MessageCircle size={18} className="text-brand-400" />
              <span className="text-sm font-semibold text-base-100">OWL AI</span>
            </div>
            <button
              type="button"
              onClick={() => setOpen(false)}
              aria-label="OWL AI 닫기"
              className="p-1.5 rounded-lg text-base-400 hover:bg-base-800 hover:text-base-200"
            >
              <X size={18} />
            </button>
          </header>
          <div className="flex-1 min-h-0">
            <Suspense fallback={<div className="p-6 text-center text-sm text-base-500">불러오는 중...</div>}>
              <ChatPage floating />
            </Suspense>
          </div>
        </div>
      )}

      <button
        type="button"
        onClick={() => { setLoaded(true); setOpen(o => !o) }}
        aria-label={open ? 'OWL AI 닫기' : 'OWL AI 열기'}
        aria-expanded={open}
        className={cn(
          'fixed z-50 bottom-5 right-5 sm:bottom-6 sm:right-6 w-14 h-14 rounded-full shadow-lg',
          'bg-brand-600 hover:bg-brand-500 text-white flex items-center justify-center transition-colors',
          open && 'hidden sm:flex',
        )}
      >
        {open ? <X size={24} /> : <MessageCircle size={24} />}
      </button>
    </>
  )
}
