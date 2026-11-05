import { useState } from 'react'
import { CheckCircle2, AlertTriangle, Check } from 'lucide-react'
import { ackAlert, type ActiveAlert } from '../api/alert'
import { TONES } from '../utils/severity'
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
  onSelectHost?: (host: string) => void
  host?: string // 지정 시 해당 호스트만 (상세 화면)
}

export default function ActiveAlertsPanel({ alerts, onChanged, onSelectHost, host }: Props) {
  const [pending, setPending] = useState<string | null>(null)

  const items = alerts
    .filter(a => !a.in_maintenance && (!host || a.host === host))
    .sort((a, b) =>
      Number(a.acked) - Number(b.acked) ||
      Number(b.severity === 'critical') - Number(a.severity === 'critical'))
  const open = items.filter(a => !a.acked).length

  const ack = async (a: ActiveAlert) => {
    const key = `${a.host}|${a.category}|${a.severity}`
    setPending(key)
    try {
      await ackAlert(a.host, a.category, a.severity)
      onChanged()
    } finally {
      setPending(null)
    }
  }

  if (items.length === 0) {
    return (
      <div className={cn('rounded-xl border px-4 py-3 flex items-center gap-2 text-sm', TONES.emerald.bg, TONES.emerald.border, TONES.emerald.text)}>
        <CheckCircle2 size={16} className="shrink-0" />
        {host ? '이 서버에 처리할 문제가 없습니다' : '처리할 문제가 없습니다 — 모든 서버가 정상 범위입니다'}
      </div>
    )
  }

  return (
    <section className="rounded-xl border border-slate-800 bg-slate-900/50" aria-label="처리할 문제">
      <header className="px-4 py-3 border-b border-slate-800 flex items-center gap-2">
        <AlertTriangle size={16} className={open > 0 ? TONES.rose.text : 'text-slate-500'} />
        <h2 className="text-sm font-semibold text-slate-200">
          처리할 문제 <span className={open > 0 ? TONES.rose.text : 'text-slate-500'}>{open}건</span>
        </h2>
        {items.length > open && <span className="text-xs text-slate-500">· 확인함 {items.length - open}건</span>}
      </header>
      <ul className="divide-y divide-slate-800">
        {items.map(a => {
          const key = `${a.host}|${a.category}|${a.severity}|${a.message}`
          const tone = TONES[a.severity === 'critical' ? 'rose' : 'amber']
          return (
            <li key={key} className={cn('px-4 py-3 flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-4', a.acked && 'opacity-60')}>
              <div className="flex items-center gap-2 min-w-0 flex-1">
                <span className={cn('shrink-0 rounded-md px-2 py-0.5 text-xs font-semibold border', tone.bg, tone.border, tone.text)}>
                  {a.severity === 'critical' ? '위험' : '주의'}
                </span>
                {!host && (
                  <button
                    type="button"
                    onClick={() => onSelectHost?.(a.host)}
                    className="shrink-0 text-sm font-semibold text-slate-100 hover:text-indigo-300 hover:underline"
                  >
                    {a.host}
                  </button>
                )}
                <span className="text-sm text-slate-300 truncate">
                  <span className="text-slate-500">{CATEGORY_LABEL[a.category] ?? a.category} · </span>
                  {a.message}
                </span>
              </div>
              {a.acked ? (
                <span className="text-xs text-slate-500 flex items-center gap-1 sm:shrink-0">
                  <Check size={14} /> 확인함 — 복구될 때까지 메일 중지
                </span>
              ) : (
                <button
                  type="button"
                  onClick={() => ack(a)}
                  disabled={pending !== null}
                  className="sm:shrink-0 self-start sm:self-auto px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-semibold border border-slate-700 disabled:opacity-50"
                  title="확인 처리하면 복구될 때까지 같은 알림 메일을 보내지 않습니다"
                >
                  {pending === `${a.host}|${a.category}|${a.severity}` ? '처리 중…' : '확인'}
                </button>
              )}
            </li>
          )
        })}
      </ul>
    </section>
  )
}
