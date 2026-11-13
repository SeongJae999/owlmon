import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowDown, ArrowUp, CornerLeftUp, Plus, RotateCcw, Save, Trash2, Workflow, X } from 'lucide-react'
import PageToolbar from '../components/PageToolbar'
import TopologyMap from '../components/TopologyMap'
import { useHosts } from '../hooks/useMonitoring'
import { getSNMPStatus } from '../api/snmp'
import { errorText } from '../api/users'
import {
  defaultLayout, getTopologyLayout, saveTopologyLayout,
  type DeviceKind, type TopologyDevice, type TopologyLayout,
} from '../api/topology'
import { cn } from '../utils/cn'

const KIND_LABEL: Record<DeviceKind, string> = { internet: '인터넷·회선', firewall: '방화벽·보안', network: '스위치·네트워크', server: '서버' }
const inputCls = 'w-full h-9 px-2.5 rounded-lg bg-base-800 border border-base-700 text-sm text-base-100 focus:outline-none focus:border-brand-500'

// 구역이 빈 장비는 구성도에서 뺀 것
interface Row extends TopologyDevice {
  monitored: boolean
}

export default function TopologyPage() {
  const { data: hosts, isSuccess: hostsReady } = useHosts()
  const { data: snmp, isSuccess: snmpReady } = useQuery({ queryKey: ['snmpStatus'], queryFn: getSNMPStatus })
  const { data: layout, isSuccess: layoutReady, isError } = useQuery({ queryKey: ['topologyLayout'], queryFn: getTopologyLayout })
  // 저장하면 편집기가 새로 그려지므로 저장 완료 안내는 바깥에서 들고 있는다
  const [notice, setNotice] = useState('')

  if (isError) return <div role="alert" className="py-12 text-center text-sm text-rose-300">구성도 설정을 불러오지 못했습니다</div>
  if (!hostsReady || !snmpReady || !layoutReady) return <div className="py-12 text-center text-sm text-base-500">불러오는 중...</div>

  const servers = hosts ?? []
  const networks = (snmp ?? []).map(d => d.Device.Name)
  // 저장된 내용이 바뀌면 편집기를 새로 그려 초안을 다시 잡는다
  return <Editor key={JSON.stringify(layout)} saved={layout} servers={servers} networks={networks} notice={notice} setNotice={setNotice} />
}

function initialRows(saved: TopologyLayout, servers: string[], networks: string[]): { zones: string[]; rows: Row[] } {
  const monitored = new Set([...servers, ...networks])
  const base = saved.devices.length ? saved : defaultLayout(servers, networks)
  const rows: Row[] = base.devices.map(d => ({ ...d, monitored: monitored.has(d.name) }))
  const known = new Set(rows.map(r => r.name))
  for (const name of servers) if (!known.has(name)) rows.push({ name, kind: 'server', zone: '', monitored: true })
  for (const name of networks) if (!known.has(name)) rows.push({ name, kind: 'network', zone: '', monitored: true })
  return { zones: base.zones, rows }
}

// 구성도에서 뺀 장비를 가리키는 연결은 저장할 수 없으니 정리해서 보낸다
function toLayout(zones: string[], rows: Row[]): TopologyLayout {
  const placed = new Set(rows.filter(r => r.zone).map(r => r.name))
  return {
    zones,
    devices: rows.filter(r => r.zone).map(({ name, kind, zone, parent, pair }) => ({
      name, kind, zone,
      ...(parent && placed.has(parent) ? { parent } : {}),
      ...(pair && placed.has(pair) ? { pair } : {}),
    })),
  }
}

