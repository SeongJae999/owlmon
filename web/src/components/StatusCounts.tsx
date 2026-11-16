import { TONES, type Tone } from '../utils/severity'
import { cn } from '../utils/cn'

interface Props {
  crit: number
  warn: number
  handling?: number
  loading?: boolean
  unknown?: boolean // 조회 실패·갱신 중단 — 숫자를 믿을 수 없을 때
  className?: string
}

export default function StatusCounts({ crit, warn, handling = 0, loading, unknown, className }: Props) {
  const pill = (tone: Tone) => cn('rounded-lg border px-3 py-1 whitespace-nowrap font-bold', TONES[tone].bg, TONES[tone].text, TONES[tone].border)
  return (
    <div role="status" className={cn('flex items-center gap-2', className)}>
      {unknown ? (
        <span className={pill('slate')}>상태 확인 불가</span>
      ) : loading ? (
        <span className={pill('slate')}>불러오는 중</span>
      ) : !crit && !warn && !handling ? (
        <span className={pill('emerald')}>모두 정상</span>
      ) : (
        <>
          <span className={pill(crit ? 'rose' : 'slate')}>위험 {crit}</span>
          <span className={pill(warn ? 'amber' : 'slate')}>주의 {warn}</span>
          {/* '처리 중'은 조치 중으로 읽히지만 실제로는 담당자가 보고 알림 메일만 멈춘 상태 — 교대 인계 때 방치되지 않게 사실대로 */}
          <span className={pill('slate')}>확인 {handling}</span>
        </>
      )}
    </div>
  )
}
