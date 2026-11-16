import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { Check } from 'lucide-react'
import { ackAlerts } from '../api/alert'
import { errorText } from '../api/users'
import { DOT, LABEL, type AckKey, type Tile } from '../hooks/useFleet'
import { ago, lasting } from '../utils/time'
import { cn } from '../utils/cn'

// 상태색은 점에만 — 이 목록만 빨간 글자 배지를 쓰면 장비 표·구성도와 표현이 달라짐. 확인한 문제는 회색 점(누가 확인했는지는 내용 옆에)
function Grade({ x }: { x: Tile }) {
  return (
    <span className="inline-flex shrink-0 items-center gap-1.5 text-sm text-base-200">
      <span className={cn('h-2 w-2 rounded-full', x.acked ? 'bg-base-500' : DOT[x.state])} />{LABEL[x.state]}
    </span>
  )
}
// 아래 장비 표와 같은 열 리듬 — 등급 · 장비 · 종류 · 내용 · 지속 · 확인
// 화면 폭이 아니라 이 목록 칸의 폭 기준 — 장비 상태 격자와 나란히 놓여 반 폭이 되면 종류 열을 빼고 이름·내용에 자리를 줌
const ROW = '@lg:grid @lg:items-center @lg:gap-x-3 @lg:grid-cols-[3.5rem_minmax(0,1.2fr)_minmax(0,1.5fr)_5rem_4.5rem] @4xl:grid-cols-[3.5rem_minmax(0,1.1fr)_7.5rem_minmax(0,2fr)_6.5rem_5rem]'

interface Props {
  open: Tile[]
  impactOf: (name: string) => string[]
  rowAckKeys: (x: Tile) => AckKey[]
  failed: string[]
  stale: boolean
  pending?: boolean // 아직 첫 응답을 못 받은 출처가 있음
  now: Date
  // 상황판(벽 모니터, 조회 전용 계정)에선 끔 — 누르면 403만 받는다
  canAck: boolean
  compact?: boolean // 상황판 오른쪽 좁은 칸 — 표 대신 2줄 카드
  large?: boolean // 상황판(벽 모니터) — 몇 미터 떨어져서도 읽히게
  id?: string
  className?: string
}

