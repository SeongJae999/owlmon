import { AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer } from 'recharts'

interface Props {
  title: string
  value: number | null
  unit?: string
  data: { time: string; value: number }[]
  color: string
  warning?: number  // 경고 임계값
  critical?: number // 위험 임계값
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

export default function MetricCard({ title, value, unit = '%', data, color, warning, critical }: Props) {
  const status = getStatus(value, warning, critical)
  const displayColor = statusColor[status]

  return (
    <div style={{
      background: '#1e293b',
      border: `1px solid ${displayColor}33`,
      borderRadius: 12,
      padding: '20px 24px',
      display: 'flex',
      flexDirection: 'column',
      gap: 12,
    }}>
      {/* 헤더 */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <span style={{ color: '#94a3b8', fontSize: 14, fontWeight: 600 }}>{title}</span>
        <span style={{
          background: `${displayColor}22`,
          color: displayColor,
          padding: '2px 8px',
          borderRadius: 4,
          fontSize: 12,
          fontWeight: 700,
        }}>
          {status === 'normal' ? '정상' : status === 'warning' ? '경고' : status === 'critical' ? '위험' : '-'}
        </span>
      </div>

      {/* 현재 값 */}
      <div style={{ color: '#fff', fontSize: 36, fontWeight: 700, fontFamily: 'Consolas, monospace' }}>
        {value !== null ? `${value.toFixed(1)}${unit}` : '-'}
      </div>

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
            formatter={(v: number) => [`${v}${unit}`, title]}
          />
          <Area type="monotone" dataKey="value" stroke={color} fill={`url(#grad-${title})`} strokeWidth={2} dot={false} />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}
