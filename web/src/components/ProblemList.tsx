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
const badge = (tone: Tone) => cn('shrink-0 rounded-md border px-1.5 py-0.5 text-xs font-bold tabular-nums', TONES[tone].bg, TONES[tone].text, TONES[tone].border)

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
  className?: string
}

export default function ProblemList({ open, fresh, handling, crit, warn, impactOf, rowAckKeys, failed, stale, pending, now, canAck, className }: Props) {
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

  return (
    <section className={cn('rounded-xl border border-base-800 bg-base-900/50 flex flex-col overflow-hidden', className)} aria-label="처리할 문제">
      <h2 className="px-4 py-3 border-b border-base-800 text-sm font-semibold text-base-200 flex items-center gap-2">
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
      ) : (
        <ul className="min-h-0 overflow-y-auto divide-y divide-base-800">
          {open.map(x => {
            const impact = impactOf(x.name)
            const keys = canAck && !x.acked ? rowAckKeys(x) : []
            return (
              <li key={x.key} className={cn('flex items-start gap-3 px-4 py-3', x.acked && 'opacity-60')}>
                <Link to={x.to} className="min-w-0 flex-1 rounded-sm hover:bg-base-800/40 -mx-1 px-1">
                  <div className="flex items-center gap-2">
                    <span className={badge(x.acked ? 'slate' : TONE[x.state])}>{LABEL[x.state]}</span>
                    <span className="font-semibold truncate text-base-100">{x.name}</span>
                    <span className="ml-auto shrink-0 text-xs text-base-500">{x.group}</span>
                  </div>
                  {x.purpose && <p className="mt-1 text-sm text-base-400 truncate">{x.purpose}</p>}
                  <p className="mt-0.5 text-sm text-base-200">
                    {x.detail}{x.since && <span className="text-base-400 tabular-nums"> · {lasting(x.since, now)}</span>}
                  </p>
                  {x.acked && (
                    <p className="mt-1 text-sm text-base-400" title="복구될 때까지 같은 알림 메일을 보내지 않습니다">
                      확인함{x.acked.by && ` · ${x.acked.by}`}{x.acked.at && ` · ${ago(x.acked.at, now)}`}
                    </p>
                  )}
                  {impact.length > 0 && (
                    <p className="mt-1 text-sm text-base-400 truncate">하위 {impact.length}대 영향: {impact.join(', ')}</p>
                  )}
                  {error?.key === x.key && <p role="alert" className="mt-1 text-sm text-base-300">{error.msg}</p>}
                </Link>
                {keys.length > 0 && (
                  <button
                    type="button"
                    onClick={() => ack(x)}
                    disabled={busy !== null}
                    title={impact.length > 0 ? '이 장비와 함께 끊긴 하위 장비의 알림 메일을 복구될 때까지 보내지 않습니다' : '복구될 때까지 같은 알림 메일을 보내지 않습니다'}
                    className="shrink-0 inline-flex items-center gap-1 h-8 px-3 rounded-lg border border-base-700 bg-base-800/60 text-xs font-semibold text-base-200 hover:bg-base-700 disabled:opacity-50"
                  >
                    <Check size={13} /> {busy === x.key ? '처리 중…' : '확인'}
                  </button>
                )}
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
