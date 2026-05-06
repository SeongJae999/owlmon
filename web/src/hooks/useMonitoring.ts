import { useQuery } from '@tanstack/react-query'
import { fetchHosts, fetchAllHostStatuses, fetchAllHostMetrics, fetchMetrics, fetchServiceChecks } from '../api/prometheus'
import { getAlertStatus, getMaintenanceHosts } from '../api/alert'
import { fetchUptime } from '../api/asset'
import { getAnomalyData } from '../api/anomaly'

export const useHosts = () => {
  return useQuery({
    queryKey: ['hosts'],
    queryFn: fetchHosts,
  })
}

export const useAllHostStatuses = (hosts: string[]) => {
  return useQuery({
    queryKey: ['hostStatuses', hosts],
    queryFn: () => fetchAllHostStatuses(hosts),
    enabled: hosts.length > 0,
    refetchInterval: 30000,
  })
}

export const useAllHostMetrics = () => {
  return useQuery({
    queryKey: ['allHostMetrics'],
    queryFn: fetchAllHostMetrics,
    refetchInterval: 30000,
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
