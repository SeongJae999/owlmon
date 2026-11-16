import { useCallback } from 'react'
import { useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { fetchHosts, fetchOnlineHosts, fetchAllHostMetrics, fetchMetrics, fetchServiceChecks } from '../api/prometheus'
import { getAlertStatus, getMaintenanceHosts, getAlertAcks } from '../api/alert'
import { fetchUptime, fetchHostFreshness } from '../api/asset'
import { getAnomalyData } from '../api/anomaly'

// 장비 상태 화면 재조회 주기 — 수집 주기(15초)와 맞춤. 서버가 상태 조회를 캐시해 열린 화면 수와 무관하게 부하가 일정함
export const REFRESH_MS = 15_000

// Prometheus 목록은 최근 5분 데이터가 있는 서버만 준다. 오래 꺼진 서버일수록 목록에서 사라져 "정상"처럼 보이므로,
// 서버가 down으로 계속 보고하는 호스트(최근 7일 내 수집 이력)를 합친다
let lastDown: string[] = []
async function fetchKnownHosts(qc: QueryClient): Promise<string[]> {
  const [live, alerts] = await Promise.all([
    fetchHosts(),
    // 알림 조회만 실패하면 직전에 알던 끊긴 서버를 유지 — 빈 값으로 두면 오래 끊긴 서버가 다시 사라짐.
    // 알림 상태는 useAlertStatus 캐시를 함께 써서 서버의 무거운 알림 조회를 두 번 부르지 않음
    qc.fetchQuery({ queryKey: ['alertStatus'], queryFn: getAlertStatus, staleTime: 25_000 }).then(a => (lastDown = a.filter(x => x.category === 'down').map(x => x.host))).catch(() => lastDown),
  ])
  return [...new Set([...live, ...alerts])].sort()
}

export const useHosts = () => {
  const qc = useQueryClient()
  return useQuery({
    queryKey: ['hosts'],
    queryFn: () => fetchKnownHosts(qc),
    refetchInterval: 60000,             // 호스트 목록 1분마다 갱신
    refetchIntervalInBackground: true,
  })
}

export const useAllHostStatuses = (hosts: string[]) => {
  // select 참조가 고정돼야 React Query가 결과를 재사용함 — 인라인이면 렌더마다 다시 계산
  const select = useCallback((online: string[]) => {
    const on = new Set(online)
    // 끊기지 않은 호스트만 결과에 있음 — 없는 호스트는 끊김 (서버 알림과 같은 기준)
    return Object.fromEntries(hosts.map(h => [h, on.has(h) ? 'online' : 'offline'])) as Record<string, 'online' | 'offline'>
  }, [hosts])
  return useQuery({
    queryKey: ['hostStatuses'],
    queryFn: fetchOnlineHosts,
    select,
    enabled: hosts.length > 0,
    refetchInterval: REFRESH_MS,
    refetchIntervalInBackground: true,
  })
}

export const useAllHostMetrics = () => {
  return useQuery({
    queryKey: ['allHostMetrics'],
    queryFn: fetchAllHostMetrics,
    refetchInterval: REFRESH_MS,
    refetchIntervalInBackground: true,
  })
}

export const useHostMetrics = (host: string) => {
  return useQuery({
    queryKey: ['metrics', host],
    queryFn: () => fetchMetrics(host),
    enabled: !!host,
    refetchInterval: REFRESH_MS,
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
    refetchInterval: REFRESH_MS,
    refetchIntervalInBackground: true, // 다른 탭을 보는 동안에도 탭 제목의 미확인 건수가 갱신되도록
  })
}

export const useHostFreshness = () => {
  return useQuery({
    queryKey: ['hostFreshness'],
    queryFn: fetchHostFreshness,
    refetchInterval: REFRESH_MS,
    refetchIntervalInBackground: true,
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
    refetchInterval: REFRESH_MS,
    refetchIntervalInBackground: true,
  })
}
