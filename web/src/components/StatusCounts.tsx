import { TONES, type Tone } from '../utils/severity'
import { cn } from '../utils/cn'

interface Props {
  crit: number
  warn: number
  handling?: number
  loading?: boolean
  className?: string
}

export default function StatusCounts({ crit, warn, handling = 0, loading, className }: Props) {
  const pill = (tone: Tone) => cn('rounded-lg border px-3 py-1 whitespace-nowrap font-bold', TONES[tone].bg, TONES[tone].text, TONES[tone].border)
  return (
    <div role="status" className={cn('flex items-center gap-2', className)}>
      {loading ? (
        <span className={pill('slate')}>상태 확인 중</span>
      ) : !crit && !warn && !handling ? (
        <span className={pill('emerald')}>모두 정상</span>
      ) : (
        <>
          <span className={pill(crit ? 'rose' : 'slate')}>위험 {crit}</span>
          <span className={pill(warn ? 'amber' : 'slate')}>주의 {warn}</span>
          <span className={pill('slate')}>처리 중 {handling}</span>
        </>
      )}
    </div>
  )
}
