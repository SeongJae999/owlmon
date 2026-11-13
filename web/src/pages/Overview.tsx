import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Monitor } from 'lucide-react'
import Loading from '../components/Loading'
import DeviceTable from '../components/DeviceTable'
import ProblemList from '../components/ProblemList'
import GroupCards from '../components/GroupCards'
import StatusCounts from '../components/StatusCounts'
import PageToolbar from '../components/PageToolbar'
import { useFleet, type Group } from '../hooks/useFleet'
import { useUptimes } from '../hooks/useMonitoring'
import { useClock } from '../hooks/useClock'
import { setMaintenance } from '../api/alert'
import { getMe } from '../api/auth'
import { ago } from '../utils/time'

// 상황판과 같은 판정(useFleet)만 쓴다 — 화면마다 따로 세면 같은 장애가 다르게 보임
export default function OverviewPage() {
  const [filter, setFilter] = useState<Group | null>(null)
  const qc = useQueryClient()
  const now = useClock(60_000)
  const fleet = useFleet()
  const { data: uptimes = {} } = useUptimes()
  const { data: me } = useQuery({ queryKey: ['me'], queryFn: getMe, staleTime: 5 * 60_000 })
  const stale = fleet.stale(now)

  const onToggleMaintenance = async (host: string, enabled: boolean) => {
    try {
      await setMaintenance(host, enabled)
    } finally {
      qc.invalidateQueries({ queryKey: ['maintenanceHosts'] })
      qc.invalidateQueries({ queryKey: ['alertStatus'] })
    }
  }

  if (fleet.loading) {
    return <Loading label="장비 정보를 불러오는 중…" rows={6} />
  }

  return (
    <div className="space-y-6">
      <PageToolbar icon={Monitor} title="지금 상태" description="처리할 문제와 서버·네트워크·웹사이트·DB 상태를 한눈에 확인합니다">
        <StatusCounts crit={fleet.crit} warn={fleet.warn} handling={fleet.handling} loading={fleet.pending} unknown={stale} />
      </PageToolbar>

      {stale && (
        <p role="status" className="text-sm text-base-300">
          <span className="mr-2 inline-block h-2 w-2 rounded-full bg-amber-400 align-middle" />
          {fleet.failed.length > 0 ? `데이터 갱신 중단 — ${fleet.failed.join('·')} 조회 실패` : '데이터 갱신 지연'}
          {fleet.oldestUpdate > 0 && <span className="text-base-500"> (마지막 갱신 {ago(new Date(fleet.oldestUpdate).toISOString(), now)})</span>}
        </p>
      )}

      <ProblemList
        open={fleet.open} fresh={fleet.fresh} handling={fleet.handling} crit={fleet.crit} warn={fleet.warn}
        impactOf={fleet.impactOf} rowAckKeys={fleet.rowAckKeys} failed={fleet.failed} stale={stale} pending={fleet.pending} now={now}
        canAck={!!me && me.role !== 'viewer'}
      />

      <section className="space-y-3" aria-label="전체 장비">
        <GroupCards tiles={fleet.tiles} groupFailed={fleet.groupFailed} groupPending={fleet.groupPending} selected={filter} onSelect={setFilter} />
        <DeviceTable
          tiles={fleet.tiles} filter={filter} groupFailed={fleet.groupFailed} groupPending={fleet.groupPending} uptimes={uptimes} now={now}
          canMaint={me?.role === 'admin'} onToggleMaintenance={onToggleMaintenance}
        />
      </section>
    </div>
  )
}
