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
  uptimes: Record<string, number>
  maintenanceHosts: string[]
  onSelect: (host: string) => void
  onToggleMaintenance: (host: string, enabled: boolean) => void
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

function SummaryBox({ label, count, color }: { label: string; count: number; color: string }) {
  return (
    <div style={{
      background: '#1e293b', border: `1px solid ${color}44`,
      borderRadius: 8, padding: '12px 24px', textAlign: 'center',
    }}>
      <div style={{ fontSize: 28, fontWeight: 700, color }}>{count}</div>
      <div style={{ fontSize: 12, color: '#475569', marginTop: 2 }}>{label}</div>
    </div>
  )
}

export default function HostOverview({ hosts, hostStatuses, hostMetrics, activeAlerts, uptimes, maintenanceHosts, onSelect, onToggleMaintenance }: Props) {
  const maintenanceSet = new Set(maintenanceHosts)

  const counts = hosts.reduce(
    (acc, host) => {
      if (maintenanceSet.has(host)) { acc.maintenance++; return acc }
      const offline = hostStatuses[host] === 'offline'
      const hasCritical = activeAlerts.some(a => a.host === host && a.severity === 'critical')
      const hasWarning = activeAlerts.some(a => a.host === host && a.severity === 'warning')
      if (offline || hasCritical) acc.fault++
      else if (hasWarning) acc.warning++
      else acc.ok++
      return acc
    },
    { ok: 0, warning: 0, fault: 0, maintenance: 0 },
  )

  return (
    <div style={{ marginBottom: 32 }}>
      {/* 요약 바 */}
      <div style={{ display: 'flex', gap: 12, marginBottom: 24, flexWrap: 'wrap' }}>
        <SummaryBox label="전체" count={hosts.length} color="#7dd3fc" />
        <SummaryBox label="정상" count={counts.ok} color="#22c55e" />
        <SummaryBox label="경고" count={counts.warning} color="#f59e0b" />
        <SummaryBox label="장애" count={counts.fault} color="#ef4444" />
        {counts.maintenance > 0 && <SummaryBox label="유지보수" count={counts.maintenance} color="#a78bfa" />}
      </div>

      <h2 style={{ color: '#94a3b8', fontSize: 13, fontWeight: 600, marginBottom: 12, letterSpacing: '0.05em', textTransform: 'uppercase' }}>
        전체 호스트 현황
      </h2>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))', gap: 12 }}>
        {hosts.map((host) => {
          const status = hostStatuses[host]
          const metrics = hostMetrics[host]
          const alertCount = activeAlerts.filter(a => a.host === host && !a.acked).length
          const isOffline = status === 'offline'
          const uptime = uptimes[host]
          const inMaintenance = maintenanceSet.has(host)

          const borderColor = inMaintenance ? '#7c3aed' : alertCount > 0 ? '#ef4444' : isOffline ? '#475569' : '#334155'

          return (
            <div
              key={host}
              style={{
                background: inMaintenance ? '#1a1030' : '#1e293b',
                border: `1px solid ${borderColor}`,
                borderRadius: 10, padding: '14px 16px',
                opacity: inMaintenance ? 0.75 : 1,
              }}
            >
              {/* 헤더 */}
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 10 }}>
                <div
                  onClick={() => onSelect(host)}
                  style={{ display: 'flex', alignItems: 'center', gap: 6, cursor: 'pointer', flex: 1 }}
                >
                  <span style={{ width: 7, height: 7, borderRadius: '50%', flexShrink: 0, background: inMaintenance ? '#7c3aed' : isOffline ? '#ef4444' : '#22c55e' }} />
                  <span style={{ color: '#e2e8f0', fontSize: 13, fontWeight: 600 }}>{host}</span>
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                  {inMaintenance && (
                    <span style={{ background: '#2e1065', color: '#a78bfa', fontSize: 10, fontWeight: 700, padding: '2px 6px', borderRadius: 10 }}>
                      유지보수
                    </span>
                  )}
                  {!inMaintenance && alertCount > 0 && (
                    <span style={{ background: '#7f1d1d', color: '#fca5a5', fontSize: 10, fontWeight: 700, padding: '2px 6px', borderRadius: 10 }}>
                      🚨 {alertCount}
                    </span>
                  )}
                  {!inMaintenance && isOffline && alertCount === 0 && (
                    <span style={{ background: '#1e293b', color: '#64748b', fontSize: 10, fontWeight: 600, padding: '2px 6px', borderRadius: 10, border: '1px solid #334155' }}>
                      오프라인
                    </span>
                  )}
                </div>
              </div>

              {/* 메트릭 바 */}
              <div
                onClick={() => onSelect(host)}
                style={{ display: 'flex', flexDirection: 'column', gap: 6, cursor: 'pointer' }}
              >
                {(['CPU', 'MEM', 'DSK'] as const).map((label, i) => {
                  const val = [metrics?.cpu, metrics?.memory, metrics?.disk][i] ?? null
                  const [w, c] = [[70, 90], [80, 95], [85, 90]][i]
                  return (
                    <div key={label} style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                      <span style={{ color: '#475569', fontSize: 11, width: 28 }}>{label}</span>
                      <div style={{ flex: 1 }}>
                        <MetricBar value={val} warning={w} critical={c} />
                      </div>
                    </div>
                  )
                })}
              </div>

              {/* 하단: 가동률 + 유지보수 토글 */}
              <div style={{ marginTop: 10, paddingTop: 10, borderTop: '1px solid #334155', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                {uptime !== undefined ? (
                  <span style={{ fontSize: 12, fontWeight: 700, color: uptime >= 99 ? '#22c55e' : uptime >= 95 ? '#f59e0b' : '#ef4444' }}>
                    {uptime.toFixed(1)}% 가동
                  </span>
                ) : (
                  <span />
                )}
                <button
                  onClick={() => onToggleMaintenance(host, !inMaintenance)}
                  style={{
                    background: inMaintenance ? '#2e1065' : 'none',
                    border: `1px solid ${inMaintenance ? '#7c3aed' : '#334155'}`,
                    color: inMaintenance ? '#a78bfa' : '#475569',
                    padding: '2px 8px', borderRadius: 4, cursor: 'pointer', fontSize: 11,
                  }}
                >
                  {inMaintenance ? '유지보수 해제' : '유지보수'}
                </button>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
