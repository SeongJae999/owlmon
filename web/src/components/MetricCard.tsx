import { AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer, ReferenceLine } from 'recharts'
import { AlertTriangle, TrendingUp, Info } from 'lucide-react'
import { cn } from '../utils/cn'

interface AnomalyInfo {
  z_score: number
  mean: number
  severity: string
}

interface DiskPredictionInfo {
  days_left: number
  slope: number
  r2: number
}

interface Props {
  title: string
  value: number | null
  unit?: string
  data: { time: string; value: number }[]
  color: string
  warning?: number
  critical?: number
  anomaly?: AnomalyInfo | null
  diskPrediction?: DiskPredictionInfo | null
}

function getStatus(value: number | null, warning = 70, critical = 90) {
  if (value === null) return 'unknown'
  if (value >= critical) return 'critical'
  if (value >= warning) return 'warning'
  return 'normal'
}

const statusConfig = {
  normal: {
    color: '#34d399',
    label: '정상',
    bg: 'bg-emerald-500/10',
    text: 'text-emerald-400',
    border: 'border-emerald-500/30'
  },
  warning: {
    color: '#fbbf24',
    label: '경고',
    bg: 'bg-amber-500/10',
    text: 'text-amber-400',
    border: 'border-amber-500/30'
  },
  critical: {
    color: '#f87171',
    label: '위험',
    bg: 'bg-rose-500/10',
    text: 'text-rose-400',
    border: 'border-rose-500/30'
  },
  unknown: {
    color: '#64748b',
    label: '데이터 없음',
    bg: 'bg-slate-800',
    text: 'text-slate-500',
    border: 'border-slate-700'
  },
}

export default function MetricCard({ title, value, unit = '%', data, color, warning, critical, anomaly, diskPrediction }: Props) {
  const status = getStatus(value, warning, critical)
  const hasAnomaly = anomaly != null
  const cfg = statusConfig[status]

  return (
    <div
      className={cn(
        "bg-slate-900 rounded-2xl border p-5 transition-colors duration-200 relative overflow-hidden",
        hasAnomaly ? "border-purple-500/40" : "border-slate-800"
      )}
    >
      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-2">
          <div className={cn("w-2 h-2 rounded-full", hasAnomaly ? "bg-purple-400" : status === 'normal' ? "bg-emerald-400" : status === 'warning' ? "bg-amber-400" : "bg-rose-400")} />
          <h4 className="text-sm font-semibold text-slate-300">{title}</h4>
        </div>

        <div className="flex items-center gap-2">
          {hasAnomaly && (
            <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md bg-purple-500/20 text-purple-300 border border-purple-500/30 text-xs font-semibold">
              <AlertTriangle size={11} /> 이상
            </span>
          )}
          <span className={cn(
            "px-2 py-0.5 rounded-md text-xs font-semibold border",
            cfg.bg, cfg.text, cfg.border
          )}>
            {cfg.label}
          </span>
        </div>
      </div>

      {/* Current Value */}
      <div className="flex items-baseline gap-1.5 mb-4">
        <span className="text-3xl font-bold text-slate-100 tabular-nums leading-none">
          {value !== null ? value.toFixed(1) : '--'}
        </span>
        <span className="text-sm font-medium text-slate-500">{unit}</span>
      </div>

      {/* Disk Prediction / Anomaly Details */}
      {(diskPrediction || hasAnomaly) && (
        <div className="mb-4">
          {diskPrediction && diskPrediction.days_left >= 0 && diskPrediction.r2 >= 0.5 && (
            <div className={cn(
              "flex items-center justify-between px-3 py-2 rounded-lg border",
              diskPrediction.days_left <= 7
                ? "bg-rose-500/10 border-rose-500/30 text-rose-300"
                : "bg-amber-500/10 border-amber-500/30 text-amber-300"
            )}>
              <div className="flex items-center gap-1.5 font-semibold text-xs">
                <TrendingUp size={13} />
                {diskPrediction.days_left <= 1
                  ? '24시간 내 부족 예상'
                  : `약 ${Math.round(diskPrediction.days_left)}일 후 부족 예상`}
              </div>
              <div className="text-xs font-medium opacity-70 tabular-nums">
                {diskPrediction.slope >= 0 ? '+' : ''}{diskPrediction.slope.toFixed(2)}%/h
              </div>
            </div>
          )}
          {hasAnomaly && !diskPrediction && (
            <div className="bg-purple-500/10 border border-purple-500/30 rounded-lg p-3">
              <p className="text-xs font-semibold text-purple-300 mb-1 flex items-center gap-1.5">
                <Info size={12} /> 이상 감지
              </p>
              <p className="text-xs font-medium text-purple-400/90">
                Z-점수 {anomaly.z_score.toFixed(2)} (평소 평균 {anomaly.mean.toFixed(1)}%)
              </p>
            </div>
          )}
        </div>
      )}

      {/* Chart Container */}
      <div className="h-28 w-full -mx-2">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={data} margin={{ top: 0, right: 0, left: 0, bottom: 0 }}>
            <defs>
              <linearGradient id={`grad-${title}`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={color} stopOpacity={0.15} />
                <stop offset="100%" stopColor={color} stopOpacity={0} />
              </linearGradient>
            </defs>
            <XAxis dataKey="time" hide />
            <YAxis domain={[0, 100]} hide />
            <Tooltip
              contentStyle={{ 
                backgroundColor: 'rgba(15, 23, 42, 0.9)', 
                border: 'none', 
                borderRadius: '16px',
                padding: '12px 16px',
                boxShadow: '0 20px 25px -5px rgb(0 0 0 / 0.1)',
                backdropFilter: 'blur(8px)'
              }}
              labelStyle={{ display: 'none' }}
              itemStyle={{ color: '#fff', fontSize: '12px', fontWeight: '600' }}
              formatter={(v) => [`${v}${unit}`, title]}
            />
            {hasAnomaly && (
              <ReferenceLine y={anomaly.mean} stroke="#a855f7" strokeDasharray="4 4" strokeWidth={1.5} opacity={0.5} />
            )}
            <Area 
              type="monotone" 
              dataKey="value" 
              stroke={color} 
              fill={`url(#grad-${title})`} 
              strokeWidth={3} 
              dot={false}
              animationDuration={1500}
              animationEasing="ease-in-out"
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </div>
  )
}
