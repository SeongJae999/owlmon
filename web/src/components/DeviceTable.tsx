import { useNavigate, Link } from 'react-router-dom'
import { AlertTriangle, Monitor, Wrench } from 'lucide-react'
import { GROUPS, LABEL, type Group, type State, type Tile } from '../hooks/useFleet'
import { lasting } from '../utils/time'
import { cn } from '../utils/cn'

interface Props {
  tiles: Tile[]
  filter: Group | null
  groupFailed: (g: Group) => boolean
  groupPending: (g: Group) => boolean
  uptimes: Record<string, number>
  now: Date
  canMaint: boolean
  onToggleMaintenance: (host: string, enabled: boolean) => void
}

const DOT: Record<State, string> = { crit: 'bg-rose-500', warn: 'bg-amber-400', maint: 'bg-purple-400', ok: 'bg-emerald-400', wait: 'bg-base-500' }

// 정상 막대는 중립색 — 수십 행 모두에 색을 칠하면 기준을 넘은 칸이 묻힘
function Meter({ label, value, warn, crit }: { label: string; value: number | null; warn: number; crit: number }) {
  const level = value === null ? null : value >= crit ? 'crit' : value >= warn ? 'warn' : 'ok'
  return (
    <span className="inline-flex items-center gap-1.5 w-[104px]">
      <span className="w-9 shrink-0 text-xs text-base-500">{label}</span>
      <span className="h-1.5 w-8 shrink-0 rounded-full bg-base-800 overflow-hidden">
        {value !== null && (
          <span
            className={cn('block h-full', level === 'crit' ? 'bg-rose-500' : level === 'warn' ? 'bg-amber-400' : 'bg-base-500')}
            style={{ width: `${Math.min(100, value)}%` }}
          />
        )}
      </span>
      <span className={cn('w-8 text-right text-xs tabular-nums',
        level === 'crit' ? 'text-rose-300 font-semibold' : level === 'warn' ? 'text-amber-300 font-semibold' : 'text-base-300')}>
        {value === null ? '—' : `${Math.round(value)}%`}
      </span>
    </span>
  )
}

