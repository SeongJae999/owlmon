import { useEffect, useState, useCallback } from 'react'
import { AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer } from 'recharts'
import MetricCard from './components/MetricCard'
import ServiceCheckCard from './components/ServiceCheckCard'
import LoginPage from './components/LoginPage'
import AlertSettings from './components/AlertSettings'
import AlertHistory from './components/AlertHistory'
import { fetchMetrics, fetchHosts, fetchAllHostStatuses, fetchServiceChecks, queryRange } from './api/prometheus'
import { isLoggedIn, logout } from './api/auth'
import { getAlertConfig, getAlertStatus, type AlertConfig, type ActiveAlert } from './api/alert'
import type { ServiceCheck } from './api/prometheus'

interface Metrics {
  cpu: number | null
  memory: number | null
  disk: number | null
  rx: number | null
  tx: number | null
}

interface ChartData {
  cpu: { time: string; value: number }[]
  memory: { time: string; value: number }[]
  disk: { time: string; value: number }[]
  rx: { time: string; value: number }[]
  tx: { time: string; value: number }[]
}

export default function App() {
  const [loggedIn, setLoggedIn] = useState(isLoggedIn())

  if (!loggedIn) {
    return <LoginPage onLogin={() => setLoggedIn(true)} />
  }

  return <Dashboard onLogout={() => { logout(); setLoggedIn(false) }} />
}

function Dashboard({ onLogout }: { onLogout: () => void }) {
  const [hosts, setHosts] = useState<string[]>([])
  const [hostStatuses, setHostStatuses] = useState<Record<string, 'online' | 'offline'>>({})
  const [selectedHost, setSelectedHost] = useState<string>('')
  const [metrics, setMetrics] = useState<Metrics>({ cpu: null, memory: null, disk: null, rx: null, tx: null })
  const [chartData, setChartData] = useState<ChartData>({ cpu: [], memory: [], disk: [], rx: [], tx: [] })
  const [serviceChecks, setServiceChecks] = useState<ServiceCheck[]>([])
  const [lastUpdated, setLastUpdated] = useState<string>('-')
  const [showAlertSettings, setShowAlertSettings] = useState(false)
  const [showAlertHistory, setShowAlertHistory] = useState(false)
  const [alertCfg, setAlertCfg] = useState<AlertConfig | null>(null)
  const [activeAlerts, setActiveAlerts] = useState<ActiveAlert[]>([])

  useEffect(() => {
    getAlertConfig().then(setAlertCfg).catch(() => {})
  }, [])

  // 호스트 목록 초기 로드
  useEffect(() => {
    fetchHosts().then((list) => {
      setHosts(list)
      if (list.length > 0) setSelectedHost(list[0])
    })
  }, [])

  const refresh = useCallback(async () => {
    if (!selectedHost || hosts.length === 0) return
    const [current, cpuRange, memRange, diskRange, rxRange, txRange, statuses, checks, alerts] = await Promise.all([
      fetchMetrics(selectedHost),
      queryRange(`system_cpu_usage_percent{host_name="${selectedHost}"}`),
      queryRange(`system_memory_usage_percent{host_name="${selectedHost}"}`),
      queryRange(`max(system_disk_usage_percent{host_name="${selectedHost}"})`),
      queryRange(`sum(system_network_rx_bytes_per_second{host_name="${selectedHost}"})`),
      queryRange(`sum(system_network_tx_bytes_per_second{host_name="${selectedHost}"})`),
      fetchAllHostStatuses(hosts),
      fetchServiceChecks(selectedHost),
      getAlertStatus(),
    ])
    setMetrics(current)
    setChartData({ cpu: cpuRange, memory: memRange, disk: diskRange, rx: rxRange, tx: txRange })
    setHostStatuses(statuses)
    setServiceChecks(checks)
    setActiveAlerts(alerts)
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
            <button onClick={refresh} style={{ background: '#1e293b', border: '1px solid #334155', color: '#94a3b8', padding: '6px 14px', borderRadius: 6, cursor: 'pointer', fontSize: 13 }}>
              새로고침
            </button>
            <button onClick={() => setShowAlertHistory(true)} style={{ background: '#1e293b', border: '1px solid #334155', color: '#94a3b8', padding: '6px 14px', borderRadius: 6, cursor: 'pointer', fontSize: 13 }}>
              알림 히스토리
            </button>
            <button onClick={() => setShowAlertSettings(true)} style={{ background: '#1e293b', border: '1px solid #334155', color: '#94a3b8', padding: '6px 14px', borderRadius: 6, cursor: 'pointer', fontSize: 13 }}>
              알림 설정
            </button>
            <button onClick={onLogout} style={{ background: 'none', border: '1px solid #334155', color: '#475569', padding: '6px 14px', borderRadius: 6, cursor: 'pointer', fontSize: 13 }}>
              로그아웃
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

        {/* 활성 알림 배너 */}
        {activeAlerts.length > 0 && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8, marginBottom: 16 }}>
            {activeAlerts.map((a, i) => (
              <div key={i} style={{
                background: a.severity === 'critical' ? '#450a0a' : '#422006',
                border: `1px solid ${a.severity === 'critical' ? '#ef4444' : '#f59e0b'}`,
                borderRadius: 8, padding: '10px 16px',
                color: a.severity === 'critical' ? '#fca5a5' : '#fcd34d',
                fontSize: 13, display: 'flex', alignItems: 'center', gap: 10,
              }}>
                <span>{a.severity === 'critical' ? '🚨' : '⚠️'}</span>
                <span><strong>{a.host}</strong> — {a.message}</span>
              </div>
            ))}
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
          <MetricCard title="CPU 사용률" value={metrics.cpu} data={chartData.cpu} color="#7dd3fc" warning={alertCfg ? alertCfg.cpu_threshold * 0.8 : 70} critical={alertCfg?.cpu_threshold ?? 90} />
          <MetricCard title="메모리 사용률" value={metrics.memory} data={chartData.memory} color="#a78bfa" warning={alertCfg ? alertCfg.mem_threshold * 0.85 : 80} critical={alertCfg?.mem_threshold ?? 95} />
          <MetricCard title="디스크 사용률" value={metrics.disk} data={chartData.disk} color="#34d399" warning={alertCfg?.disk_warn ?? 85} critical={alertCfg?.disk_crit ?? 90} />
          <NetworkCard title="네트워크 수신 (RX)" valueBps={metrics.rx} data={chartData.rx} color="#f472b6" />
          <NetworkCard title="네트워크 송신 (TX)" valueBps={metrics.tx} data={chartData.tx} color="#fb923c" />
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

      {showAlertHistory && <AlertHistory onClose={() => setShowAlertHistory(false)} />}
      {showAlertSettings && <AlertSettings onClose={() => {
        setShowAlertSettings(false)
        getAlertConfig().then(setAlertCfg).catch(() => {})
      }} />}
    </div>
  )
}

