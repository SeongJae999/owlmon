import { useState } from 'react'
import { MessageSquare, Plus, Send, Siren, Trash2, Webhook as WebhookIcon } from 'lucide-react'
import { testChannel, type Channels, type MinSeverity, type Webhook } from '../api/alert'
import { errorText } from '../api/users'
import { cn } from '../utils/cn'

const input = 'w-full bg-base-800 border border-base-700 rounded-lg px-3 py-2 text-sm text-base-100 focus:outline-none focus:ring-2 focus:ring-brand-500/20 focus:border-brand-500'
const label = 'text-xs font-semibold text-base-400'
const btn = 'inline-flex items-center gap-1.5 h-9 px-3 rounded-lg border border-base-700 text-sm text-base-200 hover:bg-base-800 disabled:opacity-50'

// 쉼표 목록은 칸을 벗어날 때 나눔 — 입력 중에 나누면 "a, " 다음 글자를 칠 수 없음. 설정을 불러온 뒤에만 그려져 처음 값으로 충분
function ListField({ id, value, onChange, placeholder }: { id: string; value: string[]; onChange: (v: string[]) => void; placeholder: string }) {
  const [raw, setRaw] = useState(value.join(', '))
  return (
    <input id={id} className={input} value={raw} placeholder={placeholder}
      onChange={e => setRaw(e.target.value)}
      onBlur={() => onChange(raw.split(/[,\n]/).map(x => x.trim()).filter(Boolean))} />
  )
}

function HeadersField({ value, onChange }: { value: Record<string, string>; onChange: (v: Record<string, string>) => void }) {
  const toText = (h: Record<string, string>) => Object.entries(h).map(([k, v]) => `${k}: ${v}`).join('\n')
  const [raw, setRaw] = useState(toText(value))
  return (
    <textarea id="sms-headers" rows={2} className={cn(input, 'font-mono')} value={raw} placeholder="Authorization: Bearer 발급받은키"
      onChange={e => setRaw(e.target.value)}
      onBlur={() => onChange(Object.fromEntries(raw.split('\n').map(l => l.split(/:(.*)/s)).filter(p => p[0]?.trim() && p[1] !== undefined).map(p => [p[0].trim(), p[1].trim()])))} />
  )
}

function SeveritySelect({ id, value, def, onChange }: { id: string; value: MinSeverity; def: 'warning' | 'critical'; onChange: (v: MinSeverity) => void }) {
  return (
    <select id={id} className={input} value={value || def} onChange={e => onChange(e.target.value as MinSeverity)}>
      <option value="warning">주의 이상</option>
      <option value="critical">위험만</option>
    </select>
  )
}

// 시험 결과는 버튼 옆에 — 어떤 경로가 실패했는지 바로 보이게
function useTest() {
  const [state, setState] = useState<Record<string, { busy?: boolean; ok?: boolean; msg?: string }>>({})
  const run = async (key: string, fn: () => Promise<void>) => {
    setState(s => ({ ...s, [key]: { busy: true } }))
    try {
      await fn()
      setState(s => ({ ...s, [key]: { ok: true, msg: '보냈습니다 — 받는 쪽에서 확인하세요' } }))
    } catch (e) {
      setState(s => ({ ...s, [key]: { ok: false, msg: errorText(e, '보내지 못했습니다') } }))
    }
  }
  return { state, run }
}

function TestResult({ r }: { r?: { busy?: boolean; ok?: boolean; msg?: string } }) {
  if (!r || r.busy) return null
  return <p role="status" className={cn('text-xs', r.ok ? 'text-emerald-300' : 'text-rose-300')}>{r.msg}</p>
}

