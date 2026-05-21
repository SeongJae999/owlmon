import { useState, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { Info, Settings, X } from 'lucide-react'
import { getEmailStatus } from '../api/alert'

const DISMISS_KEY = 'owlmon:email-banner-dismissed'
const DISMISS_DAYS = 7

/**
 * 글로벌 안내 배너 — SMTP/수신자 미설정 시 표시
 *
 * 톤: 경고 X (일부러 안 쓰는 경우 정당함 — 카톡/슬랙 등 다른 채널 또는 DB만 활용 가능)
 *     안내(파란색) + dismiss 가능 — 사용자 자율
 *
 * 향후 알림 채널 시스템(카톡/슬랙) 도입 시 자연스럽게 채널 상태 표시로 진화.
 */
export default function EmailStatusBanner() {
  const [dismissed, setDismissed] = useState(false)

  // localStorage에 7일 내 dismiss 기록 있으면 숨김
  useEffect(() => {
    const raw = localStorage.getItem(DISMISS_KEY)
    if (raw) {
      const at = parseInt(raw, 10)
      if (Date.now() - at < DISMISS_DAYS * 24 * 60 * 60 * 1000) {
        setDismissed(true)
      } else {
        localStorage.removeItem(DISMISS_KEY)
      }
    }
  }, [])

  const { data } = useQuery({
    queryKey: ['email-status'],
    queryFn: getEmailStatus,
    staleTime: 60_000,
    refetchOnWindowFocus: false,
  })

  if (!data || data.healthy || dismissed) return null

  const handleDismiss = () => {
    localStorage.setItem(DISMISS_KEY, String(Date.now()))
    setDismissed(true)
  }

  return (
    <div className="mb-4 bg-blue-500/10 border border-blue-500/30 rounded-xl p-3 flex items-start gap-3">
      <Info size={18} className="text-blue-400 shrink-0 mt-0.5" />
      <div className="flex-1 min-w-0">
        <div className="text-sm font-semibold text-blue-200">
          알람이 DB에는 기록되지만 이메일로는 발송되지 않습니다
        </div>
        <div className="mt-1 text-xs text-blue-300/80 leading-relaxed">
          {data.issues.join(' · ')}
        </div>
        <div className="mt-1.5 text-[11px] text-blue-300/60">
          일부러 사용 안 하시는 경우 우측 ✕ 로 7일간 숨길 수 있습니다 (카카오톡/슬랙 등 다른 채널 도입 시 자연스럽게 채널 상태 표시로 전환 예정).
        </div>
      </div>
      <div className="shrink-0 flex items-center gap-1">
        <Link
          to="/settings"
          className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-blue-500/15 hover:bg-blue-500/25 text-blue-200 text-xs font-semibold transition-colors border border-blue-500/30"
        >
          <Settings size={12} />
          설정
        </Link>
        <button
          onClick={handleDismiss}
          className="p-1.5 rounded-lg text-blue-300/60 hover:text-blue-200 hover:bg-blue-500/10 transition-colors"
          title="7일간 숨기기"
        >
          <X size={14} />
        </button>
      </div>
    </div>
  )
}