function formatBps(bps: number | null): string {
  if (bps === null) return '-'
  if (bps >= 1024 * 1024) return `${(bps / 1024 / 1024).toFixed(1)} MB/s`
  if (bps >= 1024) return `${(bps / 1024).toFixed(1)} KB/s`
  return `${bps.toFixed(0)} B/s`
}

function NetworkCard({ title, valueBps, data, color }: {
  title: string
  valueBps: number | null
  data: { time: string; value: number }[]
  color: string
}) {
  return (
    <div style={{ background: '#1e293b', borderRadius: 12, padding: '20px 24px', border: '1px solid #334155' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 12 }}>
        <span style={{ color: '#94a3b8', fontSize: 13, fontWeight: 500 }}>{title}</span>
      </div>
      <div style={{ fontSize: 28, fontWeight: 700, color: '#f1f5f9', marginBottom: 16 }}>
        {formatBps(valueBps)}
      </div>
      <ResponsiveContainer width="100%" height={80}>
        <AreaChart data={data} margin={{ top: 0, right: 0, left: 0, bottom: 0 }}>
          <defs>
            <linearGradient id={`grad-net-${title}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor={color} stopOpacity={0.3} />
              <stop offset="95%" stopColor={color} stopOpacity={0} />
            </linearGradient>
          </defs>
          <XAxis dataKey="time" hide />
          <YAxis hide />
          <Tooltip
            contentStyle={{ background: '#0f1117', border: '1px solid #334155', borderRadius: 6, fontSize: 12 }}
            labelStyle={{ color: '#94a3b8' }}
            itemStyle={{ color: '#e2e8f0' }}
            formatter={(v) => [formatBps(typeof v === 'number' ? v : null), title]}
          />
          <Area type="monotone" dataKey="value" stroke={color} fill={`url(#grad-net-${title})`} strokeWidth={2} dot={false} />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}
