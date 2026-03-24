import { AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer, ReferenceLine } from 'recharts'

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

const statusColor = {
  normal: '#22c55e',
  warning: '#f59e0b',
  critical: '#ef4444',
  unknown: '#475569',
}

export default function MetricCard({ title, value, unit = '%', data, color, warning, critical, anomaly, diskPrediction }: Props) {
  const status = getStatus(value, warning, critical)
  const hasAnomaly = anomaly != null
  const borderColor = hasAnomaly ? '#7c3aed' : statusColor[status]

  return (
    <div style={{
      background: hasAnomaly ? '#1a0a2e' : '#1e293b',
      border: `1px solid ${borderColor}${hasAnomaly ? '' : '33'}`,
      borderRadius: 12,
      padding: '20px 24px',
      display: 'flex',
      flexDirection: 'column',
      gap: 12,
    }}>
      {/* 헤더 */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <span style={{ color: '#94a3b8', fontSize: 14, fontWeight: 600 }}>{title}</span>
        <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
          {hasAnomaly && (
            <span style={{
              background: anomaly.severity === 'critical' ? '#7c3aed33' : '#7c3aed22',
              color: '#c4b5fd',
              padding: '2px 8px',
              borderRadius: 4,
              fontSize: 11,
              fontWeight: 700,
            }}>
              평소 대비 이상
            </span>
          )}
          <span style={{
            background: `${statusColor[status]}22`,
            color: statusColor[status],
            padding: '2px 8px',
            borderRadius: 4,
            fontSize: 12,
            fontWeight: 700,
          }}>
            {status === 'normal' ? '정상' : status === 'warning' ? '경고' : status === 'critical' ? '위험' : '-'}
          </span>
        </div>
      </div>

      {/* 현재 값 */}
      <div style={{ color: '#fff', fontSize: 36, fontWeight: 700, fontFamily: 'Consolas, monospace' }}>
        {value !== null ? `${value.toFixed(1)}${unit}` : '-'}
      </div>

      {/* 디스크 예측 */}
      {diskPrediction && diskPrediction.days_left >= 0 && diskPrediction.r2 >= 0.5 && (
        <div style={{
          background: diskPrediction.days_left <= 7 ? '#7f1d1d33' : '#78350f22',
          border: `1px solid ${diskPrediction.days_left <= 7 ? '#7f1d1d' : '#78350f'}`,
          borderRadius: 6,
          padding: '6px 10px',
          fontSize: 12,
          color: diskPrediction.days_left <= 7 ? '#fca5a5' : '#fcd34d',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
        }}>
          <span>
            {diskPrediction.days_left <= 1
              ? '24시간 내 용량 부족 예상'
              : `약 ${Math.round(diskPrediction.days_left)}일 후 용량 부족 예상`}
          </span>
          <span style={{ color: '#64748b', fontSize: 11 }}>
            {diskPrediction.slope >= 0 ? '+' : ''}{diskPrediction.slope.toFixed(2)}%/h
          </span>
        </div>
      )}

      {/* 차트 */}
      <ResponsiveContainer width="100%" height={80}>
        <AreaChart data={data} margin={{ top: 0, right: 0, left: 0, bottom: 0 }}>
          <defs>
            <linearGradient id={`grad-${title}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor={color} stopOpacity={0.3} />
              <stop offset="95%" stopColor={color} stopOpacity={0} />
            </linearGradient>
          </defs>
          <XAxis dataKey="time" hide />
          <YAxis domain={[0, 100]} hide />
          <Tooltip
            contentStyle={{ background: '#0f1117', border: '1px solid #334155', borderRadius: 6, fontSize: 12 }}
            labelStyle={{ color: '#94a3b8' }}
            itemStyle={{ color: '#e2e8f0' }}
            formatter={(v) => [`${v}${unit}`, title]}
          />
          {hasAnomaly && (
            <ReferenceLine y={anomaly.mean} stroke="#7c3aed" strokeDasharray="4 4" strokeWidth={1} />
          )}
          <Area type="monotone" dataKey="value" stroke={color} fill={`url(#grad-${title})`} strokeWidth={2} dot={false} />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}
