import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { AlertTriangle, Settings } from 'lucide-react'
import { getEmailStatus } from '../api/alert'

/**
 * 글로벌 경고 배너 — SMTP/수신자 미설정 시 표시
 *
 * 알람은 DB에 기록되지만 SMTP가 안 설정되어 있으면 실제 메일이 안 감.
 * MVP 핵심 가치(알림) 손상 가능성을 즉시 알려준다.
 */
export default function EmailStatusBanner() {
  const { data } = useQuery({
    queryKey: ['email-status'],
    queryFn: getEmailStatus,
    staleTime: 60_000,        // 1분 캐시
    refetchOnWindowFocus: false,
  })

  if (!data || data.healthy) return null

  return (
    <div className="mb-4 bg-amber-500/10 border border-amber-500/40 rounded-xl p-3 flex items-start gap-3">
      <AlertTriangle size={18} className="text-amber-400 shrink-0 mt-0.5" />
      <div className="flex-1 min-w-0">
        <div className="text-sm font-bold text-amber-200">
          ⚠️ 알람 이메일이 실제로 발송되지 않습니다
        </div>
        <div className="mt-1 text-xs text-amber-300/90 leading-relaxed">
          알람은 DB에 기록되지만 메일이 안 가는 상태입니다. 다음을 확인하세요:
        </div>
        <ul className="mt-1.5 space-y-0.5">
          {data.issues.map((issue, i) => (
            <li key={i} className="text-xs text-amber-300/80 flex items-center gap-1.5">
              <span className="w-1 h-1 rounded-full bg-amber-400" />
              {issue}
            </li>
          ))}
        </ul>
      </div>
      <Link
        to="/settings"
        className="shrink-0 inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-amber-500/20 hover:bg-amber-500/30 text-amber-200 text-xs font-semibold transition-colors border border-amber-500/30"
      >
        <Settings size={12} />
        설정 열기
      </Link>
    </div>
  )
}
