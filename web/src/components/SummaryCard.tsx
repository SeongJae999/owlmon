import { cn } from '../utils/cn'

/**
 * 페이지 상단 요약 통계 카드 — 아이콘 + 라벨 + 큰 숫자 (+선택 suffix)
 * AlertHistory/RuleStats/AuditLog 등에서 복붙되던 것을 단일화.
 * 색 키는 팔레트 정식명만 사용 (indigo/emerald/amber/rose/blue/slate).
 */
const THEMES: Record<string, string> = {
  indigo:  'bg-indigo-500/10 border-indigo-500/30 text-indigo-300',
  emerald: 'bg-emerald-500/10 border-emerald-500/30 text-emerald-300',
  amber:   'bg-amber-500/10 border-amber-500/30 text-amber-300',
  rose:    'bg-rose-500/10 border-rose-500/30 text-rose-300',
  blue:    'bg-blue-500/10 border-blue-500/30 text-blue-300',
  slate:   'bg-slate-500/10 border-slate-700 text-slate-300',
}

export default function SummaryCard({ icon: Icon, label, value, color, suffix }: {
  icon: any
  label: string
  value: string | number
  color: string
  suffix?: string
}) {
  return (
    <div className={cn('rounded-xl p-3 border flex items-center gap-3', THEMES[color])}>
      <Icon size={20} className="opacity-70 shrink-0" />
      <div className="min-w-0">
        <div className="text-[11px] font-semibold opacity-80 truncate">{label}</div>
        <div className="text-xl font-bold tabular-nums">
          {typeof value === 'number' ? value.toLocaleString() : value}
          {suffix && <span className="text-xs font-normal opacity-60 ml-1">{suffix}</span>}
        </div>
      </div>
    </div>
  )
}
