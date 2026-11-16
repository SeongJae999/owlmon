import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from 'lucide-react'
import { PAGE_SIZE } from '../utils/paged'

export default function Pager({ total, page, onPage }: { total: number; page: number; onPage: (p: number) => void }) {
  const last = Math.ceil(total / PAGE_SIZE) - 1
  if (last < 1) return null
  const steps = [
    [ChevronsLeft, '첫 페이지', 0], [ChevronLeft, '이전 페이지', page - 1],
    [ChevronRight, '다음 페이지', page + 1], [ChevronsRight, '끝 페이지', last],
  ] as const
  return (
    <nav aria-label="페이지" className="flex items-center justify-end gap-1.5 px-4 py-2 border-t border-base-800 text-xs text-base-400">
      <span className="mr-2 tabular-nums">
        {(page * PAGE_SIZE + 1).toLocaleString()}–{Math.min(total, (page + 1) * PAGE_SIZE).toLocaleString()} / {total.toLocaleString()}
      </span>
      {steps.map(([Icon, label, p]) => {
        const off = p < 0 || p > last || p === page
        // disabled 대신 aria-disabled — 끝 페이지에 닿아도 키보드 포커스가 버튼에 남게
        return (
          <button key={label} type="button" aria-label={label} title={label} aria-disabled={off} onClick={() => !off && onPage(p)}
            className="p-1.5 rounded-md border border-base-700 bg-base-800 text-base-400 hover:text-base-200 hover:bg-base-700 aria-disabled:opacity-30 aria-disabled:cursor-not-allowed">
            <Icon size={14} />
          </button>
        )
      })}
    </nav>
  )
}