export default function NotifyChannels({ value, onChange }: { value: Channels; onChange: (c: Channels) => void }) {
  const { state, run } = useTest()
  const sms = value.sms
  const setSMS = (patch: Partial<typeof sms>) => onChange({ ...value, sms: { ...sms, ...patch } })
  const setHook = (i: number, patch: Partial<Webhook>) => onChange({ ...value, webhooks: value.webhooks.map((w, j) => j === i ? { ...w, ...patch } : w) })
  const esc = value.escalation

  return (
    <div className="space-y-8">
      <section className="space-y-3" aria-labelledby="hooks-title">
        <div className="flex items-center gap-2 px-1">
          <WebhookIcon size={16} className="text-brand-400" />
          <h4 id="hooks-title" className="text-sm font-bold text-base-200">웹훅</h4>
          <span className="text-xs text-base-500">메신저(Slack·Mattermost·잔디 등)나 상위 관제 시스템으로 보냄</span>
        </div>
        {value.webhooks.map((w, i) => (
          <div key={i} className="grid grid-cols-1 md:grid-cols-[8rem_minmax(0,1fr)_8rem_7rem] gap-2 p-3 rounded-xl border border-base-700 bg-base-800/40">
            <input aria-label="웹훅 이름" className={input} value={w.name} placeholder="이름" onChange={e => setHook(i, { name: e.target.value })} />
            <input aria-label="웹훅 주소" className={input} value={w.url} placeholder="https://hooks.slack.com/services/…" onChange={e => setHook(i, { url: e.target.value })} />
            <select aria-label="웹훅 형식" className={input} value={w.format} onChange={e => setHook(i, { format: e.target.value as Webhook['format'] })}>
              <option value="slack">메신저(text)</option>
              <option value="json">일반 JSON</option>
            </select>
            <SeveritySelect id={`hook-sev-${i}`} value={w.min_severity} def="warning" onChange={v => setHook(i, { min_severity: v })} />
            <div className="md:col-span-4 flex flex-wrap items-center gap-3">
              <label className="flex items-center gap-1.5 text-xs text-base-300">
                <input type="checkbox" checked={w.recovery} onChange={e => setHook(i, { recovery: e.target.checked })} /> 복구 알림도 보냄
              </label>
              <button type="button" className={btn} disabled={!w.url || state[`hook-${i}`]?.busy}
                onClick={() => run(`hook-${i}`, () => testChannel({ channel: 'webhook', webhook: w }))}>
                <Send size={14} /> 시험 발송
              </button>
              <button type="button" className={btn} aria-label={`${w.name || '웹훅'} 삭제`}
                onClick={() => onChange({ ...value, webhooks: value.webhooks.filter((_, j) => j !== i) })}>
                <Trash2 size={14} />
              </button>
              <TestResult r={state[`hook-${i}`]} />
            </div>
          </div>
        ))}
        <button type="button" className={btn}
          onClick={() => onChange({ ...value, webhooks: [...value.webhooks, { name: '', url: '', format: 'slack', min_severity: '', recovery: true }] })}>
          <Plus size={14} /> 웹훅 추가
        </button>
      </section>

      <section className="space-y-3" aria-labelledby="sms-title">
        <div className="flex flex-wrap items-center gap-2 px-1">
          <MessageSquare size={16} className="text-brand-400" />
          <h4 id="sms-title" className="text-sm font-bold text-base-200">문자(SMS)</h4>
          <span className="text-xs text-base-500">문자 업체나 기관 내부 문자 서버의 HTTP API</span>
          <label className="ml-auto flex items-center gap-1.5 text-xs text-base-300">
            <input type="checkbox" checked={sms.enabled} onChange={e => setSMS({ enabled: e.target.checked })} /> 사용
          </label>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-[minmax(0,1fr)_7rem_12rem] gap-2 p-3 rounded-xl border border-base-700 bg-base-800/40">
          <div className="space-y-1">
            <label htmlFor="sms-url" className={label}>게이트웨이 주소</label>
            <input id="sms-url" className={input} value={sms.url} placeholder="https://sms.example.go.kr/api/send" onChange={e => setSMS({ url: e.target.value })} />
          </div>
          <div className="space-y-1">
            <label htmlFor="sms-method" className={label}>방식</label>
            <select id="sms-method" className={input} value={sms.method} onChange={e => setSMS({ method: e.target.value as typeof sms.method })}>
              <option>POST</option><option>GET</option><option>PUT</option>
            </select>
          </div>
          <div className="space-y-1">
            <label htmlFor="sms-ct" className={label}>본문 형식</label>
            <select id="sms-ct" className={input} value={sms.content_type} onChange={e => setSMS({ content_type: e.target.value })}>
              <option value="application/json">JSON</option>
              <option value="application/x-www-form-urlencoded">폼(form)</option>
            </select>
          </div>
          <div className="md:col-span-3 space-y-1">
            <label htmlFor="sms-headers" className={label}>헤더 (한 줄에 하나, 이름: 값)</label>
            <HeadersField value={sms.headers} onChange={h => setSMS({ headers: h })} />
          </div>
          {sms.method !== 'GET' && (
            <div className="md:col-span-3 space-y-1">
              <label htmlFor="sms-body" className={label}>본문 양식 — <code>{'{{phone}}'}</code> 받는 번호, <code>{'{{message}}'}</code> 알림 제목</label>
              <textarea id="sms-body" rows={2} className={cn(input, 'font-mono')} value={sms.body} onChange={e => setSMS({ body: e.target.value })} />
            </div>
          )}
          <div className="md:col-span-2 space-y-1">
            <label htmlFor="sms-phones" className={label}>받는 번호 (쉼표로 구분)</label>
            <ListField id="sms-phones" value={sms.phones} onChange={v => setSMS({ phones: v })} placeholder="010-1234-5678, 010-…" />
          </div>
          <div className="space-y-1">
            <label htmlFor="sms-sev" className={label}>보낼 등급</label>
            <SeveritySelect id="sms-sev" value={sms.min_severity} def="critical" onChange={v => setSMS({ min_severity: v })} />
          </div>
          <div className="md:col-span-3 flex flex-wrap items-center gap-3">
            <label className="flex items-center gap-1.5 text-xs text-base-300">
              <input type="checkbox" checked={sms.recovery} onChange={e => setSMS({ recovery: e.target.checked })} /> 복구 알림도 보냄
            </label>
            <button type="button" className={btn} disabled={!sms.url || !sms.phones.length || state.sms?.busy}
              onClick={() => run('sms', () => testChannel({ channel: 'sms', sms, phone: sms.phones[0] }))}>
              <Send size={14} /> 첫 번호로 시험 발송
            </button>
            <TestResult r={state.sms} />
          </div>
        </div>
      </section>

      <section className="space-y-3" aria-labelledby="esc-title">
        <div className="flex flex-wrap items-center gap-2 px-1">
          <Siren size={16} className="text-rose-400" />
          <h4 id="esc-title" className="text-sm font-bold text-base-200">미확인 시 단계 통보</h4>
          <span className="text-xs text-base-500">화면에서 확인할 수 있는 위험 알림을 아무도 확인하지 않으면 2차 담당자에게 한 번 더 보냄</span>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-[9rem_minmax(0,1fr)_minmax(0,1fr)] gap-2 p-3 rounded-xl border border-base-700 bg-base-800/40">
          <div className="space-y-1">
            <label htmlFor="esc-min" className={label}>확인 기다리는 시간</label>
            <div className="flex items-center gap-1.5">
              <input id="esc-min" type="number" min={0} max={1440} className={input} value={esc.after_min || ''} placeholder="끔"
                onChange={e => onChange({ ...value, escalation: { ...esc, after_min: Math.max(0, Math.min(1440, Math.floor(Number(e.target.value) || 0))) } })} />
              <span className="text-xs text-base-400">분</span>
            </div>
          </div>
          <div className="space-y-1">
            <label htmlFor="esc-mails" className={label}>2차 담당자 메일</label>
            <ListField id="esc-mails" value={esc.recipients} onChange={v => onChange({ ...value, escalation: { ...esc, recipients: v } })} placeholder="manager@example.go.kr" />
          </div>
          <div className="space-y-1">
            <label htmlFor="esc-phones" className={label}>2차 담당자 문자 (문자 사용을 켜야 나감)</label>
            <ListField id="esc-phones" value={esc.phones} onChange={v => onChange({ ...value, escalation: { ...esc, phones: v } })} placeholder="010-…" />
          </div>
        </div>
      </section>
    </div>
  )
}
