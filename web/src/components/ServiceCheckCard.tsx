interface Props {
  name: string
  type: string
  target: string
  status: number  // 1 = 정상, 0 = 장애
  latencyMs: number | null
}

export default function ServiceCheckCard({ name, type, target, status, latencyMs }: Props) {
  const isUp = status === 1
  const statusColor = isUp ? '#22c55e' : '#ef4444'
  const statusText = isUp ? '정상' : '장애'

  return (
    <div style={{
      background: '#1e293b',
      border: `1px solid ${statusColor}33`,
      borderRadius: 12,
      padding: '16px 20px',
      display: 'flex',
      flexDirection: 'column',
      gap: 8,
    }}>
      {/* 헤더 */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <span style={{ color: '#e2e8f0', fontWeight: 600, fontSize: 14 }}>{name}</span>
        <span style={{
          background: `${statusColor}22`,
          color: statusColor,
          padding: '2px 8px',
          borderRadius: 4,
          fontSize: 12,
          fontWeight: 700,
        }}>
          {statusText}
        </span>
      </div>

      {/* 응답시간 */}
      <div style={{ color: '#fff', fontSize: 24, fontWeight: 700, fontFamily: 'Consolas, monospace' }}>
        {latencyMs !== null ? `${latencyMs.toFixed(0)}ms` : '-'}
      </div>

      {/* 타입 + 대상 */}
      <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
        <span style={{
          background: '#334155',
          color: '#94a3b8',
          padding: '1px 6px',
          borderRadius: 3,
          fontSize: 11,
          fontWeight: 700,
          textTransform: 'uppercase',
        }}>
          {type}
        </span>
        <span style={{ color: '#475569', fontSize: 12, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {target}
        </span>
      </div>
    </div>
  )
}
