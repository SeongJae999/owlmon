import { useEffect, useRef, useState } from 'react'
import { Link, useLocation, useNavigationType, useSearchParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronRight, Monitor } from 'lucide-react'
import Loading from '../components/Loading'
import DeviceTable from '../components/DeviceTable'
import ProblemList from '../components/ProblemList'
import GroupCards from '../components/GroupCards'
import StatusCounts from '../components/StatusCounts'
import PageToolbar from '../components/PageToolbar'
import GroupTree from '../components/GroupTree'
import DataAge from '../components/DataAge'
import { useGroupScope } from '../hooks/useGroupScope'
import { UNGROUPED } from '../api/groups'
import { useFleet, KINDS, type Kind } from '../hooks/useFleet'
import { useUptimes } from '../hooks/useMonitoring'
import { useClock } from '../hooks/useClock'
import { setMaintenance } from '../api/alert'
import { getMe } from '../api/auth'
import { ago } from '../utils/time'
import { cn } from '../utils/cn'

// 상황판과 같은 판정(useFleet)만 쓴다 — 화면마다 따로 세면 같은 장애가 다르게 보임
export default function OverviewPage() {
  // 종류 필터를 주소에 둠 — 상황판의 종류 카드(예: /?kind=서버)가 바로 그 종류 표로 열리게
  const [params, setParams] = useSearchParams()
  // 예전 주소(?group=)로 저장된 북마크도 열리게 읽고 ?kind=로 바꿔 둠
  const legacy = params.get('group')
  // 종류만 바꾸고 그룹(?g=)은 그대로 둠 — 그룹 안에서 종류를 거르는 흐름
  const withKind = (k: string | null) => {
    const next = new URLSearchParams(params)
    next.delete('group')
    next.delete('p')
    if (k) next.set('kind', k)
    else next.delete('kind')
    return next
  }
  useEffect(() => {
    if (legacy !== null) setParams(withKind(legacy), { replace: true })
  }, [legacy]) // eslint-disable-line react-hooks/exhaustive-deps
  const kind = params.get('kind') ?? legacy
  const filter = KINDS.find(g => g.name === kind)?.name ?? null
  // 화면 안에서 카드를 눌러 거른 이동은 스크롤하지 않음 — 메뉴·링크로 온 이동만 장비 표로 내림
  const self = useRef(false)
  const setFilter = (g: Kind | null) => { self.current = true; setParams(withKind(g), { replace: true }) }
  const qc = useQueryClient()
  // 수집 지연 표시가 재조회 주기(15초) 사이에도 흐르게
  const now = useClock(15_000)
  const fleet = useFleet()
  // 그룹을 고르면 숫자·처리할 문제·장비 표가 모두 그 그룹(하위 포함)만 — 화면마다 따로 거르면 숫자가 어긋남
  const scope = useGroupScope(fleet.tiles, fleet.open, fleet.topoLookup)
  // 화면 안에서 그룹을 고른 이동도 장비 표로 내리지 않음
  const selectGroup = (g: string) => { self.current = true; scope.select(g) }
  const { data: uptimes = {} } = useUptimes()
  const { data: me } = useQuery({ queryKey: ['me'], queryFn: getMe, staleTime: 5 * 60_000 })
  const stale = fleet.stale(now)

  // 메뉴·링크로 종류를 골라 들어오면(같은 메뉴를 다시 눌러도) 장비 표까지 내려 줌.
  // 이동마다 바뀌는 location.key로 판단, 표가 그려진 뒤(서버 목록 로딩 끝)에야 실행
  const { key, hash } = useLocation()
  // 주소만 바꾸는 이동(장비 표 검색어·페이지 입력)은 replace — 그때마다 내리면 입력 중에 화면이 튐
  const navType = useNavigationType()
  // 바깥에서 온 이동(뒤로 가기·장비 찾기 '모두 보기')이면 장비 표를 다시 마운트해 주소의 검색어를 다시 읽게 — 표 안 입력(replace)으로는 다시 마운트하지 않음
  const [navKey, setNavKey] = useState(key)
  if (navType !== 'REPLACE' && navKey !== key) setNavKey(key)
  const ready = !fleet.loading && fleet.tiles.length > 0
  useEffect(() => {
    if (self.current) { self.current = false; return }
    if ((!kind && hash !== '#hosts') || !ready || navType === 'REPLACE') return
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

  const admin = me?.role === 'admin'
  // 그룹이 없으면 조회 전용 사용자에겐 빈 트리를 보이지 않음 — 만들 수 있는 관리자에게만 안내
  const showTree = scope.idx.byId.size > 0 || admin

  return (
    <div className={cn(showTree && 'lg:grid lg:grid-cols-[15rem_minmax(0,1fr)] lg:gap-4 lg:items-start')}>
      {showTree && (
        <GroupTree idx={scope.idx} counts={scope.counts} total={{ total: fleet.tiles.length, crit: fleet.crit, warn: fleet.warn }}
          selected={scope.selected} onSelect={selectGroup} canEdit={admin}
          className="max-lg:hidden lg:sticky lg:top-4 lg:max-h-[calc(100dvh-7rem)]" />
      )}
    <div className="space-y-6 min-w-0">
      <PageToolbar icon={Monitor} title={scope.selected ? scope.label : '지금 상태'}
        description={scope.selected ? '선택한 그룹(하위 그룹 포함)의 처리할 문제와 장비 상태' : '처리할 문제와 서버·네트워크·웹사이트·DB 상태를 한눈에 확인합니다'}>
        {showTree && scope.idx.byId.size > 0 && (
          <select aria-label="그룹" value={scope.selected} onChange={e => selectGroup(e.target.value)}
            className="lg:hidden h-9 max-w-[12rem] rounded-lg border border-base-700 bg-base-800 px-2 text-sm text-base-100">
            <option value="">전체</option>
            {scope.options.map(o => <option key={o.id} value={o.id}>{o.label}</option>)}
            <option value={UNGROUPED}>미지정</option>
          </select>
        )}
        <DataAge tiles={scope.tiles} now={now.getTime()} className="max-sm:hidden" />
        <StatusCounts crit={scope.crit} warn={scope.warn} handling={scope.handling} loading={fleet.pending} unknown={stale} />
      </PageToolbar>

      {scope.unresolved && (
        <p role="status" className="text-sm text-base-300">
          <span className="mr-2 inline-block h-2 w-2 rounded-full bg-amber-400 align-middle" />
          {scope.unresolved === 'loading' ? '그룹 정보를 불러오는 중 — 잠시 전체를 표시합니다' : '그룹 정보를 불러오지 못했거나 없는 그룹입니다 — 전체를 표시합니다'}
        </p>
      )}

      {stale && (
        <p role="status" className="text-sm text-base-300">
          <span className="mr-2 inline-block h-2 w-2 rounded-full bg-amber-400 align-middle" />
          {fleet.failed.length > 0 ? `데이터 갱신 중단 — ${fleet.failed.join('·')} 조회 실패` : '데이터 갱신 지연'}
          {fleet.oldestUpdate > 0 && <span className="text-base-500"> (마지막 갱신 {ago(new Date(fleet.oldestUpdate).toISOString(), now)})</span>}
        </p>
      )}

      {/* 장애가 몰리면 처리할 문제가 화면의 대부분을 쓰고 장비 표는 아래로 밀림 — 원인 장비가 작은 창 밖에 숨지 않게. 1280px 이상(노트북 포함)은 종류 카드를 옆에 세로로 */}
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1fr)_20rem] xl:items-start">
        <ProblemList
          open={scope.open}
          impactOf={fleet.impactOf} rowAckKeys={fleet.rowAckKeys} failed={fleet.failed} stale={stale} pending={fleet.pending} now={now}
          canAck={!!me && me.role !== 'viewer'} id="problems" className="min-w-0 max-h-[60vh]"
        />
        <GroupCards tiles={scope.tiles} kindFailed={fleet.kindFailed} kindPending={fleet.kindPending} selected={filter} onSelect={setFilter}
          now={now.getTime()} className="xl:grid-cols-1" />
      </div>

      <section className="space-y-3" aria-label="전체 장비">
        {/* 종류별 화면은 메뉴에 없음 — 등록·포트 상세 같은 그 종류만의 화면은 여기서 들어감 (장비가 0대여도) */}
        {filter && filter !== '서버' && (
          <Link to={KINDS.find(g => g.name === filter)!.to} className="inline-flex items-center gap-1 text-sm text-brand-300 hover:underline">
            {filter} 화면 열기 (등록·상세) <ChevronRight size={14} />
          </Link>
        )}
        <DeviceTable
          key={`${scope.selected}|${navKey}`} inGroup={!!scope.selected}
          tiles={scope.tiles} filter={filter} kindFailed={fleet.kindFailed} kindPending={fleet.kindPending} uptimes={uptimes} now={now}
          canMaint={me?.role === 'admin'} onToggleMaintenance={onToggleMaintenance}
        />
      </section>
    </div>
    </div>
  )
}
