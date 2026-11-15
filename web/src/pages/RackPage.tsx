import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus, Rows3, Trash2 } from 'lucide-react'
import PageToolbar from '../components/PageToolbar'
import { fetchAssets, unitRange, type Asset } from '../api/asset'
import { deleteRack, fetchRacks, saveRack, type Rack } from '../api/rack'
import { getMe } from '../api/auth'
import { errorText } from '../api/users'
import { useFleet, LABEL, type State, type Tile } from '../hooks/useFleet'
import { cn } from '../utils/cn'

// 42U 랙이 한 화면(약 760px)에 들어오는 칸 높이
const U_H = 18
// 상태색은 왼쪽 막대와 점에만 — 정상 장비까지 색을 칠하면 장애 장비가 묻힘
const BAR: Record<State, string> = { crit: 'bg-rose-500', warn: 'bg-amber-400', maint: 'bg-purple-400', ok: 'bg-base-600', wait: 'bg-base-700' }
const inputCls = 'h-9 px-2.5 rounded-lg bg-base-800 border border-base-700 text-sm text-base-100 focus:outline-none focus:border-brand-500'

export default function RackPage() {
  const qc = useQueryClient()
  const { data: racks, isSuccess: racksReady, isError: racksError } = useQuery({ queryKey: ['racks'], queryFn: fetchRacks })
  const { data: assets, isSuccess: assetsReady, isError: assetsError } = useQuery({ queryKey: ['assets'], queryFn: fetchAssets })
  const { data: me } = useQuery({ queryKey: ['me'], queryFn: getMe, staleTime: 5 * 60_000 })
  const { tiles, groupPending, groupFailed } = useFleet()
  const admin = me?.role === 'admin'
  const [form, setForm] = useState<{ rack: Rack; editing: boolean } | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  // 처리할 문제·장비 상세의 위치를 누르고 들어오면 그 장비를 가운데로 (?focus=장비이름)
  const focus = useSearchParams()[0].get('focus')
  // 위쪽 안내 띠·장비 상태가 늦게 와서 화면이 밀리므로, 다 받은 뒤 한 박자 늦게 맞춘다
  const settled = racksReady && assetsReady && !groupPending('서버') && !groupPending('네트워크 장비')
  useEffect(() => {
    if (!settled || !focus) return
    // 본문이 부드러운 스크롤이라 그대로 두면 이어지는 렌더에 묻혀 멈춤 — 바로 이동. 아래쪽 목록이 늦게 그려지면 여유가 모자라 한 번 더
    const go = () => document.getElementById(`rack-dev-${focus}`)?.scrollIntoView({ block: 'center', inline: 'center', behavior: 'instant' })
    const ts = [setTimeout(go, 300), setTimeout(go, 1200)]
    return () => ts.forEach(clearTimeout)
  }, [settled, focus])

  if (racksError || assetsError) return <div role="alert" className="py-12 text-center text-sm text-rose-300">랙·장비 정보를 불러오지 못했습니다</div>
  if (!racksReady || !assetsReady) return <div className="py-12 text-center text-sm text-base-500">불러오는 중...</div>

  // 같은 이름의 웹사이트·인증서 타일과 섞이지 않게 랙에 들어가는 종류부터 찾는다
  const tileOf = (name: string): Tile | undefined =>
    tiles.find(t => t.name === name && (t.group === '서버' || t.group === '네트워크 장비')) ?? tiles.find(t => t.name === name && t.group === '데이터베이스')
  // 랙 목록에 없는 랙을 가리키는 장비(동시 삭제 등)는 그릴 곳이 없으니 위치 없는 장비로 본다
  const rackNames = new Set((racks ?? []).map(r => r.name))
  const placed = (assets ?? []).filter(a => a.rack && a.rack_unit > 0 && rackNames.has(a.rack))
  const unplaced = tiles.filter(t => (t.group === '서버' || t.group === '네트워크 장비') && !placed.some(a => a.host_name === t.name))
  // 장비 상태를 아직 못 받았거나 못 받으면 상태가 없는 장비를 "감시 안 함"으로 단정하지 않음
  // 랙 장비는 서버·네트워크 장비, 없으면 DB로 상태를 찾으니 세 종류 모두 본다
  const statusPending = groupPending('서버') || groupPending('네트워크 장비') || groupPending('데이터베이스')
  const statusFailed = groupFailed('서버') || groupFailed('네트워크 장비') || groupFailed('데이터베이스')
  const noTile = statusFailed ? '상태 모름' : statusPending ? '확인 중' : '감시하지 않는 장비'
  const rooms = [...new Set((racks ?? []).map(r => r.room))]

  const run = async (fn: () => Promise<void>) => {
    setBusy(true)
    setError('')
    try {
      await fn()
      setForm(null)
      qc.invalidateQueries({ queryKey: ['racks'] })
    } catch (e) {
      setError(errorText(e, '저장하지 못했습니다'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-5">
      <PageToolbar icon={Rows3} title="랙 실장도" description={admin ? '랙 앞면 기준 — 아래가 1U. 장비의 랙·칸은 장비 목록이나 구성도 편집의 엑셀 일괄 등록에서 정합니다' : '랙 앞면 기준 — 아래가 1U'}>
        {admin && !form && (
          <button type="button" onClick={() => { setError(''); setForm({ rack: { name: '', room: rooms[0] ?? '', height: 42 }, editing: false }) }}
            className="flex items-center gap-2 px-4 py-2 rounded-lg bg-brand-600 hover:bg-brand-500 text-white text-sm font-semibold">
            <Plus size={16} /> 랙 추가
          </button>
        )}
      </PageToolbar>

      {form && (
        <form
          onSubmit={e => {
            e.preventDefault()
            // 같은 이름으로 "추가"하면 서버는 기존 랙을 고친다 — 다른 장소·높이의 기존 랙을 모르고 덮어쓰지 않게
            if (!form.editing && rackNames.has(form.rack.name.trim())) return setError(`"${form.rack.name.trim()}"은(는) 이미 있는 랙입니다 — 바꾸려면 그 랙의 연필 버튼을 누르세요`)
            run(() => saveRack(form.rack))
          }}
          className="flex flex-wrap items-end gap-3 border-y border-base-800 px-4 py-3"
        >
          <label className="space-y-1 text-xs text-base-500">
            <span>랙 이름</span>
            <input required value={form.rack.name} disabled={form.editing} onChange={e => setForm({ ...form, rack: { ...form.rack, name: e.target.value } })}
              placeholder="예: A랙" className={cn(inputCls, 'block w-36 disabled:opacity-60')} />
          </label>
          <label className="space-y-1 text-xs text-base-500">
            <span>설치 장소</span>
            <input value={form.rack.room} list="rack-rooms" onChange={e => setForm({ ...form, rack: { ...form.rack, room: e.target.value } })}
              placeholder="예: 본관 2층 서버실" className={cn(inputCls, 'block w-56')} />
            <datalist id="rack-rooms">{rooms.filter(Boolean).map(r => <option key={r} value={r} />)}</datalist>
          </label>
          <label className="space-y-1 text-xs text-base-500">
            <span>높이 (U)</span>
            <input type="number" min={1} max={60} required value={form.rack.height || ''}
              onChange={e => setForm({ ...form, rack: { ...form.rack, height: Math.floor(Number(e.target.value) || 0) } })}
              className={cn(inputCls, 'block w-24')} />
          </label>
          <button type="submit" disabled={busy} className="h-9 px-4 rounded-lg bg-brand-600 hover:bg-brand-500 text-white text-sm font-semibold disabled:opacity-50">
            {busy ? '저장 중' : '저장'}
          </button>
          <button type="button" onClick={() => { setForm(null); setError('') }} className="h-9 px-3 rounded-lg border border-base-700 text-sm text-base-300 hover:bg-base-800">취소</button>
        </form>
      )}
      {error && <p role="alert" className="border-y border-base-800 px-4 py-2.5 text-sm text-rose-300">{error}</p>}
      {statusFailed && <p role="alert" className="border-y border-base-800 px-4 py-2.5 text-sm text-base-300">서버·네트워크 장비 상태를 불러오지 못했습니다 — 랙의 상태 표시가 최신이 아닐 수 있습니다</p>}

      {(racks ?? []).length === 0 ? (
        <div className="border-y border-base-800 py-16 text-center space-y-2">
          <p className="text-sm font-semibold text-base-300">등록된 랙이 없습니다</p>
          {/* 조회 계정은 설정 메뉴(장비 목록·구성도 편집)에 못 들어가므로 갈 수 없는 곳을 안내하지 않음 */}
          <p className="text-sm text-base-500">
            {admin ? (
              <>
                <Link to="/assets" className="text-brand-300 hover:underline">장비 목록</Link>에서 장비의 랙·시작 U를 적거나, <Link to="/topology" className="text-brand-300 hover:underline">구성도 편집</Link>의 엑셀 일괄 등록에 랙·U 열을 채우면 랙이 자동으로 만들어집니다
              </>
            ) : '관리자에게 장비의 랙 위치 등록을 요청하세요'}
          </p>
        </div>
      ) : (
        // 장소가 달라도 옆으로 늘어놓음 — 42U 랙은 세로로 길어서 장소별로 아래로 쌓으면 다른 장소 랙을 보려면 한참 내려야 함
        <div className="flex flex-wrap items-start gap-x-8 gap-y-5">
        {rooms.map(room => (
          <section key={room} className="space-y-3" aria-label={room || '설치 장소 미지정'}>
            <h2 className="text-sm font-semibold text-base-200">{room || <span className="text-base-500">설치 장소 미지정</span>}</h2>
            <div className="flex flex-wrap items-start gap-4">
              {(racks ?? []).filter(r => r.room === room).map(rack => (
                <RackView
                  key={rack.name} rack={rack} assets={placed.filter(a => a.rack === rack.name)} tileOf={tileOf} noTile={noTile} focus={focus}
                  onEdit={admin ? () => { setError(''); setForm({ rack, editing: true }) } : undefined}
                  onDelete={admin ? () => run(() => deleteRack(rack.name)) : undefined}
                />
              ))}
            </div>
          </section>
        ))}
        </div>
      )}

      {/* 목록을 못 받은 종류는 통째로 빠져 대수가 틀리므로, 실패 중엔 개수를 보여 주지 않음 */}
      {unplaced.length > 0 && !statusPending && !statusFailed && (
        <section className="border-y border-base-800 px-4 py-3 text-sm" aria-label="랙 위치가 없는 장비">
          <span className="text-base-400">랙 위치가 없는 장비 {unplaced.length}대 </span>
          <span className="text-base-500">{unplaced.slice(0, 12).map(t => t.name).join(', ')}{unplaced.length > 12 && ` 외 ${unplaced.length - 12}대`}</span>
          {admin && <Link to="/assets" className="ml-2 text-brand-300 hover:underline">장비 목록에서 정하기</Link>}
        </section>
      )}
    </div>
  )
}

function RackView({ rack, assets, tileOf, noTile, focus, onEdit, onDelete }: {
  rack: Rack; assets: Asset[]; tileOf: (name: string) => Tile | undefined; noTile: string; focus: string | null; onEdit?: () => void; onDelete?: () => void
}) {
  const used = assets.reduce((n, a) => n + (a.rack_size || 1), 0)
  const bad = assets.filter(a => { const t = tileOf(a.host_name); return (t?.state === 'crit' || t?.state === 'warn') && !t.acked && !t.causedBy }).length
  return (
    <figure className="w-60 rounded-lg border border-base-800 bg-base-900/50" aria-label={`${rack.name} 랙`}>
      <figcaption className="flex items-center gap-2 px-3 py-2 border-b border-base-800">
        <span className="font-semibold text-base-100">{rack.name}</span>
        <span className="text-xs text-base-500 tabular-nums">{used}/{rack.height}U</span>
        {bad > 0 && <span className="text-xs font-semibold text-rose-300 tabular-nums">문제 {bad}</span>}
        <span className="ml-auto flex gap-0.5">
          {onEdit && <button type="button" onClick={onEdit} aria-label={`${rack.name} 수정`} className="p-1 rounded text-base-500 hover:text-base-100 hover:bg-base-800"><Pencil size={13} /></button>}
          {onDelete && assets.length === 0 && (
            <button type="button" onClick={onDelete} aria-label={`${rack.name} 삭제`} className="p-1 rounded text-base-500 hover:text-rose-300 hover:bg-base-800"><Trash2 size={13} /></button>
          )}
        </span>
      </figcaption>
      <div className="flex px-2 py-2">
        {/* 칸 번호 — 랙 레일 표기처럼 아래가 1 */}
        <ol className="w-6 shrink-0 text-right pr-1.5 text-[10px] leading-none text-base-600 tabular-nums" aria-hidden>
          {Array.from({ length: rack.height }, (_, i) => rack.height - i).map(u => (
            <li key={u} style={{ height: U_H }} className="flex items-center justify-end">{u % 5 === 0 || u === 1 ? u : ''}</li>
          ))}
        </ol>
        <div className="relative flex-1 border-x-2 border-base-700 bg-[repeating-linear-gradient(to_bottom,transparent_0,transparent_17px,var(--color-base-800)_17px,var(--color-base-800)_18px)]"
          style={{ height: rack.height * U_H }}>
          {assets.map(a => <Device key={a.host_name} asset={a} rack={rack} tile={tileOf(a.host_name)} noTile={noTile} focused={a.host_name === focus} />)}
        </div>
      </div>
    </figure>
  )
}

function Device({ asset, rack, tile, noTile, focused }: { asset: Asset; rack: Rack; tile?: Tile; noTile: string; focused: boolean }) {
  const size = asset.rack_size || 1
  const state: State = tile?.state ?? 'wait'
  const bad = state === 'crit' || state === 'warn'
  // 확인했거나 상위 장비 때문에 같이 끊긴 장비는 처리할 문제가 아님 — 메인 장비 표·구성도와 같은 기준으로 흐리게
  const open = bad && !tile?.acked && !tile?.causedBy
  const label = tile?.causedBy ? '상위 영향' : bad && tile?.acked ? '확인함' : tile && state !== 'ok' ? LABEL[state] : ''
  const blink = open && state === 'crit'
  const pos = `${rack.name} ${unitRange(asset.rack_unit, size)}`
  const title = [asset.host_name, tile ? label || LABEL[state] : noTile, tile?.detail, pos].filter(Boolean).join('\n')
  const body = (
    <>
      <span className={cn('w-1 self-stretch shrink-0 rounded-sm', tile?.causedBy ? 'bg-base-500' : BAR[state], blink && 'motion-safe:animate-blink')} />
      <span className="min-w-0 flex-1 truncate">
        <span className={cn('font-semibold', open ? 'text-base-50' : 'text-base-200')}>{asset.host_name}</span>
        {size >= 2 && asset.description && <span className="ml-1.5 text-base-500">{asset.description}</span>}
      </span>
      {label && <span className={cn('shrink-0 text-[10px]', open ? 'text-base-200' : 'text-base-500')}>{label}</span>}
    </>
  )
  const cls = cn(
    'absolute inset-x-0.5 flex items-center gap-1.5 px-1 rounded-sm border text-xs overflow-hidden',
    open && state === 'crit' ? 'border-rose-500/50 bg-rose-500/10' : open ? 'border-amber-400/40 bg-amber-400/10' : 'border-base-700 bg-base-800',
    tile && 'hover:border-base-500',
    focused && 'z-10 ring-2 ring-brand-400 ring-offset-1 ring-offset-base-900',
  )
  const style = { top: (rack.height - (asset.rack_unit + size - 1)) * U_H + 1, height: size * U_H - 2 }
  return tile
    ? <Link id={`rack-dev-${asset.host_name}`} to={tile.to} title={title} aria-label={title.replace(/\n/g, ', ')} className={cls} style={style}>{body}</Link>
    : <div id={`rack-dev-${asset.host_name}`} title={title} aria-label={title.replace(/\n/g, ', ')} className={cls} style={style}>{body}</div>
}
