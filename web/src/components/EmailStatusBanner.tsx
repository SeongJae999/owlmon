import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { Info, X } from 'lucide-react'
import { getEmailStatus } from '../api/alert'

const DISMISS_KEY = 'owlmon:email-banner-dismissed'
const DISMISS_DAYS = 7

// 메일·웹훅·문자 어느 것으로도 알림이 나가지 않을 때만 — 일부러 DB 기록만 쓰는 곳도 있어 경고가 아닌 안내, 숨길 수 있음
export default function EmailStatusBanner() {
  // 7일 안에 숨긴 기록이 있으면 숨김 — 첫 렌더에서 바로 읽어 배너가 잠깐 보였다 사라지지 않게
  const [dismissed, setDismissed] = useState(() => {
    try {
      const at = parseInt(localStorage.getItem(DISMISS_KEY) ?? '', 10)
      return Date.now() - at < DISMISS_DAYS * 24 * 60 * 60 * 1000
    } catch { return false }
  })

  const { data } = useQuery({
    queryKey: ['email-status'],
    queryFn: getEmailStatus,
    staleTime: 60_000,
    refetchOnWindowFocus: false,
  })

  if (!data || data.healthy || dismissed) return null

  const handleDismiss = () => {
    try { localStorage.setItem(DISMISS_KEY, String(Date.now())) } catch { /* 저장 못 해도 이번엔 숨김 */ }
    setDismissed(true)
  }

  const smtpIssue = !data.smtp_configured
  const recipientIssue = (data.recipients_count ?? 0) === 0

  return (
    <div className="bg-blue-500/10 border border-blue-500/30 rounded-xl p-3 flex items-start gap-3">
      <Info size={18} className="text-blue-400 shrink-0 mt-0.5" />
      <div className="flex-1 min-w-0 space-y-1.5">
        <div className="text-sm font-semibold text-blue-200">
          알림이 DB에만 기록되고 어디로도 발송되지 않습니다
        </div>
        {smtpIssue && (
          <div className="text-xs text-blue-300/90 leading-relaxed">
            <strong>SMTP 서버 미설정:</strong> 서버 관리자가 <code className="bg-base-800 px-1 rounded text-xs">docker-compose .env</code>에 <code className="bg-base-800 px-1 rounded text-xs">SMTP_HOST/PORT/USERNAME/PASSWORD</code> 설정 후 서버 재시작 필요
          </div>
        )}
        {!smtpIssue && recipientIssue && (
          <div className="text-xs text-blue-300/90 leading-relaxed">
            <strong>알림 수신자 미등록:</strong> 아래 이메일 수신자에 추가
          </div>
        )}
        <div className="text-xs text-blue-300/60">
          메일 대신 아래 웹훅·문자만 설정해도 됩니다. 일부러 DB 기록만 쓰신다면 우측 X로 7일간 숨김.
        </div>
      </div>
      <div className="shrink-0 flex items-center gap-1">
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
          title="7일간 숨기기" aria-label="7일간 숨기기"
        >
          <X size={14} />
        </button>
      </div>
    </div>
  )
}