export default function DeviceTable({ tiles, filter, groupFailed, groupPending, uptimes, now, canMaint, onToggleMaintenance }: Props) {
  const navigate = useNavigate()
  const order = GROUPS.map(g => g.name)
  // 문제 순 정렬은 위 "처리할 문제"가 맡음 — 여기선 종류·이름순으로 고정해 30초마다 행 위치가 바뀌지 않게
  const rows = tiles
    .filter(x => !filter || x.group === filter)
    .sort((a, b) => order.indexOf(a.group) - order.indexOf(b.group) || a.name.localeCompare(b.name))
  const anyFailed = GROUPS.some(g => groupFailed(g.name))

  const empty = (msg: string, hint: string, failed = false) => (
    <div role={failed ? 'alert' : undefined} className={cn('rounded-xl border bg-base-900/50 p-10 flex flex-col items-center text-center gap-2', failed ? 'border-amber-500/30' : 'border-base-800')}>
      {failed ? <AlertTriangle size={36} className="text-amber-400" /> : <Monitor size={36} className="text-base-700" />}
      <p className="text-sm font-semibold text-base-300">{msg}</p>
      <p className="text-xs text-base-500">{hint}</p>
    </div>
  )
  if (rows.length === 0 && filter) {
    return groupFailed(filter) ? empty(`${filter} 목록을 불러오지 못했습니다`, '서버 또는 수집기 연결을 확인하세요. 30초마다 다시 시도합니다', true)
      : groupPending(filter) ? empty(`${filter} 상태를 확인하는 중…`, '첫 응답을 기다리고 있습니다')
      : filter === '서버' ? empty('연결된 서버가 없습니다', '에이전트를 설치하면 자동으로 표시됩니다 (첫 수집 후 30초 이내)')
      : empty(`등록된 ${filter}가 없습니다`, '해당 메뉴에서 추가하면 표시됩니다')
  }
  if (tiles.length === 0 && !anyFailed && GROUPS.some(g => groupPending(g.name))) {
    return empty('장비 상태를 확인하는 중…', '첫 응답을 기다리고 있습니다')
  }
  if (tiles.length === 0) {
    return anyFailed ? (
      <div role="alert" className="rounded-xl border border-amber-500/30 bg-base-900/50 p-10 flex flex-col items-center text-center gap-2">
        <AlertTriangle size={36} className="text-amber-400" />
        <p className="text-sm font-semibold text-base-300">장비 목록을 불러오지 못했습니다</p>
        <p className="text-xs text-base-500">서버 또는 수집기(Prometheus) 연결을 확인하세요. 30초마다 다시 시도합니다</p>
      </div>
    ) : (
      <div className="rounded-xl border border-base-800 bg-base-900/50 p-10 flex flex-col items-center text-center gap-2">
        <Monitor size={36} className="text-base-700" />
        <p className="text-sm font-semibold text-base-300">등록된 장비가 없습니다</p>
        <p className="text-xs text-base-500">서버는 에이전트를 설치하면, 네트워크·웹사이트·DB는 각 메뉴에서 추가하면 표시됩니다</p>
      </div>
    )
  }

  return (
    <div id="hosts" className="scroll-mt-20 rounded-xl border border-base-800 bg-base-900/50 overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-base-800 text-left text-xs text-base-500">
            <th className="px-4 py-2 font-medium w-32">상태</th>
            <th className="px-3 py-2 font-medium">이름</th>
            <th className="px-3 py-2 font-medium w-40">종류</th>
            <th className="px-3 py-2 font-medium">지표</th>
            <th className="px-3 py-2 font-medium w-28">지속</th>
            <th className="px-3 py-2 font-medium w-14"><span className="sr-only">점검</span></th>
          </tr>
        </thead>
        <tbody className="divide-y divide-base-800/70">
          {rows.map(x => {
            const bad = x.state === 'crit' || x.state === 'warn'
            const problem = bad && !x.acked && !x.causedBy
            // 최근 조회가 실패하면 지난 "정상"을 그대로 믿지 않음 (위험·주의는 지난 값이라도 계속 보여 줌)
            const unknown = groupFailed(x.group) && x.state === 'ok'
            const label = x.down && x.state !== 'maint' ? (x.causedBy ? '상위 영향' : '연결 끊김')
              : unknown ? '확인 불가' : bad && x.acked ? `${LABEL[x.state]} · 확인함` : LABEL[x.state]
            const server = x.group === '서버'
            const uptime = server ? uptimes[x.name] : undefined
            return (
              <tr key={x.key} onClick={() => navigate(x.to)} className="h-11 cursor-pointer hover:bg-base-800/40">
                <td className="px-4 whitespace-nowrap">
                  <span className={cn('mr-2 inline-block h-2 w-2 rounded-full align-middle',
                    unknown || (bad && x.acked) || x.causedBy ? 'bg-base-500' : DOT[x.state])} />
                  <span className={problem ? 'font-semibold text-base-100' : 'text-base-400'}>{label}</span>
                </td>
                <td className="px-3 max-w-0 w-[28%]">
                  <Link to={x.to} onClick={e => e.stopPropagation()} className={cn('block truncate hover:underline', problem ? 'font-semibold text-base-50' : 'text-base-200')}>
                    {x.name}
                    {x.purpose && <span className="ml-2 font-normal text-base-500">{x.purpose}</span>}
                  </Link>
                </td>
                <td className="px-3 max-w-0 truncate text-base-400" title={x.sub}>
                  {x.group}{x.sub && <span className="ml-1.5 text-xs text-base-500">{x.sub}</span>}
                </td>
                <td className="px-3 whitespace-nowrap">
                  {server && x.bars && !x.down ? (
                    <span className="inline-flex items-center gap-3">
                      {x.bars.map(b => <Meter key={b.label} label={b.label} value={b.value} warn={b.warn} crit={b.crit} />)}
                      {uptime !== undefined && <span className="text-xs text-base-500 tabular-nums">가동 {uptime.toFixed(1)}%</span>}
                    </span>
                  ) : (
                    <span className={problem ? 'text-base-100' : 'text-base-400'}>{x.detail}</span>
                  )}
                </td>
                <td className="px-3 whitespace-nowrap text-base-400 tabular-nums">{x.since && bad ? lasting(x.since, now) : ''}</td>
                <td className="px-3 text-right">
                  {server && canMaint && (
                    <button
                      type="button"
                      onClick={e => { e.stopPropagation(); onToggleMaintenance(x.name, x.state !== 'maint') }}
                      className={cn('p-1.5 rounded-md border transition-colors',
                        x.state === 'maint' ? 'bg-purple-500 text-white border-transparent' : 'bg-base-800 text-base-400 border-base-700 hover:text-base-200 hover:bg-base-700')}
                      title={x.state === 'maint' ? '점검 모드 해제' : '점검 모드 설정'}
                      aria-label={x.state === 'maint' ? `${x.name} 점검 모드 해제` : `${x.name} 점검 모드 설정`}
                    >
                      <Wrench size={13} />
                    </button>
                  )}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
