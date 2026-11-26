import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { ServerCog, Plus, Ban, CheckCircle2, Loader2, AlertTriangle } from 'lucide-react'
import PageToolbar from '../components/PageToolbar'
import ConfirmDialog from '../components/ConfirmDialog'
import { CodeBlock } from './SupportPage'
import { getAgents, issueEnrollToken, revokeAgent, type Agent, type AgentsInfo } from '../api/agents'
import { errorText } from '../api/users'
import { useHosts } from '../hooks/useMonitoring'
import { TONES } from '../utils/severity'
import { ago } from '../utils/time'
import { cn } from '../utils/cn'

const inputCls = 'w-full px-3 py-2 rounded-lg bg-base-800 border border-base-700 text-sm text-base-100 focus:outline-none focus:border-brand-500'
const TOKEN_TTL_MS = 24 * 60 * 60_000 // 서버의 등록 토큰 유효기간과 같게

type Issued = { name: string; token: string; expiresAt: string }

// 설치 명령 한 줄 — 자체 서명 인증서면 검증을 끄는 대신 인증서 지문을 고정한다
function installCommands(server: string, token: string, cert: AgentsInfo['cert'], pinned: boolean) {
  const sh = `${server}/api/agent/install-agent.sh`
  const ps = `${server}/api/agent/install-agent.ps1`
  const tls = `[Net.ServicePointManager]::SecurityProtocol='Tls12'; `
  if (pinned && cert) {
    const pin = `'sha256//${cert.spki_pin}'`
    return {
      linux: `curl -fsSk --pinnedpubkey ${pin} ${sh} | sudo bash -s -- --server ${server} --enroll-token ${token} --pin ${pin} --server-cert-sha256 ${cert.sha256}`,
      windows: `${tls}$p='${cert.sha256}'; [Net.ServicePointManager]::ServerCertificateValidationCallback={param($s,$c)([BitConverter]::ToString([Security.Cryptography.SHA256]::Create().ComputeHash($c.GetRawCertData())) -replace '-') -eq $p}; & ([scriptblock]::Create((iwr -UseBasicParsing ${ps}).Content)) -Server ${server} -EnrollToken ${token} -ServerCertSha256 ${cert.sha256}`,
    }
  }
  return {
    linux: `curl -fsS ${sh} | sudo bash -s -- --server ${server} --enroll-token ${token}`,
    windows: `${tls}& ([scriptblock]::Create((iwr -UseBasicParsing ${ps}).Content)) -Server ${server} -EnrollToken ${token}`,
  }
}

