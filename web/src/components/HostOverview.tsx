import type { ActiveAlert } from '../api/alert'

interface HostMetrics {
  cpu: number | null
  memory: number | null
  disk: number | null
}

interface Props {
  hosts: string[]
  hostStatuses: Record<string, 'online' | 'offline'>
  hostMetrics: Record<string, HostMetrics>
  activeAlerts: ActiveAlert[]
  onSelect: (host: string) => void
}

function MetricBar({ value, warning, critical }: { value: number | null; warning: number; critical: number }) {
  if (value === null) return <div style={{ color: '#475569', fontSize: 11 }}>-</div>

  const color = value >= critical ? '#ef4444' : value >= warning ? '#f59e0b' : '#22c55e'
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
      <div style={{ flex: 1, height: 4, background: '#334155', borderRadius: 2, overflow: 'hidden' }}>
        <div style={{ width: `${Math.min(value, 100)}%`, height: '100%', background: color, borderRadius: 2 }} />
      </div>
      <span style={{ color, fontSize: 11, fontWeight: 600, minWidth: 36, textAlign: 'right' }}>
        {value.toFixed(0)}%
      </span>
    </div>
  )
}

export default function HostOverview({ hosts, hostStatuses, hostMetrics, activeAlerts, onSelect }: Props) {
  return (
    <div style={{ marginBottom: 32 }}>
      <h2 style={{ color: '#94a3b8', fontSize: 13, fontWeight: 600, marginBottom: 12, letterSpacing: '0.05em', textTransform: 'uppercase' }}>
        전체 호스트 현황
      </h2>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))', gap: 12 }}>
        {hosts.map((host) => {
          const status = hostStatuses[host]
          const metrics = hostMetrics[host]
          const alertCount = activeAlerts.filter(a => a.host === host).length
          const isOffline = status === 'offline'

          return (
            <div
              key={host}
              onClick={() => onSelect(host)}
              style={{
                background: '#1e293b',
                border: `1px solid ${alertCount > 0 ? '#ef4444' : isOffline ? '#475569' : '#334155'}`,
                borderRadius: 10,
                padding: '14px 16px',
                cursor: 'pointer',
                transition: 'border-color 0.15s',
              }}
              onMouseEnter={e => (e.currentTarget.style.borderColor = '#7dd3fc')}
              onMouseLeave={e => (e.currentTarget.style.borderColor = alertCount > 0 ? '#ef4444' : isOffline ? '#475569' : '#334155')}
            >
              {/* 호스트 이름 + 상태 */}
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  <span style={{
                    width: 7, height: 7, borderRadius: '50%', flexShrink: 0,
                    background: isOffline ? '#ef4444' : '#22c55e',
                  }} />
                  <span style={{ color: '#e2e8f0', fontSize: 13, fontWeight: 600 }}>{host}</span>
                </div>
                {alertCount > 0 && (
                  <span style={{
                    background: '#7f1d1d', color: '#fca5a5',
                    fontSize: 10, fontWeight: 700, padding: '2px 6px', borderRadius: 10,
                  }}>
                    🚨 {alertCount}
                  </span>
                )}
                {isOffline && (
                  <span style={{
                    background: '#1e293b', color: '#64748b',
                    fontSize: 10, fontWeight: 600, padding: '2px 6px', borderRadius: 10,
                    border: '1px solid #334155',
                  }}>
                    오프라인
                  </span>
                )}
              </div>

              {/* 메트릭 바 */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <span style={{ color: '#475569', fontSize: 11, width: 28 }}>CPU</span>
                  <div style={{ flex: 1 }}>
                    <MetricBar value={metrics?.cpu ?? null} warning={70} critical={90} />
                  </div>
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <span style={{ color: '#475569', fontSize: 11, width: 28 }}>MEM</span>
                  <div style={{ flex: 1 }}>
                    <MetricBar value={metrics?.memory ?? null} warning={80} critical={95} />
                  </div>
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <span style={{ color: '#475569', fontSize: 11, width: 28 }}>DSK</span>
                  <div style={{ flex: 1 }}>
                    <MetricBar value={metrics?.disk ?? null} warning={85} critical={90} />
                  </div>
                </div>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
