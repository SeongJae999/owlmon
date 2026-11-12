import { useState } from 'react'
import { Check } from 'lucide-react'
import { ackAlert, type ActiveAlert } from '../api/alert'
import { errorText } from '../api/users'
import { useClock } from '../hooks/useClock'
import { TONES } from '../utils/severity'
import { ago, lasting } from '../utils/time'
import { cn } from '../utils/cn'

const CATEGORY_LABEL: Record<string, string> = {
  cpu: 'CPU',
  memory: '메모리',
  disk: '디스크',
  down: '연결 끊김',
}

interface Props {
  alerts: ActiveAlert[]
  onChanged: () => void
  host: string
  error?: boolean
  loading?: boolean // 첫 응답 전 — 빈 목록을 "문제 없음"으로 단정하지 않게
}

export default function ActiveAlertsPanel({ alerts, onChanged, host, error, loading }: Props) {
  const now = useClock(60_000)
  const [pending, setPending] = useState<string | null>(null)
  const [failMsg, setFailMsg] = useState<string | null>(null)

  const items = alerts
    .filter(a => !a.in_maintenance && a.host === host)
    .sort((a, b) =>
      Number(a.acked) - Number(b.acked) ||
      Number(b.severity === 'critical') - Number(a.severity === 'critical'))
  const open = items.filter(a => !a.acked).length

  const ack = async (a: ActiveAlert) => {
    const key = `${a.category}|${a.severity}`
    setPending(key)
    setFailMsg(null)
    try {
      await ackAlert(a.host, a.category, a.severity)
    } catch (e) {
      setFailMsg(errorText(e, '확인 처리에 실패했습니다'))
    } finally {
      onChanged()
      setPending(null)
    }
  }

  // 조회 실패를 "문제 없음"으로 보여주면 장애를 정상으로 오인함
  if (error) {
    return (
      <p role="alert" className="rounded-xl border border-base-800 bg-base-900/50 px-4 py-3 text-sm text-base-300">
        <span className="mr-2 inline-block h-2 w-2 rounded-full bg-amber-400 align-middle" />
        알림 상태를 불러오지 못했습니다. 서버 연결을 확인하세요
      </p>
    )
  }

  if (loading) {
    return (
      <p className="rounded-xl border border-base-800 bg-base-900/50 px-4 py-3 text-sm text-base-500">알림 상태 확인 중…</p>
    )
  }

  if (items.length === 0) {
    return (
      <p className="rounded-xl border border-base-800 bg-base-900/50 px-4 py-3 text-sm text-base-500">
        이 서버에 처리할 문제가 없습니다
      </p>
    )
  }

  return (
    <section className="rounded-xl border border-base-800 bg-base-900/50" aria-label="처리할 문제">
      <h2 className="px-4 py-3 border-b border-base-800 text-sm font-semibold text-base-200 flex items-center gap-2">
        처리할 문제 <span className="tabular-nums">{open}건</span>
        {items.length > open && <span className="ml-auto font-normal text-xs text-base-500 tabular-nums">처리 중 {items.length - open}건</span>}
      </h2>
      {failMsg && <p role="alert" className="px-4 pt-3 text-sm text-base-300">{failMsg}</p>}
      <ul className="divide-y divide-base-800">
        {items.map(a => {
          const tone = TONES[a.acked ? 'slate' : a.severity === 'critical' ? 'rose' : 'amber']
          return (
            <li key={`${a.category}|${a.severity}|${a.message}`} className={cn('px-4 py-3 flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-4', a.acked && 'opacity-60')}>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2 min-w-0">
                  <span className={cn('shrink-0 rounded-md px-1.5 py-0.5 text-xs font-bold border', tone.bg, tone.border, tone.text)}>
                    {a.severity === 'critical' ? '위험' : '주의'}
                  </span>
                  <span className="text-sm text-base-200 truncate">
                    <span className="text-base-500">{CATEGORY_LABEL[a.category] ?? a.category} · </span>
                    {a.message}
                    {a.since && <span className="text-base-400 tabular-nums"> · {lasting(a.since, now)}</span>}
                  </span>
                </div>
                {a.acked && (
                  <p className="mt-1 text-sm text-base-400" title="복구될 때까지 같은 알림 메일을 보내지 않습니다">
                    확인함{a.acked_by && ` · ${a.acked_by}`}{a.acked_at && ` · ${ago(a.acked_at, now)}`}
                  </p>
                )}
              </div>
              {!a.acked && (
                <button
                  type="button"
                  onClick={() => ack(a)}
                  disabled={pending !== null}
                  className="sm:shrink-0 self-start sm:self-auto inline-flex items-center gap-1 h-8 px-3 rounded-lg border border-base-700 bg-base-800/60 text-xs font-semibold text-base-200 hover:bg-base-700 disabled:opacity-50"
                  title="복구될 때까지 같은 알림 메일을 보내지 않습니다"
                >
                  <Check size={13} /> {pending === `${a.category}|${a.severity}` ? '처리 중…' : '확인'}
                </button>
              )}
            </li>
          )
        })}
      </ul>
    </section>
  )
}
