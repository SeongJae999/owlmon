import { useQuery } from '@tanstack/react-query'
import { fetchHosts, fetchOnlineHosts, fetchAllHostMetrics, fetchMetrics, fetchServiceChecks } from '../api/prometheus'
import { getAlertStatus, getMaintenanceHosts, getAlertAcks } from '../api/alert'
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
    queryKey: ['hostStatuses'],
    queryFn: fetchOnlineHosts,
    // 2분 안에 데이터를 보낸 호스트만 결과에 있음 — 없는 호스트는 끊김 (판정 기준은 예전 호스트별 조회와 같음)
    select: (online: string[]) => Object.fromEntries(hosts.map(h => [h, online.includes(h) ? 'online' : 'offline'])) as Record<string, 'online' | 'offline'>,
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
    refetchIntervalInBackground: true, // 탭이 가려진 동안 이것만 멈추면 돌아왔을 때 "갱신 지연"으로 오판
  })
}

export const useAnomalyData = () => {
  return useQuery({
    queryKey: ['anomalyData'],
    queryFn: getAnomalyData,
    refetchInterval: 30000,
  })
}

export const useAlertAcks = () => {
  return useQuery({
    queryKey: ['alertAcks'],
    queryFn: getAlertAcks,
    refetchInterval: 30000,
    refetchIntervalInBackground: true,
  })
}
