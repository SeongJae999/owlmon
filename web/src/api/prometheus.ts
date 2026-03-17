import axios from 'axios'

const PROMETHEUS_URL = import.meta.env.VITE_PROMETHEUS_URL ?? ''

// Prometheus instant query (현재 값)
async function query(promql: string): Promise<number | null> {
  const res = await axios.get(`${PROMETHEUS_URL}/api/v1/query`, {
    params: { query: promql },
  })
  const result = res.data?.data?.result
  if (!result || result.length === 0) return null
  return parseFloat(result[0].value[1])
}

// Prometheus range query (시계열)
export async function queryRange(
  promql: string,
  minutes = 30
): Promise<{ time: string; value: number }[]> {
  const end = Math.floor(Date.now() / 1000)
  const start = end - minutes * 60

  const res = await axios.get(`${PROMETHEUS_URL}/api/v1/query_range`, {
    params: { query: promql, start, end, step: '30s' },
  })
  const result = res.data?.data?.result
  if (!result || result.length === 0) return []

  return result[0].values.map(([ts, val]: [number, string]) => ({
    time: new Date(ts * 1000).toLocaleTimeString('ko-KR', { hour: '2-digit', minute: '2-digit' }),
    value: parseFloat(parseFloat(val).toFixed(1)),
  }))
}

// 연결된 호스트 목록 조회
export async function fetchHosts(): Promise<string[]> {
  const res = await axios.get(`${PROMETHEUS_URL}/api/v1/label/host_name/values`)
  return res.data?.data ?? []
}

// 호스트 다운 여부 확인 (최근 2분 내 데이터가 있으면 온라인)
export async function fetchHostStatus(host: string): Promise<'online' | 'offline'> {
  const res = await axios.get(`${PROMETHEUS_URL}/api/v1/query`, {
    params: { query: `count_over_time(system_cpu_usage_percent{host_name="${host}"}[2m])` },
  })
  const result = res.data?.data?.result
  // 결과 있음 → 최근 2분 내 데이터 수신 → 온라인
  // 결과 없음 → 데이터 없거나 만료 → 오프라인
  return result && result.length > 0 ? 'online' : 'offline'
}

// 전체 호스트 상태 맵 조회
export async function fetchAllHostStatuses(hosts: string[]): Promise<Record<string, 'online' | 'offline'>> {
  const statuses = await Promise.all(hosts.map((h) => fetchHostStatus(h)))
  return Object.fromEntries(hosts.map((h, i) => [h, statuses[i]]))
}

export interface ServiceCheck {
  name: string
  type: string
  target: string
  status: number
  latencyMs: number | null
}

// 특정 호스트의 서비스 체크 결과 조회
export async function fetchServiceChecks(host: string): Promise<ServiceCheck[]> {
  const [statusRes, latencyRes] = await Promise.all([
    axios.get(`${PROMETHEUS_URL}/api/v1/query`, {
      params: { query: `service_check_status{host_name="${host}"}` },
    }),
    axios.get(`${PROMETHEUS_URL}/api/v1/query`, {
      params: { query: `service_check_latency_ms{host_name="${host}"}` }, // unit 없이 정의했으므로 suffix 없음
    }),
  ])

  const statusResults = statusRes.data?.data?.result ?? []
  const latencyResults = latencyRes.data?.data?.result ?? []

  // latency 맵 생성 (check_name 기준)
  const latencyMap: Record<string, number> = {}
  for (const r of latencyResults) {
    latencyMap[r.metric.check_name] = parseFloat(r.value[1])
  }

  return statusResults.map((r: { metric: Record<string, string>; value: [number, string] }) => ({
    name: r.metric.check_name,
    type: r.metric.check_type,
    target: r.metric.target,
    status: parseFloat(r.value[1]),
    latencyMs: latencyMap[r.metric.check_name] ?? null,
  }))
}

// 특정 호스트의 현재 메트릭
// 오프라인 상태일 경우 최근 1시간 내 마지막 값을 반환
export async function fetchMetrics(host?: string) {
  const filter = host ? `{host_name="${host}"}` : ''

  // last_over_time: 지정 범위 내 마지막 값 반환 (오프라인 시에도 직전 값 표시)
  const [cpu, memory, disk] = await Promise.all([
    query(`last_over_time(system_cpu_usage_percent${filter}[1h])`),
    query(`last_over_time(system_memory_usage_percent${filter}[1h])`),
    query(`last_over_time(system_disk_usage_percent${filter}[1h])`),
  ])
  return { cpu, memory, disk }
}
