import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { Check } from 'lucide-react'
import { ackAlert } from '../api/alert'
import { errorText } from '../api/users'
import { LABEL, type AckKey, type State, type Tile } from '../hooks/useFleet'
import { TONES, type Tone } from '../utils/severity'
import { ago, lasting } from '../utils/time'
import { cn } from '../utils/cn'

const TONE: Record<State, Tone> = { crit: 'rose', warn: 'amber', maint: 'slate', ok: 'emerald', wait: 'slate' }
const badge = (tone: Tone) => cn('inline-block shrink-0 rounded-md border px-1.5 py-0.5 text-xs font-bold tabular-nums', TONES[tone].bg, TONES[tone].text, TONES[tone].border)
// 아래 장비 표와 같은 열 리듬 — 등급 · 장비 · 종류 · 내용 · 지속 · 확인
const ROW = 'md:grid md:grid-cols-[3.5rem_minmax(0,1.1fr)_7.5rem_minmax(0,2fr)_6.5rem_5rem] md:items-center md:gap-x-3'

interface Props {
  open: Tile[]
  fresh: Tile[]
  handling: number
  crit: number
  warn: number
  impactOf: (name: string) => string[]
  rowAckKeys: (x: Tile) => AckKey[]
  failed: string[]
  stale: boolean
  pending?: boolean // 아직 첫 응답을 못 받은 출처가 있음
  now: Date
  // 상황판(벽 모니터, 조회 전용 계정)에선 끔 — 누르면 403만 받는다
  canAck: boolean
  compact?: boolean // 상황판 오른쪽 좁은 칸 — 표 대신 2줄 카드
  className?: string
}

