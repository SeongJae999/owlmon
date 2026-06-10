import { useNavigate } from 'react-router-dom'
import HostOverview from '../components/HostOverview'
import { 
  useHosts, 
  useAllHostStatuses, 
  useAllHostMetrics, 
  useAlertStatus, 
  useUptimes, 
  useMaintenanceHosts 
} from '../hooks/useMonitoring'
import { setMaintenance } from '../api/alert'
import { RefreshCcw } from 'lucide-react'

export default function OverviewPage() {
  const navigate = useNavigate()
  
  const { data: hosts = [], isLoading: hostsLoading } = useHosts()
  const { data: hostStatuses = {} } = useAllHostStatuses(hosts)
  const { data: hostMetrics = {} } = useAllHostMetrics()
  const { data: activeAlerts = [], refetch: refetchAlerts } = useAlertStatus()
  const { data: uptimes = {} } = useUptimes()
  const { data: maintenanceHosts = [], refetch: refetchMaintenance } = useMaintenanceHosts()

  const onToggleMaintenance = async (host: string, enabled: boolean) => {
    await setMaintenance(host, enabled)
    refetchMaintenance()
    refetchAlerts()
  }

  if (hostsLoading) {
    return (
      <div className="flex flex-col items-center justify-center h-[60vh] text-slate-500 space-y-4 animate-pulse">
        <RefreshCcw size={48} className="animate-spin opacity-30" />
        <p className="text-sm font-semibold">호스트 정보를 불러오는 중…</p>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Page Hero */}
      <div>
        <h1 className="text-xl font-bold text-slate-100">시스템 상태 대시보드</h1>
        <p className="text-slate-400 text-sm mt-1">
          모든 호스트의 실시간 성능과 에이전트 연결 상태, 이상 탐지를 한눈에 확인합니다.
        </p>
      </div>

      <HostOverview
        hosts={hosts}
        hostStatuses={hostStatuses}
        hostMetrics={hostMetrics}
        activeAlerts={activeAlerts}
        uptimes={uptimes}
        maintenanceHosts={maintenanceHosts}
        onSelect={(host) => navigate(`/detail?host=${host}`)}
        onToggleMaintenance={onToggleMaintenance}
      />
    </div>
  )
}
