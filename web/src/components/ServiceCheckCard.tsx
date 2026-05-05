import { CheckCircle2, AlertCircle } from 'lucide-react'
import { cn } from '../utils/cn'

interface Props {
  name: string
  type: string
  target: string
  status: number  // 1 = 정상, 0 = 장애
  latencyMs: number | null
}

export default function ServiceCheckCard({ name, type, target, status, latencyMs }: Props) {
  const isUp = status === 1

  return (
    <div
      className={cn(
        "bg-slate-900 rounded-2xl border p-5 transition-colors duration-200 group",
        isUp ? "border-slate-800 hover:border-emerald-500/40" : "border-rose-500/40"
      )}
    >
      {/* Header */}
      <div className="flex items-center justify-between gap-3 mb-4">
        <h4 className="text-sm font-semibold text-slate-200 truncate min-w-0 flex-1" title={name}>{name}</h4>
        <span className={cn(
          "inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-xs font-semibold border shrink-0",
          isUp ? "bg-emerald-500/10 text-emerald-400 border-emerald-500/30" : "bg-rose-500/10 text-rose-400 border-rose-500/30"
        )}>
          {isUp ? (
            <><CheckCircle2 size={11} /> 정상</>
          ) : (
            <><AlertCircle size={11} /> 장애</>
          )}
        </span>
      </div>

      {/* Latency */}
      <div className="flex items-baseline gap-1 mb-4">
        <span className={cn(
          "text-2xl font-bold tabular-nums",
          isUp ? "text-slate-100" : "text-rose-400"
        )}>
          {latencyMs !== null ? `${latencyMs.toFixed(0)}` : '--'}
        </span>
        <span className="text-xs font-medium text-slate-500">ms</span>
      </div>

      {/* Type + Target */}
      <div className="flex items-center gap-2 pt-3 border-t border-slate-800">
        <span className="px-1.5 py-0.5 bg-slate-800 text-slate-400 rounded text-xs font-semibold uppercase shrink-0">
          {type}
        </span>
        <span className="text-xs text-slate-500 truncate font-mono" title={target}>
          {target}
        </span>
      </div>
    </div>
  )
}