export default function ProblemList({ open, impactOf, rowAckKeys, failed, stale, pending, now, canAck, compact, large, id, className }: Props) {
  const qc = useQueryClient()
  // 누른 행만 잠금 — 목록이 다시 받아져 그 행이 '확인함'으로 바뀐 뒤에 풂. 먼저 풀면 같은 행을 또 누르거나, 확인한 행이 내려가며 그 자리에 올라온 다른 장애를 보지 않고 확인하게 됨
  const [busy, setBusy] = useState<ReadonlySet<string>>(new Set())
  // 실패 안내도 행별 — 하나로 두면 다른 행을 누르는 순간 앞 행의 실패 안내가 지워져 처리된 줄 앎
  const [errors, setErrors] = useState<ReadonlyMap<string, string>>(new Map())
  const setError = (key: string, msg: string | null) => setErrors(prev => {
    const next = new Map(prev)
    if (msg) next.set(key, msg)
    else next.delete(key)
    return next
  })
  const setBusyKey = (key: string, on: boolean) => setBusy(prev => {
    const next = new Set(prev)
    if (on) next.add(key)
    else next.delete(key)
    return next
  })

  const ack = async (x: Tile) => {
    setBusyKey(x.key, true)
    setError(x.key, null)
    try {
      await ackAlerts(rowAckKeys(x))
    } catch (e) {
      setError(x.key, errorText(e, '확인 처리에 실패했습니다'))
    } finally {
      await Promise.all([
        qc.invalidateQueries({ queryKey: ['alertStatus'] }),
        qc.invalidateQueries({ queryKey: ['alertAcks'] }),
      ]).catch(() => {})
      setBusyKey(x.key, false)
    }
  }

  // 하위 영향·확인자 — 내용(detail)이 길어 잘려도 이 정보는 남도록 따로 둔다
  const ackText = (x: Tile) => x.acked && `${x.acked.by ? `${x.acked.by} 확인` : '확인함'}${x.acked.at ? ` ${ago(x.acked.at, now)}` : ''}`
  const notes = (x: Tile, names = false) => {
    const impact = impactOf(x.name)
    return [
      ackText(x),
      impact.length > 0 && (names ? `하위 ${impact.length}대: ${impact.join(', ')}` : `하위 ${impact.length}대 영향`),
    ].filter(Boolean).join(' · ')
  }

  return (
    <section id={id} className={cn('@container rounded-xl border border-base-800 bg-base-900/50 flex flex-col overflow-hidden scroll-mt-4', className)} aria-label="처리할 문제">
      {/* 건수는 머리글 숫자(위험·주의·확인)에만 — 같은 숫자를 여러 모양으로 보이면 어느 것이 기준인지 헷갈림 */}
      <h2 className={cn('px-4 py-2.5 border-b border-base-800 font-semibold text-base-200', large ? 'text-base' : 'text-sm')}>처리할 문제</h2>
      {open.length === 0 ? (
        // 조회 실패·갱신 중단을 "문제 없음"으로 보이면 장애를 정상으로 오인함
        stale ? (
          <p className="px-4 py-3 text-sm text-base-300">
            <span className="mr-2 inline-block h-2 w-2 rounded-full bg-amber-400 align-middle" />
            상태 확인 불가 — {failed.length > 0 ? `${failed.join('·')} 조회 실패` : '데이터 갱신 지연'}
          </p>
        ) : pending ? (
          <p className="px-4 py-3 text-sm text-base-500">불러오는 중…</p>
        ) : (
          <p className="px-4 py-3 text-sm text-base-500">처리할 문제가 없습니다</p>
        )
      ) : compact ? (
        <ul className="min-h-0 overflow-y-auto divide-y divide-base-800">
          {open.map(x => (
            <li key={x.key} className={cn('flex flex-wrap items-center', x.acked && 'opacity-55')}>
              <Link to={x.to} className={cn('block min-w-0 flex-1 px-4 hover:bg-base-800/40', large ? 'py-2.5' : 'py-2')}>
                <div className="flex items-center gap-2">
                  <Grade x={x} />
                  <span className={cn('min-w-0 truncate font-semibold text-base-100', large && 'text-lg')}>{x.name}</span>
                  {x.since && <span className={cn('ml-auto shrink-0 text-base-500 tabular-nums', large ? 'text-sm' : 'text-xs')}>{lasting(x.since, now)}</span>}
                </div>
                <p className={cn('mt-0.5 truncate text-base-300', large ? 'text-base' : 'text-sm')}>{x.detail}</p>
                {x.location && <p className={cn('truncate text-base-400', large ? 'text-sm' : 'text-xs')} title={x.location}>위치 {x.location}</p>}
                {notes(x, true) && <p className={cn('truncate text-base-500', large ? 'text-sm' : 'text-xs')} title={notes(x, true)}>{notes(x, true)}</p>}
              </Link>
              {/* 링크 안에 버튼을 넣을 수 없어 행 오른쪽에 따로 둠 */}
              {canAck && !x.acked && rowAckKeys(x).length > 0 && (
                <button type="button" onClick={() => ack(x)} disabled={busy.has(x.key)} aria-label={`${x.name} 확인`}
                  className="mr-3 inline-flex shrink-0 items-center gap-1 h-7 px-2.5 rounded-md border border-base-700 bg-base-800/60 text-xs font-semibold text-base-200 hover:bg-base-700 disabled:opacity-50">
                  <Check size={13} /> {busy.has(x.key) ? '보내는 중…' : '확인'}
                </button>
              )}
              {errors.has(x.key) && <p role="alert" className="basis-full px-4 pb-2 text-xs text-base-300">{errors.get(x.key)} — 다시 눌러 주세요</p>}
            </li>
          ))}
        </ul>
      ) : (
        <div role="table" aria-label="처리할 문제 목록" className="min-h-0 overflow-y-auto">
          <div role="row" className={cn(ROW, 'hidden @lg:grid px-4 py-1.5 border-b border-base-800 text-xs text-base-500')}>
            <span role="columnheader">등급</span>
            <span role="columnheader">장비</span>
            <span role="columnheader" className="hidden @4xl:block">종류</span>
            <span role="columnheader">내용</span>
            <span role="columnheader">지속</span>
            <span role="columnheader"><span className="sr-only">확인</span></span>
          </div>
          {open.map(x => {
            const keys = canAck && !x.acked ? rowAckKeys(x) : []
            const impact = impactOf(x.name)
            const ackNote = ackText(x)
            return (
              <div role="row" key={x.key} className={cn(ROW, 'flex flex-wrap items-center gap-x-2 gap-y-0.5 px-4 py-2 @lg:py-1.5 @lg:min-h-11 border-b border-base-800/70 last:border-b-0 hover:bg-base-800/30', x.acked && 'opacity-55')}>
                <span role="cell"><Grade x={x} /></span>
                <span role="cell" className="min-w-0">
                  <span className="block truncate">
                    <Link to={x.to} className="font-semibold text-base-100 hover:underline">{x.name}</Link>
                    {x.purpose && <span className="ml-1.5 text-xs text-base-500">{x.purpose}</span>}
                  </span>
                  {x.location && (x.rackPlaced
                    ? <Link to={`/racks?focus=${encodeURIComponent(x.name)}`} className="block truncate text-xs text-base-400 hover:text-base-200 hover:underline" title={`${x.location} — 랙 실장도에서 보기`}>{x.location}</Link>
                    : <span className="block truncate text-xs text-base-400" title={x.location}>{x.location}</span>)}
                </span>
                <span role="cell" className="hidden @4xl:block truncate text-sm text-base-400">{x.kind}</span>
                <span role="cell" className="min-w-0 basis-full @lg:basis-auto flex items-baseline gap-1.5 text-sm">
                  <span className="min-w-0 truncate text-base-200">{x.detail}</span>
                  {/* 원인 장비임을 한눈에 — 영향 대수는 굵게, 색은 중립 */}
                  {impact.length > 0 && <span className="shrink-0 font-semibold text-base-100 tabular-nums" title={`하위 ${impact.length}대: ${impact.join(', ')}`}>하위 {impact.length}대</span>}
                  {ackNote && <span className="shrink-0 max-w-[40%] truncate text-base-500">· {ackNote}</span>}
                </span>
                <span role="cell" className="text-sm text-base-400 tabular-nums">{x.since ? lasting(x.since, now) : ''}</span>
                <span role="cell" className="@lg:text-right">
                  {keys.length > 0 && (
                    <button
                      type="button"
                      onClick={() => ack(x)}
                      disabled={busy.has(x.key)}
                      title={impactOf(x.name).length > 0 ? '이 장비와 함께 끊긴 하위 장비의 알림 메일을 복구될 때까지 보내지 않습니다' : '복구될 때까지 같은 알림 메일을 보내지 않습니다'}
                      className="inline-flex items-center gap-1 h-7 px-2.5 rounded-md border border-base-700 bg-base-800/60 text-xs font-semibold text-base-200 hover:bg-base-700 disabled:opacity-50"
                    >
                      <Check size={13} /> {busy.has(x.key) ? '보내는 중…' : '확인'}
                    </button>
                  )}
                </span>
                {errors.has(x.key) && (
                  <p role="alert" className="basis-full @lg:col-span-full text-sm text-base-300 @lg:pb-1">{errors.get(x.key)} — 다시 눌러 주세요</p>
                )}
              </div>
            )
          })}
        </div>
      )}
    </section>
  )
}