export default function AgentsPage() {
  const qc = useQueryClient()
  const [issued, setIssued] = useState<Issued | null>(null)
  const issuedAt = issued ? new Date(issued.expiresAt).getTime() - TOKEN_TTL_MS : 0
  const { data, isLoading, isError } = useQuery({
    queryKey: ['agents'], queryFn: getAgents,
    refetchInterval: issued ? 5_000 : 60_000, // 설치를 기다리는 동안엔 자주 확인
  })
  const { data: hosts = [] } = useHosts()

  const [adding, setAdding] = useState(false)
  const [name, setName] = useState('')
  const [server, setServer] = useState(window.location.origin)
  const [pinnedChoice, setPinned] = useState<boolean | null>(null)
  const [os, setOs] = useState<'linux' | 'windows'>('linux')
  const [error, setError] = useState('')
  const [confirm, setConfirm] = useState<Agent | null>(null)

  const cert = data?.cert ?? null
  const https = server.startsWith('https://')
  const pinned = https && !!cert && (pinnedChoice ?? cert.self_signed)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    try {
      const r = await issueEnrollToken(name.trim())
      setIssued({ name: r.name, token: r.token, expiresAt: r.expires_at })
      setAdding(false)
      setName('')
      qc.invalidateQueries({ queryKey: ['agents'] })
    } catch (err) {
      setError(errorText(err, '설치 명령을 만들지 못했습니다'))
    }
  }

  const revoke = async (a: Agent) => {
    setError('')
    try {
      await revokeAgent(a.id)
      qc.invalidateQueries({ queryKey: ['agents'] })
    } catch (err) {
      setError(errorText(err, '폐기하지 못했습니다'))
    }
  }

  // 등록됨: 이번 토큰으로 키를 받아 한 번이라도 접속 / 연결됨: 그 이름으로 메트릭이 들어옴
  const agent = issued ? data?.agents.find(a => a.name === issued.name) : undefined
  const registered = !!agent?.last_seen && new Date(agent.last_seen).getTime() >= issuedAt
  const connected = registered && hosts.includes(issued!.name)
  const cmds = issued && installCommands(server.replace(/\/+$/, ''), issued.token, cert, pinned)
  const now = new Date()

  return (
    <div className="space-y-5">
      <PageToolbar icon={ServerCog} title="서버 추가" description="모니터링할 서버마다 설치 명령을 만들어 그 서버에서 한 번 실행하세요">
        <button type="button" onClick={() => setAdding(a => !a)}
          className="flex items-center gap-2 px-4 py-2 rounded-lg bg-brand-600 hover:bg-brand-500 text-white text-sm font-semibold">
          <Plus size={16} /> 서버 추가
        </button>
      </PageToolbar>

      {data?.legacy_shared_key && (
        <div role="alert" className={cn('rounded-xl border px-4 py-3 text-sm flex gap-2.5', TONES.amber.bg, TONES.amber.border, TONES.amber.text)}>
          <AlertTriangle size={18} className="shrink-0 mt-0.5" />
          <p>예전 공용 키도 받고 있습니다. 공용 키로 들어온 데이터는 어느 서버가 보냈는지 확인할 수 없습니다. 서버마다 아래 [서버 추가]로 다시 설치한 뒤, OWLmon 서버의 <code className="font-mono">.env</code>에서 <code className="font-mono">OWLMON_LEGACY_SHARED_KEY=false</code>로 바꾸세요.</p>
        </div>
      )}
      {error && <div role="alert" className={cn('rounded-xl border px-4 py-2.5 text-sm', TONES.rose.bg, TONES.rose.border, TONES.rose.text)}>{error}</div>}

      {adding && (
        <form onSubmit={submit} className="rounded-xl border border-base-800 bg-base-900 p-4 grid grid-cols-1 sm:grid-cols-3 gap-3 items-end">
          <label className="space-y-1">
            <span className="text-xs font-semibold text-base-400">서버 이름</span>
            <input value={name} onChange={e => setName(e.target.value)} className={inputCls} placeholder="예: web-01" required
              pattern="[A-Za-z0-9][A-Za-z0-9._\-]{0,62}" title="영문·숫자·점(.)·밑줄(_)·하이픈(-), 63자 이내" />
          </label>
          <label className="space-y-1">
            <span className="text-xs font-semibold text-base-400">OWLmon 주소 (설치할 서버에서 열리는 주소)</span>
            <input value={server} onChange={e => setServer(e.target.value)} className={inputCls} required pattern="https://.+|http://(localhost|127\.0\.0\.1)((:|/).*)?" title="https 주소 (설치 명령의 토큰·키가 평문으로 오가지 않게)" />
          </label>
          <button type="submit" className="py-2 rounded-lg bg-brand-600 hover:bg-brand-500 text-white text-sm font-semibold">설치 명령 만들기</button>
          <p className="sm:col-span-3 text-xs text-base-500">그 서버의 호스트 이름과 같게 쓰면 지난 기록이 그대로 이어집니다. 화면과 알림엔 이 이름으로만 표시됩니다.</p>
        </form>
      )}

      {issued && cmds && (
        <section className="rounded-xl border border-base-800 bg-base-900 p-4 space-y-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h2 className="text-sm font-bold text-base-100">{issued.name} 설치 명령</h2>
            <span className={cn('flex items-center gap-1.5 px-2.5 py-1 rounded-lg border text-xs font-semibold',
              connected ? [TONES.emerald.bg, TONES.emerald.border, TONES.emerald.text] : [TONES.slate.bg, TONES.slate.border, TONES.slate.text])}>
              {connected ? <CheckCircle2 size={14} /> : <Loader2 size={14} className="animate-spin" />}
              {connected ? '연결됨' : registered ? '등록됨 — 첫 데이터를 기다리는 중' : '설치를 기다리는 중'}
            </span>
          </div>

          <div className={cn('rounded-lg border px-3 py-2 text-sm', TONES.amber.bg, TONES.amber.border, TONES.amber.text)}>
            이 명령은 지금 한 번만 볼 수 있습니다. 이 화면을 벗어나면 다시 볼 수 없으니, 필요하면 새로 만드세요.
            {' '}{new Date(issued.expiresAt).toLocaleString('ko-KR')}까지 한 번만 쓸 수 있습니다.
          </div>

          <div className="flex gap-1 p-1 w-fit bg-base-800 rounded-lg border border-base-700" role="tablist">
            {(['linux', 'windows'] as const).map(o => (
              <button key={o} type="button" role="tab" aria-selected={os === o} onClick={() => setOs(o)}
                className={cn('px-3 py-1.5 rounded-md text-sm font-semibold', os === o ? 'bg-base-700 text-base-50' : 'text-base-400 hover:text-base-200')}>
                {o === 'linux' ? 'Linux' : 'Windows'}
              </button>
            ))}
          </div>
          <CodeBlock code={cmds[os]}
            note={os === 'linux' ? '그 서버에서 sudo 권한으로 붙여 넣으세요.' : 'PowerShell을 관리자 권한으로 열고 붙여 넣으세요.'} />

          {https && cert && (
            <label className="flex items-start gap-2 text-sm text-base-300">
              <input type="checkbox" checked={pinned} onChange={e => setPinned(e.target.checked)} className="mt-1 accent-brand-500" />
              <span>
                자체 서명 인증서 (기본 설치)
                <span className="block text-xs text-base-500">인증서 확인을 끄지 않고, 이 OWLmon의 인증서 지문만 믿도록 명령에 넣습니다. 기관 인증서로 바꿨다면 끄세요.</span>
              </span>
            </label>
          )}
          {!https && <p className="text-xs text-base-500">http는 이 PC의 localhost 시험에만 쓸 수 있습니다. 설치 스크립트와 에이전트가 다른 http 주소로는 등록하지 않습니다.</p>}
        </section>
      )}

      {isLoading ? (
        <div className="py-12 text-center text-sm text-base-500">불러오는 중...</div>
      ) : isError ? (
        <div role="alert" className="py-12 text-center text-sm text-rose-300">서버 목록을 불러오지 못했습니다</div>
      ) : data && data.agents.length > 0 ? (
        <ul className="rounded-xl border border-base-800 bg-base-900/50 divide-y divide-base-800">
          {data.agents.map(a => (
            <li key={a.id} className={cn('px-4 py-3 flex items-center gap-3', a.status === 'blocked' && 'opacity-60')}>
              <div className="flex-1 min-w-0">
                <Link to={`/detail?host=${encodeURIComponent(a.name)}`} className="text-sm font-semibold text-base-100 hover:text-brand-300 truncate">{a.name}</Link>
                <p className="text-xs text-base-500 mt-0.5">
                  {a.status === 'blocked' ? '폐기됨 — 이 서버의 데이터는 받지 않습니다' : a.last_seen ? `마지막 연결 ${ago(a.last_seen, now)}` : '아직 연결된 적 없음'}
                </p>
              </div>
              {a.status !== 'blocked' && (
                <button type="button" onClick={() => setConfirm(a)}
                  className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-base-700 text-xs font-semibold text-base-300 hover:text-rose-300 hover:border-rose-500/40">
                  <Ban size={14} /> 폐기
                </button>
              )}
            </li>
          ))}
        </ul>
      ) : (
        <p className="py-8 text-center text-sm text-base-500">[서버 추가]로 등록한 서버가 아직 없습니다.</p>
      )}

      <ConfirmDialog
        open={!!confirm}
        title="서버 키 폐기"
        message={`${confirm?.name}의 키를 바로 막습니다. 그 서버에서 오는 메트릭·로그는 더 이상 받지 않습니다. 다시 쓰려면 [서버 추가]로 새로 설치하세요.`}
        confirmLabel="폐기"
        onConfirm={() => { if (confirm) revoke(confirm); setConfirm(null) }}
        onCancel={() => setConfirm(null)}
      />
    </div>
  )
}