function Editor({ saved, servers, networks, notice, setNotice }: {
  saved: TopologyLayout; servers: string[]; networks: string[]; notice: string; setNotice: (s: string) => void
}) {
  const qc = useQueryClient()
  const init = initialRows(saved, servers, networks)
  const [zones, setZones] = useState(init.zones)
  const [rows, setRows] = useState(init.rows)
  const [newZone, setNewZone] = useState('')
  const [newNode, setNewNode] = useState('')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  // 구성도에서 고른 장비 — 다음에 누르는 장비 아래로 연결된다
  const [picked, setPicked] = useState<string | null>(null)
  useEffect(() => {
    if (!picked) return
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setPicked(null) }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [picked])

  const draft = toLayout(zones, rows)
  const dirty = JSON.stringify(draft) !== JSON.stringify(saved.devices.length ? saved : null)

  const update = (name: string, patch: Partial<Row>) => setRows(rs => rs.map(r => r.name === name ? { ...r, ...patch } : r))

  // 짝은 양쪽에 서로 적고, 둘 중 누구와 짝이던 장비는 풀어서 한 장비가 짝을 하나만 갖게
  const setPair = (name: string, mate: string) => setRows(rs => rs.map(r => {
    if (r.name === name) return { ...r, pair: mate || undefined }
    if (mate && r.name === mate) return { ...r, pair: name }
    if (r.pair === name || (mate && r.pair === mate)) return { ...r, pair: undefined }
    return r
  }))

  const descendants = (name: string) => {
    const out = new Set<string>([name])
    for (let grew = true; grew;) {
      grew = false
      for (const r of rows) if (r.parent && out.has(r.parent) && !out.has(r.name)) { out.add(r.name); grew = true }
    }
    return out
  }

  const addZone = (e: React.FormEvent) => {
    e.preventDefault()
    const z = newZone.trim()
    if (!z || zones.includes(z)) return
    setZones([...zones, z])
    setNewZone('')
  }
  const moveZone = (i: number, d: -1 | 1) => {
    const next = [...zones];
    [next[i], next[i + d]] = [next[i + d], next[i]]
    setZones(next)
  }
  const removeZone = (z: string) => {
    setZones(zones.filter(x => x !== z))
    setRows(rs => rs.map(r => r.zone === z ? { ...r, zone: '' } : r))
  }

  const addNode = (e: React.FormEvent) => {
    e.preventDefault()
    const n = newNode.trim()
    if (!n || rows.some(r => r.name === n)) return
    setRows([...rows, { name: n, kind: 'internet', zone: zones[0] ?? '', monitored: false }])
    setNewNode('')
  }

  const save = async (layout: TopologyLayout, ok: string) => {
    setSaving(true)
    setError('')
    setNotice('')
    try {
      await saveTopologyLayout(layout)
      setNotice(ok)
      qc.invalidateQueries({ queryKey: ['topologyLayout'] })
    } catch (e) {
      setError(errorText(e, '저장하지 못했습니다'))
    } finally {
      setSaving(false)
    }
  }

  const placed = rows.filter(r => r.zone)
  const pickedRow = rows.find(r => r.name === picked)
  const pickedBlocked = picked ? descendants(picked) : new Set<string>()

  const pick = (name: string) => {
    if (!picked || !pickedRow) return setPicked(name)
    if (name === picked) return setPicked(null)
    if (pickedBlocked.has(name)) return
    const target = rows.find(r => r.name === name)!
    // 구성도에 없던 장비는 연결하면서 넣는다 — 같은 종류 장비가 있는 구역, 없으면 서버는 맨 아래 구역, 나머지는 위 장비와 같은 구역
    // 위 장비보다 위쪽 구역은 쓰지 않음 — 자식이 부모보다 위에 그려지면 선이 거꾸로 올라감
    const sameKind = rows.find(r => r.zone && r.kind === pickedRow.kind && r.name !== picked && zones.indexOf(r.zone) >= zones.indexOf(target.zone))
    const zone = pickedRow.zone || sameKind?.zone || (pickedRow.kind === 'server' ? zones[zones.length - 1] ?? target.zone : target.zone)
    update(picked, { parent: name, zone })
    setNotice('')
    setPicked(null)
  }

  return (
    <div className="space-y-5">
      <PageToolbar icon={Workflow} title="구성도" description="장비를 누른 뒤 그 장비가 연결된 위쪽 장비를 누르면 선이 이어집니다. 배치는 자동으로 정렬됩니다">
        {saved.devices.length > 0 && (
          <button
            type="button" disabled={saving}
            onClick={() => save({ zones: [], devices: [] }, '자동 배치로 되돌렸습니다')}
            className="flex items-center gap-2 px-4 py-2 rounded-lg border border-base-700 text-sm font-semibold text-base-300 hover:bg-base-800 disabled:opacity-50"
          >
            <RotateCcw size={16} /> 자동 배치로 되돌리기
          </button>
        )}
        <button
          type="button" disabled={saving || !dirty}
          onClick={() => save(draft, '구성도를 저장했습니다. 상황판에 바로 반영됩니다')}
          className="flex items-center gap-2 px-4 py-2 rounded-lg bg-brand-600 hover:bg-brand-500 text-white text-sm font-semibold disabled:opacity-50"
        >
          <Save size={16} /> {saving ? '저장 중' : '저장'}
        </button>
      </PageToolbar>

      {notice && <div role="status" className="border-y border-base-800 px-4 py-2.5 text-sm text-emerald-300">{notice}</div>}
      {error && <div role="alert" className="border-y border-base-800 px-4 py-2.5 text-sm text-rose-300 shadow-[inset_4px_0_0_var(--color-rose-500)]">{error}</div>}

      <section className="border-y border-base-800 p-4 space-y-3" aria-label="구성도 편집">
        <div className="flex flex-wrap items-center gap-3 min-h-9">
          <h2 className="text-sm font-semibold text-base-200">구성도</h2>
          {picked ? (
            <>
              <p role="status" className="text-sm text-base-200">
                <span className="font-semibold text-brand-300">{picked}</span> 선택 — 위에 연결된 장비를 누르세요
                <span className="text-base-500"> (흐린 장비는 아래쪽이라 고를 수 없음, Esc로 취소)</span>
              </p>
              {pickedRow?.parent && (
                <button type="button" onClick={() => { update(picked, { parent: undefined }); setPicked(null) }}
                  className="flex items-center gap-1 h-8 px-3 rounded-lg border border-base-700 text-sm text-base-200 hover:bg-base-800">
                  <CornerLeftUp size={14} /> 연결 끊고 맨 위로
                </button>
              )}
              <button type="button" onClick={() => setPicked(null)} className="h-8 px-3 rounded-lg border border-base-700 text-sm text-base-300 hover:bg-base-800">취소</button>
            </>
          ) : (
            <p className="text-sm text-base-500">장비를 누른 뒤, 그 장비가 연결된 위쪽 장비를 누르세요</p>
          )}
          {dirty && <span className="ml-auto text-sm text-amber-300">저장하지 않은 변경이 있습니다</span>}
        </div>
        {rows.length > placed.length && (
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <span className="text-base-500">구성도에 없는 장비</span>
            {rows.filter(r => !r.zone).map(r => (
              <button key={r.name} type="button" aria-pressed={picked === r.name} onClick={() => setPicked(picked === r.name ? null : r.name)}
                className={cn('h-8 px-3 rounded-lg border text-base-200 hover:bg-base-800', picked === r.name ? 'border-brand-400 ring-1 ring-brand-400' : 'border-base-700 border-dashed')}>
                {r.name}
              </button>
            ))}
          </div>
        )}
        {draft.devices.length === 0 ? (
          <p className="py-8 text-center text-sm text-base-500">구역을 정한 장비가 없습니다. 아래 구역을 만들고 장비의 구역을 정하세요</p>
        ) : (
          <div className="h-[560px] overflow-auto rounded-lg bg-base-950/40">
            <TopologyMap
              layout={draft} lookup={() => ({ state: 'ok', detail: '', to: '' })}
              onPick={pick} picked={picked} blocked={name => !!picked && name !== picked && pickedBlocked.has(name)}
            />
          </div>
        )}
      </section>

      <section className="border-y border-base-800 p-4 space-y-3">
        <h2 className="text-sm font-semibold text-base-200">구역 <span className="font-normal text-base-500">위에서 아래 순서로 그려집니다</span></h2>
        <ol className="flex flex-wrap gap-2">
          {zones.map((z, i) => (
            <li key={z} className="flex items-center gap-1 h-9 pl-3 pr-1 rounded-lg bg-base-800 border border-base-700 text-sm text-base-100">
              <span className="mr-1 text-xs text-base-500 tabular-nums">{i + 1}</span>{z}
              <button type="button" aria-label={`${z} 위로`} disabled={i === 0} onClick={() => moveZone(i, -1)} className="p-1 rounded text-base-400 hover:text-base-100 disabled:opacity-30"><ArrowUp size={14} /></button>
              <button type="button" aria-label={`${z} 아래로`} disabled={i === zones.length - 1} onClick={() => moveZone(i, 1)} className="p-1 rounded text-base-400 hover:text-base-100 disabled:opacity-30"><ArrowDown size={14} /></button>
              <button type="button" aria-label={`${z} 삭제`} onClick={() => removeZone(z)} className="p-1 rounded text-base-400 hover:text-rose-300"><X size={14} /></button>
            </li>
          ))}
          <li>
            <form onSubmit={addZone} className="flex gap-1">
              <input value={newZone} onChange={e => setNewZone(e.target.value)} placeholder="예: DMZ, 서버팜" aria-label="새 구역 이름" className={cn(inputCls, 'w-40')} />
              <button type="submit" className="h-9 px-3 rounded-lg border border-base-700 text-sm text-base-200 hover:bg-base-800"><Plus size={14} /></button>
            </form>
          </li>
        </ol>
      </section>

      <section className="border-y border-base-800">
        <div className="flex flex-wrap items-center justify-between gap-3 px-4 pt-4">
          <h2 className="text-sm font-semibold text-base-200">
            장비 <span className="font-normal text-base-500">구성도에 {placed.length}대, 빠진 장비 {rows.length - placed.length}대</span>
          </h2>
          <form onSubmit={addNode} className="flex gap-1">
            <input value={newNode} onChange={e => setNewNode(e.target.value)} placeholder="감시하지 않는 장비 (예: 인터넷, KT 회선)" aria-label="추가할 장비 이름" className={cn(inputCls, 'w-72')} />
            <button type="submit" className="flex items-center gap-1 h-9 px-3 rounded-lg border border-base-700 text-sm text-base-200 hover:bg-base-800"><Plus size={14} /> 추가</button>
          </form>
        </div>
        <div className="overflow-x-auto p-4">
          <table className="w-full min-w-[860px] text-sm">
            <thead>
              <tr className="text-left text-xs text-base-500">
                <th className="pb-2 font-semibold">장비</th>
                <th className="pb-2 font-semibold w-40">종류</th>
                <th className="pb-2 font-semibold w-40">구역</th>
                <th className="pb-2 font-semibold w-48">위에 연결된 장비</th>
                <th className="pb-2 font-semibold w-48">이중화 짝</th>
                <th className="pb-2 w-10" />
              </tr>
            </thead>
            <tbody className="divide-y divide-base-800">
              {rows.map(r => {
                const blocked = descendants(r.name)
                // 짝은 한쪽에만 적혀 있어도 양쪽 행에 보이게
                const mate = r.pair ?? rows.find(x => x.pair === r.name)?.name ?? ''
                return (
                  <tr key={r.name} className={cn(!r.zone && 'opacity-60')}>
                    <td className="py-2 pr-3">
                      <div className="font-semibold text-base-100">{r.name}</div>
                      <div className="text-xs text-base-500">{r.monitored ? '감시 중' : '표시만'}</div>
                    </td>
                    <td className="py-2 pr-2">
                      <select aria-label={`${r.name} 종류`} value={r.kind} onChange={e => update(r.name, { kind: e.target.value as DeviceKind })} className={inputCls}>
                        {(Object.keys(KIND_LABEL) as DeviceKind[]).map(k => <option key={k} value={k}>{KIND_LABEL[k]}</option>)}
                      </select>
                    </td>
                    <td className="py-2 pr-2">
                      <select aria-label={`${r.name} 구역`} value={r.zone} onChange={e => update(r.name, { zone: e.target.value })} className={inputCls}>
                        <option value="">구성도에서 빼기</option>
                        {zones.map(z => <option key={z} value={z}>{z}</option>)}
                      </select>
                    </td>
                    <td className="py-2 pr-2">
                      <select aria-label={`${r.name} 위에 연결된 장비`} value={r.parent ?? ''} disabled={!r.zone} onChange={e => update(r.name, { parent: e.target.value || undefined })} className={inputCls}>
                        <option value="">없음 (맨 위)</option>
                        {placed.filter(p => !blocked.has(p.name)).map(p => <option key={p.name} value={p.name}>{p.name}</option>)}
                      </select>
                    </td>
                    <td className="py-2 pr-2">
                      <select aria-label={`${r.name} 이중화 짝`} value={mate} disabled={!r.zone} onChange={e => setPair(r.name, e.target.value)} className={inputCls}>
                        <option value="">없음</option>
                        {placed.filter(p => p.name !== r.name && p.zone === r.zone).map(p => <option key={p.name} value={p.name}>{p.name}</option>)}
                      </select>
                    </td>
                    <td className="py-2 text-right">
                      {!r.monitored && (
                        <button type="button" aria-label={`${r.name} 삭제`} onClick={() => setRows(rs => rs.filter(x => x.name !== r.name))} className="p-1.5 rounded text-base-500 hover:text-rose-300">
                          <Trash2 size={15} />
                        </button>
                      )}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </section>

    </div>
  )
}
