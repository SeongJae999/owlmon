import { useEffect, useState, useCallback } from 'react'
import MetricCard from './components/MetricCard'
import ServiceCheckCard from './components/ServiceCheckCard'
import { fetchMetrics, fetchHosts, fetchAllHostStatuses, fetchServiceChecks, queryRange } from './api/prometheus'
import type { ServiceCheck } from './api/prometheus'

interface Metrics {
  cpu: number | null
  memory: number | null
  disk: number | null
}

interface ChartData {
  cpu: { time: string; value: number }[]
  memory: { time: string; value: number }[]
  disk: { time: string; value: number }[]
}

export default function App() {
  const [hosts, setHosts] = useState<string[]>([])
  const [hostStatuses, setHostStatuses] = useState<Record<string, 'online' | 'offline'>>({})
  const [selectedHost, setSelectedHost] = useState<string>('')
  const [metrics, setMetrics] = useState<Metrics>({ cpu: null, memory: null, disk: null })
  const [chartData, setChartData] = useState<ChartData>({ cpu: [], memory: [], disk: [] })
  const [serviceChecks, setServiceChecks] = useState<ServiceCheck[]>([])
  const [lastUpdated, setLastUpdated] = useState<string>('-')

  // 호스트 목록 초기 로드
  useEffect(() => {
    fetchHosts().then((list) => {
      setHosts(list)
      if (list.length > 0) setSelectedHost(list[0])
    })
  }, [])

  const refresh = useCallback(async () => {
    if (!selectedHost || hosts.length === 0) return
    const [current, cpuRange, memRange, diskRange, statuses, checks] = await Promise.all([
      fetchMetrics(selectedHost),
      queryRange(`system_cpu_usage_percent{host_name="${selectedHost}"}`),
      queryRange(`system_memory_usage_percent{host_name="${selectedHost}"}`),
      queryRange(`system_disk_usage_percent{host_name="${selectedHost}"}`),
      fetchAllHostStatuses(hosts),
      fetchServiceChecks(selectedHost),
    ])
    setMetrics(current)
    setChartData({ cpu: cpuRange, memory: memRange, disk: diskRange })
    setHostStatuses(statuses)
    setServiceChecks(checks)
    setLastUpdated(new Date().toLocaleTimeString('ko-KR'))
  }, [selectedHost, hosts])

  useEffect(() => {
    refresh()
    const timer = setInterval(refresh, 30_000)
    return () => clearInterval(timer)
  }, [refresh])

  const isOffline = hostStatuses[selectedHost] === 'offline'

  return (
    <div style={{ background: '#0f1117', minHeight: '100vh', padding: '32px 24px', fontFamily: 'Segoe UI, sans-serif' }}>
      <div style={{ maxWidth: 1100, margin: '0 auto' }}>

        {/* 헤더 */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end', marginBottom: 24 }}>
          <div>
            <h1 style={{ color: '#fff', fontSize: 24, fontWeight: 700, margin: 0 }}>OWLmon</h1>
            <p style={{ color: '#475569', fontSize: 13, margin: '4px 0 0' }}>시스템 모니터링 대시보드</p>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <span style={{ color: '#475569', fontSize: 13 }}>마지막 갱신: {lastUpdated}</span>
            <button
              onClick={refresh}
              style={{
                background: '#1e293b',
                border: '1px solid #334155',
                color: '#94a3b8',
                padding: '6px 14px',
                borderRadius: 6,
                cursor: 'pointer',
                fontSize: 13,
              }}
            >
              새로고침
            </button>
          </div>
        </div>

        {/* 호스트 탭 */}
        {hosts.length > 0 && (
          <div style={{ display: 'flex', gap: 8, marginBottom: 28, borderBottom: '1px solid #1e293b' }}>
            {hosts.map((host) => {
              const status = hostStatuses[host]
              const dotColor = status === 'offline' ? '#ef4444' : status === 'online' ? '#22c55e' : '#475569'
              return (
                <button
                  key={host}
                  onClick={() => setSelectedHost(host)}
                  style={{
                    background: 'none',
                    border: 'none',
                    borderBottom: selectedHost === host ? '2px solid #7dd3fc' : '2px solid transparent',
                    color: selectedHost === host ? '#7dd3fc' : '#475569',
                    padding: '8px 16px',
                    cursor: 'pointer',
                    fontSize: 13,
                    fontWeight: selectedHost === host ? 600 : 400,
                    marginBottom: -1,
                    display: 'flex',
                    alignItems: 'center',
                    gap: 6,
                  }}
                >
                  <span style={{ width: 7, height: 7, borderRadius: '50%', background: dotColor, flexShrink: 0 }} />
                  {host}
                </button>
              )
            })}
          </div>
        )}

        {/* 오프라인 배너 */}
        {isOffline && (
          <div style={{
            background: '#450a0a',
            border: '1px solid #ef4444',
            borderRadius: 8,
            padding: '12px 16px',
            marginBottom: 20,
            color: '#fca5a5',
            fontSize: 13,
            display: 'flex',
            alignItems: 'center',
            gap: 8,
          }}>
            <span>●</span>
            <span><strong>{selectedHost}</strong> — 에이전트 연결이 끊겼습니다. 최근 1시간 내 마지막 수집 값을 표시합니다.</span>
          </div>
        )}

        {/* 시스템 메트릭 */}
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(300px, 1fr))', gap: 20, marginBottom: 32 }}>
          <MetricCard title="CPU 사용률" value={metrics.cpu} data={chartData.cpu} color="#7dd3fc" warning={70} critical={90} />
          <MetricCard title="메모리 사용률" value={metrics.memory} data={chartData.memory} color="#a78bfa" warning={80} critical={95} />
          <MetricCard title="디스크 사용률" value={metrics.disk} data={chartData.disk} color="#34d399" warning={75} critical={90} />
        </div>

        {/* 서비스 체크 */}
        {serviceChecks.length > 0 && (
          <>
            <h2 style={{ color: '#94a3b8', fontSize: 13, fontWeight: 600, marginBottom: 12, letterSpacing: '0.05em', textTransform: 'uppercase' }}>
              서비스 체크
            </h2>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(220px, 1fr))', gap: 12 }}>
              {serviceChecks.map((check) => (
                <ServiceCheckCard
                  key={check.name}
                  name={check.name}
                  type={check.type}
                  target={check.target}
                  status={check.status}
                  latencyMs={check.latencyMs}
                />
              ))}
            </div>
          </>
        )}

      </div>
    </div>
  )
}