export default function ProblemList({ open, fresh, handling, crit, warn, impactOf, rowAckKeys, failed, stale, pending, now, canAck, compact, className }: Props) {
  const qc = useQueryClient()
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<{ key: string; msg: string } | null>(null)

  const ack = async (x: Tile) => {
    setBusy(x.key)
    setError(null)
    try {
      const keys = rowAckKeys(x)
      await Promise.all(keys.map(k => ackAlert(k.host, k.category, k.severity)))
    } catch (e) {
      setError({ key: x.key, msg: errorText(e, '확인 처리에 실패했습니다') })
    } finally {
      // 일부만 성공해도 화면이 실제 상태를 다시 받아 오도록
      qc.invalidateQueries({ queryKey: ['alertStatus'] })
      qc.invalidateQueries({ queryKey: ['alertAcks'] })
      setBusy(null)
    }
  }

  // 하위 영향·확인자 — 내용(detail)이 길어 잘려도 이 정보는 남도록 따로 둔다
  const notes = (x: Tile, names = false) => {
    const impact = impactOf(x.name)
    return [
      x.acked && `${x.acked.by ? `${x.acked.by} 확인` : '확인함'}${x.acked.at ? ` ${ago(x.acked.at, now)}` : ''}`,
      impact.length > 0 && (names ? `하위 ${impact.length}대: ${impact.join(', ')}` : `하위 ${impact.length}대 영향`),
    ].filter(Boolean).join(' · ')
  }

  return (
    <section className={cn('rounded-xl border border-base-800 bg-base-900/50 flex flex-col overflow-hidden', className)} aria-label="처리할 문제">
      <h2 className="px-4 py-2.5 border-b border-base-800 text-sm font-semibold text-base-200 flex items-center gap-2">
        처리할 문제
        <span className={badge(crit ? 'rose' : warn ? 'amber' : 'slate')}>{fresh.length}건</span>
        {handling > 0 && <span className="ml-auto font-normal text-xs text-base-500 tabular-nums">처리 중 {handling}건</span>}
      </h2>
      {open.length === 0 ? (
        // 조회 실패·갱신 중단을 "문제 없음"으로 보이면 장애를 정상으로 오인함
        stale ? (
          <p className="px-4 py-3 text-sm text-base-300">
            <span className="mr-2 inline-block h-2 w-2 rounded-full bg-amber-400 align-middle" />
            상태 확인 불가 — {failed.length > 0 ? `${failed.join('·')} 조회 실패` : '데이터 갱신 지연'}
          </p>
        ) : pending ? (
          <p className="px-4 py-3 text-sm text-base-500">상태 확인 중…</p>
        ) : (
          <p className="px-4 py-3 text-sm text-base-500">처리할 문제가 없습니다</p>
        )
      ) : compact ? (
        <ul className="min-h-0 overflow-y-auto divide-y divide-base-800">
          {open.map(x => (
            <li key={x.key} className={cn(x.acked && 'opacity-55')}>
              <Link to={x.to} className="block px-4 py-2 hover:bg-base-800/40">
                <div className="flex items-center gap-2">
                  <span className={badge(x.acked ? 'slate' : TONE[x.state])}>{x.acked ? '확인함' : LABEL[x.state]}</span>
                  <span className="min-w-0 truncate font-semibold text-base-100">{x.name}</span>
                  {x.since && <span className="ml-auto shrink-0 text-xs text-base-500 tabular-nums">{lasting(x.since, now)}</span>}
                </div>
                <p className="mt-0.5 truncate text-sm text-base-300">{x.detail}</p>
                {x.location && <p className="truncate text-xs text-base-400" title={x.location}>위치 {x.location}</p>}
                {notes(x, true) && <p className="truncate text-xs text-base-500" title={notes(x, true)}>{notes(x, true)}</p>}
              </Link>
            </li>
          ))}
        </ul>
      ) : (
        <div role="table" aria-label="처리할 문제 목록" className="min-h-0 overflow-y-auto">
          <div role="row" className={cn(ROW, 'hidden md:grid px-4 py-1.5 border-b border-base-800 text-xs text-base-500')}>
            <span role="columnheader">등급</span>
            <span role="columnheader">장비</span>
            <span role="columnheader">종류</span>
            <span role="columnheader">내용</span>
            <span role="columnheader">지속</span>
            <span role="columnheader"><span className="sr-only">확인</span></span>
          </div>
          {open.map(x => {
            const keys = canAck && !x.acked ? rowAckKeys(x) : []
            return (
              <div role="row" key={x.key} className={cn(ROW, 'flex flex-wrap items-center gap-x-2 gap-y-0.5 px-4 py-2 md:py-1.5 md:min-h-11 border-b border-base-800/70 last:border-b-0 hover:bg-base-800/30', x.acked && 'opacity-55')}>
                <span role="cell"><span className={badge(x.acked ? 'slate' : TONE[x.state])}>{LABEL[x.state]}</span></span>
                <span role="cell" className="min-w-0">
                  <span className="block truncate">
                    <Link to={x.to} className="font-semibold text-base-100 hover:underline">{x.name}</Link>
                    {x.purpose && <span className="ml-1.5 text-xs text-base-500">{x.purpose}</span>}
                  </span>
                  {x.location && <span className="block truncate text-xs text-base-400" title={x.location}>{x.location}</span>}
                </span>
                <span role="cell" className="truncate text-sm text-base-400">{x.group}</span>
                <span role="cell" className="min-w-0 basis-full md:basis-auto flex items-baseline gap-1.5 text-sm">
                  <span className="min-w-0 truncate text-base-200">{x.detail}</span>
                  {notes(x) && <span className="shrink-0 max-w-[60%] truncate text-base-500" title={notes(x, true)}>· {notes(x)}</span>}
                </span>
                <span role="cell" className="text-sm text-base-400 tabular-nums">{x.since ? lasting(x.since, now) : ''}</span>
                <span role="cell" className="md:text-right">
                  {keys.length > 0 && (
                    <button
                      type="button"
                      onClick={() => ack(x)}
                      disabled={busy !== null}
                      title={impactOf(x.name).length > 0 ? '이 장비와 함께 끊긴 하위 장비의 알림 메일을 복구될 때까지 보내지 않습니다' : '복구될 때까지 같은 알림 메일을 보내지 않습니다'}
                      className="inline-flex items-center gap-1 h-7 px-2.5 rounded-md border border-base-700 bg-base-800/60 text-xs font-semibold text-base-200 hover:bg-base-700 disabled:opacity-50"
                    >
                      <Check size={13} /> {busy === x.key ? '처리 중…' : '확인'}
                    </button>
                  )}
                </span>
                {error?.key === x.key && (
                  <p role="alert" className="basis-full md:col-span-full text-sm text-rose-300 md:pb-1">{error.msg} — 다시 눌러 주세요</p>
                )}
              </div>
            )
          })}
        </div>
      )}
    </section>
  )
}
