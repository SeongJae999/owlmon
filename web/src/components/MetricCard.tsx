import React from 'react'
import { AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer, ReferenceLine, RadialBarChart, RadialBar, PolarAngleAxis } from 'recharts'
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
  // 절대값 표시 (예: "12.3 / 16.0 GB") — 둘 다 있으면 표시
  usedBytes?: number | null
  totalBytes?: number | null
}

// 사람 친화적 용량 포맷 (TB/GB/MB)
function formatBytes(bytes: number): string {
  const tb = 1024 ** 4
  const gb = 1024 ** 3
  const mb = 1024 ** 2
  if (bytes >= tb) return `${(bytes / tb).toFixed(1)} TB`
  if (bytes >= gb) return `${(bytes / gb).toFixed(1)} GB`
  return `${(bytes / mb).toFixed(0)} MB`
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

export default function MetricCard({ title, value, unit = '%', data, color, warning, critical, anomaly, diskPrediction, usedBytes, totalBytes }: Props) {
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

      {/* Current Value Block — gauge ring + 숫자 + 남은 용량 */}
      <div className="flex items-center gap-4 mb-3">
        {/* Gauge ring — 반원 차트 */}
        {value !== null && (
          <div className="relative w-20 h-20 shrink-0">
            <ResponsiveContainer width="100%" height="100%">
              <RadialBarChart
                innerRadius="70%"
                outerRadius="100%"
                data={[{ value: Math.min(value, 100), fill: cfg.color }]}
                startAngle={90}
                endAngle={-270}
              >
                <PolarAngleAxis type="number" domain={[0, 100]} tick={false} />
                <RadialBar background={{ fill: '#1e293b' }} dataKey="value" cornerRadius={6} />
              </RadialBarChart>
            </ResponsiveContainer>
            <div className="absolute inset-0 flex flex-col items-center justify-center">
              <span className="text-xs font-bold text-slate-100 tabular-nums leading-none">
                {value.toFixed(0)}
              </span>
              <span className="text-[8px] text-slate-500 leading-none">{unit}</span>
            </div>
          </div>
        )}

        {/* 숫자 + 절대값 + 남은 용량 */}
        <div className="flex-1 min-w-0">
          <div className="flex items-baseline gap-1.5 leading-none">
            <span className="text-3xl font-bold text-slate-100 tabular-nums">
              {value !== null ? value.toFixed(1) : '--'}
            </span>
            <span className="text-sm font-medium text-slate-500">{unit}</span>
          </div>
          {usedBytes != null && totalBytes != null && totalBytes > 0 ? (
            <>
              <div className="text-xs font-semibold text-slate-400 tabular-nums mt-1.5">
                {formatBytes(usedBytes)} <span className="text-slate-600">/</span> {formatBytes(totalBytes)}
              </div>
              <div className={cn(
                "text-[11px] font-bold tabular-nums mt-0.5",
                value !== null && value >= (critical ?? 90) ? "text-rose-300"
                  : value !== null && value >= (warning ?? 70) ? "text-amber-300"
                  : "text-emerald-300"
              )}>
                남은 용량: {formatBytes(Math.max(0, totalBytes - usedBytes))}
              </div>
            </>
          ) : (
            <div className="text-[11px] text-slate-500 mt-1.5">실시간 사용률</div>
          )}
        </div>
      </div>

      {/* Progress bar — 시각적 비율 */}
      {value !== null && (
        <div className="mb-4">
          <div className="h-1.5 w-full bg-slate-800 rounded-full overflow-hidden">
            <div
              className="h-full rounded-full transition-all duration-300"
              style={{
                width: `${Math.min(value, 100)}%`,
                background: cfg.color,
              }}
            />
          </div>
        </div>
      )}

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

      {/* Chart Container — recharts SVG가 클릭 시 브라우저 기본 focus outline 표시되는 거 제거 */}
      <div className="h-28 w-full -mx-2 outline-none [&_*]:outline-none [&_svg]:focus:outline-none">
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
