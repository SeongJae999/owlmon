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

  const smtpIssue = !data.smtp_configured
  const recipientIssue = (data.recipients_count ?? 0) === 0

  return (
    <div className="mb-4 bg-blue-500/10 border border-blue-500/30 rounded-xl p-3 flex items-start gap-3">
      <Info size={18} className="text-blue-400 shrink-0 mt-0.5" />
      <div className="flex-1 min-w-0 space-y-1.5">
        <div className="text-sm font-semibold text-blue-200">
          알림이 DB에 기록되지만 이메일로는 발송되지 않습니다
        </div>
        {smtpIssue && (
          <div className="text-xs text-blue-300/90 leading-relaxed">
            · <strong>SMTP 서버 미설정</strong> — 서버 관리자가 <code className="bg-slate-800 px-1 rounded text-[10px]">docker-compose .env</code>에 <code className="bg-slate-800 px-1 rounded text-[10px]">SMTP_HOST/PORT/USERNAME/PASSWORD</code> 설정 후 서버 재시작 필요
          </div>
        )}
        {!smtpIssue && recipientIssue && (
          <div className="text-xs text-blue-300/90 leading-relaxed">
            · <strong>알림 수신자 미등록</strong> — 알림 설정에서 이메일 추가
          </div>
        )}
        <div className="text-[11px] text-blue-300/60">
          일부러 사용 안 하시는 경우(카톡/슬랙 등 다른 채널 또는 DB만 활용) 우측 X로 7일간 숨김.
        </div>
      </div>
      <div className="shrink-0 flex items-center gap-1">
        {!smtpIssue && recipientIssue && (
          <Link
            to="/settings"
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-blue-500/15 hover:bg-blue-500/25 text-blue-200 text-xs font-semibold transition-colors border border-blue-500/30"
          >
            <Settings size={12} />
            수신자 추가
          </Link>
        )}
        {smtpIssue && (
          <Link
            to="/support#requirements"
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-blue-500/15 hover:bg-blue-500/25 text-blue-200 text-xs font-semibold transition-colors border border-blue-500/30"
          >
            설치 가이드
          </Link>
        )}
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
