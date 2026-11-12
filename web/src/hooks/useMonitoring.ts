import { useQuery } from '@tanstack/react-query'
import { fetchHosts, fetchAllHostStatuses, fetchAllHostMetrics, fetchMetrics, fetchServiceChecks } from '../api/prometheus'
import { getAlertStatus, getMaintenanceHosts } from '../api/alert'
import { fetchUptime } from '../api/asset'
import { getAnomalyData } from '../api/anomaly'

// Prometheus 목록은 최근 5분 데이터가 있는 서버만 준다. 오래 꺼진 서버일수록 목록에서 사라져 "정상"처럼 보이므로,
// 서버가 down으로 계속 보고하는 호스트(최근 7일 내 수집 이력)를 합친다
let lastDown: string[] = []
async function fetchKnownHosts(): Promise<string[]> {
  const [live, alerts] = await Promise.all([
    fetchHosts(),
    // 알림 조회만 실패하면 직전에 알던 끊긴 서버를 유지 — 빈 값으로 두면 오래 끊긴 서버가 다시 사라짐
    getAlertStatus().then(a => (lastDown = a.filter(x => x.category === 'down').map(x => x.host))).catch(() => lastDown),
  ])
  return [...new Set([...live, ...alerts])].sort()
}

// 부모가 거는 조회의 상태(실패·갱신 시각)만 구독 — enabled:false라 마운트 때 조회를 새로 걸지 않는다.
// 자식에서 useHosts()를 또 쓰면 실패 상태에서 재조회가 부모 로딩과 맞물려 무한 재마운트된다.
// 조회 함수는 그대로 두어 다른 곳에서 캐시를 무효화해도 원래 함수로 다시 조회됨
export const useHostsStatus = () => {
  const { isError, dataUpdatedAt } = useQuery({ queryKey: ['hosts'], queryFn: fetchKnownHosts, enabled: false })
  return { isError, dataUpdatedAt }
}
export const useHostStatusesStatus = (hosts: string[]) => {
  const { isError, dataUpdatedAt } = useQuery({ queryKey: ['hostStatuses', hosts], queryFn: () => fetchAllHostStatuses(hosts), enabled: false })
  return { isError, dataUpdatedAt }
}

export const useHosts = () => {
  return useQuery({
    queryKey: ['hosts'],
    queryFn: fetchKnownHosts,
    refetchInterval: 60000,             // 호스트 목록 1분마다 갱신
    refetchIntervalInBackground: true,
  })
}

export const useAllHostStatuses = (hosts: string[]) => {
  return useQuery({
    queryKey: ['hostStatuses', hosts],
    queryFn: () => fetchAllHostStatuses(hosts),
    enabled: hosts.length > 0,
    refetchInterval: 30000,
    refetchIntervalInBackground: true,
  })
}

export const useAllHostMetrics = () => {
  return useQuery({
    queryKey: ['allHostMetrics'],
    queryFn: fetchAllHostMetrics,
    refetchInterval: 30000,
    refetchIntervalInBackground: true,
  })
}

export const useHostMetrics = (host: string) => {
  return useQuery({
    queryKey: ['metrics', host],
    queryFn: () => fetchMetrics(host),
    enabled: !!host,
    refetchInterval: 30000,
  })
}

export const useServiceChecks = (host: string) => {
  return useQuery({
    queryKey: ['serviceChecks', host],
    queryFn: () => fetchServiceChecks(host),
    enabled: !!host,
    refetchInterval: 30000,
  })
}

export const useAlertStatus = () => {
  return useQuery({
    queryKey: ['alertStatus'],
    queryFn: getAlertStatus,
    refetchInterval: 30000,
    refetchIntervalInBackground: true, // 다른 탭을 보는 동안에도 탭 제목의 미확인 건수가 갱신되도록
  })
}

export const useUptimes = () => {
  return useQuery({
    queryKey: ['uptimes'],
    queryFn: fetchUptime,
    refetchInterval: 60000,
  })
}

export const useMaintenanceHosts = () => {
  return useQuery({
    queryKey: ['maintenanceHosts'],
    queryFn: getMaintenanceHosts,
    refetchInterval: 60000,
  })
}

export const useAnomalyData = () => {
  return useQuery({
    queryKey: ['anomalyData'],
    queryFn: getAnomalyData,
    refetchInterval: 30000,
  })
}
