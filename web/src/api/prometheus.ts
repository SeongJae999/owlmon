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

// 호스트 다운 여부 확인 (마지막 메트릭 수신 후 90초 초과 시 다운으로 판단)
export async function fetchHostStatus(host: string): Promise<'online' | 'offline'> {
  const res = await axios.get(`${PROMETHEUS_URL}/api/v1/query`, {
    params: { query: `(time() - timestamp(system_cpu_usage_percent{host_name="${host}"})) > 90` },
  })
  const result = res.data?.data?.result
  // 쿼리 결과가 있으면 90초 이상 데이터가 없는 것 → 오프라인
  return result && result.length > 0 ? 'offline' : 'online'
}

// 전체 호스트 상태 맵 조회
export async function fetchAllHostStatuses(hosts: string[]): Promise<Record<string, 'online' | 'offline'>> {
  const statuses = await Promise.all(hosts.map((h) => fetchHostStatus(h)))
  return Object.fromEntries(hosts.map((h, i) => [h, statuses[i]]))
}

// 특정 호스트의 현재 메트릭
export async function fetchMetrics(host?: string) {
  const filter = host ? `{host_name="${host}"}` : ''
  const [cpu, memory, disk] = await Promise.all([
    query(`system_cpu_usage_percent${filter}`),
    query(`system_memory_usage_percent${filter}`),
    query(`system_disk_usage_percent${filter}`),
  ])
  return { cpu, memory, disk }
}
