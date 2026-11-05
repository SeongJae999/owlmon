import { useNavigate } from 'react-router-dom'
import HostOverview from '../components/HostOverview'
import ActiveAlertsPanel from '../components/ActiveAlertsPanel'
import HomeSummary from '../components/HomeSummary'
import { 
  useHosts, 
  useAllHostStatuses, 
  useAllHostMetrics, 
  useAlertStatus, 
  useUptimes, 
  useMaintenanceHosts 
} from '../hooks/useMonitoring'
import { setMaintenance } from '../api/alert'
import PageToolbar from '../components/PageToolbar'
import { RefreshCcw, Monitor } from 'lucide-react'

export default function OverviewPage() {
  const navigate = useNavigate()
  
  const { data: hosts = [], isLoading: hostsLoading } = useHosts()
  const { data: hostStatuses = {} } = useAllHostStatuses(hosts)
  const { data: hostMetrics = {} } = useAllHostMetrics()
  const { data: activeAlerts = [], refetch: refetchAlerts, isError: alertsError } = useAlertStatus()
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
      <PageToolbar icon={Monitor} title="지금 상태" description="처리할 문제와 서버·네트워크·웹사이트·DB 상태를 한눈에 확인합니다" />

      <HomeSummary
        hosts={hosts}
        hostStatuses={hostStatuses}
        activeAlerts={activeAlerts}
        maintenanceHosts={maintenanceHosts}
      />

      <ActiveAlertsPanel
        alerts={activeAlerts}
        onChanged={refetchAlerts}
        error={alertsError}
        onSelectHost={(host) => navigate(`/detail?host=${host}`)}
      />

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
