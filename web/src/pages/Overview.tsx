import { useEffect, useRef } from 'react'
import { Link, useLocation, useSearchParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronRight, Monitor } from 'lucide-react'
import Loading from '../components/Loading'
import DeviceTable from '../components/DeviceTable'
import ProblemList from '../components/ProblemList'
import GroupCards from '../components/GroupCards'
import StatusCounts from '../components/StatusCounts'
import PageToolbar from '../components/PageToolbar'
import { useFleet, GROUPS, type Group } from '../hooks/useFleet'
import { useUptimes } from '../hooks/useMonitoring'
import { useClock } from '../hooks/useClock'
import { setMaintenance } from '../api/alert'
import { getMe } from '../api/auth'
import { ago } from '../utils/time'

// 상황판과 같은 판정(useFleet)만 쓴다 — 화면마다 따로 세면 같은 장애가 다르게 보임
export default function OverviewPage() {
  // 종류 필터를 주소에 둠 — 상황판의 종류 카드(예: /?group=서버)가 바로 그 종류 표로 열리게
  const [params, setParams] = useSearchParams()
  const filter = GROUPS.find(g => g.name === params.get('group'))?.name ?? null
  // 화면 안에서 카드를 눌러 거른 이동은 스크롤하지 않음 — 메뉴·링크로 온 이동만 장비 표로 내림
  const self = useRef(false)
  const setFilter = (g: Group | null) => { self.current = true; setParams(g ? { group: g } : {}, { replace: true }) }
  const qc = useQueryClient()
  const now = useClock(60_000)
  const fleet = useFleet()
  const { data: uptimes = {} } = useUptimes()
  const { data: me } = useQuery({ queryKey: ['me'], queryFn: getMe, staleTime: 5 * 60_000 })
  const stale = fleet.stale(now)

  // 메뉴·링크로 종류를 골라 들어오면(같은 메뉴를 다시 눌러도) 장비 표까지 내려 줌.
  // 이동마다 바뀌는 location.key로 판단, 표가 그려진 뒤(서버 목록 로딩 끝)에야 실행
  const { key } = useLocation()
  const group = params.get('group')
  const ready = !fleet.loading && fleet.tiles.length > 0
  useEffect(() => {
    if (self.current) { self.current = false; return }
    if (!group || !ready) return
    // 본문이 부드러운 스크롤이라 이어지는 렌더에 묻혀 멈춤 — 바로 이동. 위쪽 처리할 문제가 늦게 채워지면 표가 밀려 한 번 더
    const go = () => document.getElementById('hosts')?.scrollIntoView({ block: 'start', behavior: 'instant' })
    go()
    const t = setTimeout(go, 600)
    return () => clearTimeout(t)
  }, [key, ready]) // eslint-disable-line react-hooks/exhaustive-deps

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
        {/* 종류별 화면은 메뉴에 없음 — 등록·포트 상세 같은 그 종류만의 화면은 여기서 들어감 (장비가 0대여도) */}
        {filter && filter !== '서버' && (
          <Link to={GROUPS.find(g => g.name === filter)!.to} className="inline-flex items-center gap-1 text-sm text-brand-300 hover:underline">
            {filter} 화면 열기 (등록·상세) <ChevronRight size={14} />
          </Link>
        )}
        <DeviceTable
          tiles={fleet.tiles} filter={filter} groupFailed={fleet.groupFailed} groupPending={fleet.groupPending} uptimes={uptimes} now={now}
          canMaint={me?.role === 'admin'} onToggleMaintenance={onToggleMaintenance}
        />
      </section>
    </div>
  )
}
