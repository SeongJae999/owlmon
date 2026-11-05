import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'

interface Props {
  icon: LucideIcon
  title: string
  description?: string
  /** 우측 액션 영역 (버튼 그룹 등) */
  children?: ReactNode
}

/**
 * PageToolbar — 리소스 목록형 대시보드 페이지의 상단 툴바.
 *
 * 사용처: SNMP / SSL / DPM / Synthetic 등 "리스트 + 액션" 형태의 페이지.
 * Overview의 hero, HostDetail의 navigation 헤더와는 의미가 달라 별개 컴포넌트.
 */
export default function PageToolbar({ icon: Icon, title, description, children }: Props) {
  return (
    <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-3 bg-slate-900 p-4 rounded-2xl border border-slate-800">
      <div className="flex items-center gap-3 min-w-0">
        <div className="p-2 bg-indigo-500/15 text-indigo-400 rounded-lg border border-indigo-500/20 shrink-0">
          <Icon size={18} />
        </div>
        <div>
          <h1 className="font-bold text-sm text-slate-100 leading-tight">{title}</h1>
          {description && (
            <p className="text-xs text-slate-500 font-medium mt-0.5">{description}</p>
          )}
        </div>
      </div>
      {/* 모바일에선 제목 아래로 내려가고 버튼이 많으면 줄바꿈 */}
      {children && <div className="flex flex-wrap gap-2">{children}</div>}
    </div>
  )
}
