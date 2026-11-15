import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Download, FileUp, Upload, X } from 'lucide-react'
import { getSNMPDevices } from '../api/snmp'
import { fetchAssets, unitRange } from '../api/asset'
import { fetchRacks } from '../api/rack'
import { applyImport, planImport, KIND_NAME, TEMPLATE_EXAMPLE, TEMPLATE_HEADER, type ApplyResult, type Plan } from '../api/bulkImport'
import type { TopologyLayout } from '../api/topology'
import { decodeText, parseTable, toCSV } from '../utils/csv'
import { cn } from '../utils/cn'

const BTN = 'flex items-center gap-1.5 h-9 px-3 rounded-lg border border-base-700 text-sm text-base-200 hover:bg-base-800 disabled:opacity-50'

function downloadTemplate() {
  const url = URL.createObjectURL(new Blob([toCSV([TEMPLATE_HEADER, ...TEMPLATE_EXAMPLE])], { type: 'text/csv;charset=utf-8' }))
  const a = document.createElement('a')
  a.href = url
  a.download = 'owlmon-장비-등록-양식.csv'
  a.click()
  URL.revokeObjectURL(url)
}

// 구성도 편집기는 저장할 때마다 새로 그려지므로 그 바깥(페이지)에 띄워 등록 결과가 사라지지 않게 한다
export default function BulkImport({ layout, servers, unsaved, onClose }: { layout: TopologyLayout; servers: string[]; unsaved: boolean; onClose: () => void }) {
  const qc = useQueryClient()
  const { data: snmp, isSuccess: snmpReady, isError: snmpError } = useQuery({ queryKey: ['snmpDevices'], queryFn: getSNMPDevices })
  const { data: assets, isSuccess: assetsReady, isError: assetsError } = useQuery({ queryKey: ['assets'], queryFn: fetchAssets })
  const { data: racks, isSuccess: racksReady, isError: racksError } = useQuery({ queryKey: ['racks'], queryFn: fetchRacks })
  const [text, setText] = useState('')
  const [fileName, setFileName] = useState('')
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState<{ results: ApplyResult[]; layoutError?: string } | null>(null)

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape' && !busy) onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [busy, onClose])

  const ready = snmpReady && assetsReady && racksReady
  // 기존 등록과 비교해야 중복을 잡을 수 있으니, 목록을 못 받으면 미리보기를 만들지 않는다
  const plan: Plan | null = ready && text.trim() ? planImport(parseTable(text), { layout, snmp: snmp ?? [], assets: assets ?? [], servers, racks: racks ?? [] }) : null
  const errors = plan?.rows.filter(r => r.errors.length).length ?? 0
  const counts = plan && {
    snmp: plan.rows.filter(r => r.snmp === 'add').length,
    asset: plan.rows.filter(r => r.asset).length,
  }

  const onFile = async (f: File | undefined) => {
    if (!f) return
    setDone(null)
    setFileName(f.name)
    setText(decodeText(await f.arrayBuffer()))
  }

  const apply = async () => {
    if (!plan || !assets) return
    setBusy(true)
    try {
      setDone(await applyImport(plan, assets))
    } finally {
      setBusy(false)
      qc.invalidateQueries({ queryKey: ['snmpDevices'] })
      qc.invalidateQueries({ queryKey: ['snmpStatus'] })
      qc.invalidateQueries({ queryKey: ['assets'] })
      qc.invalidateQueries({ queryKey: ['racks'] })
      qc.invalidateQueries({ queryKey: ['topologyLayout'] })
    }
  }

  const failed = done?.results.filter(r => !r.ok) ?? []

  return (
    // 바깥을 눌러도 닫지 않음 — 표를 드래그하다 바깥에서 놓으면 붙여넣은 내용·등록 결과가 사라짐 (X·Esc로만 닫기)
    <div className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/60 p-4 sm:p-8">
    <section role="dialog" aria-modal="true" aria-label="장비 한 번에 등록"
      className="w-full max-w-6xl rounded-xl border border-base-800 bg-base-900 shadow-2xl p-5 space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <h2 className="text-sm font-semibold text-base-200">장비 한 번에 등록</h2>
        <p className="text-sm text-base-500">스위치·방화벽은 IP를 적으면 감시 대상으로 등록되고, 위치·랙·용도는 장비 목록에, 상위 장비는 구성도에 반영됩니다</p>
        <button type="button" onClick={onClose} disabled={busy} aria-label="닫기" className="ml-auto p-1.5 rounded-lg text-base-400 hover:text-base-100 hover:bg-base-800 disabled:opacity-50"><X size={16} /></button>
      </div>
      {unsaved && !done && <p className="text-sm text-amber-300">구성도에 저장하지 않은 변경이 있습니다 — 여기서 등록하면 그 변경은 사라집니다</p>}

      <ol className="flex flex-wrap items-center gap-3 text-sm">
        <li className="flex items-center gap-2">
          <span className="text-base-500">1</span>
          <button type="button" onClick={downloadTemplate} className={BTN}><Download size={14} /> 양식 받기</button>
        </li>
        <li className="flex items-center gap-2">
          <span className="text-base-500">2</span>
          <label className={cn(BTN, 'cursor-pointer')}>
            <FileUp size={14} /> CSV 파일 고르기
            <input type="file" accept=".csv,.tsv,.txt,text/csv" className="sr-only" onChange={e => { onFile(e.target.files?.[0]); e.target.value = '' }} />
          </label>
          <span className="text-base-500">또는 엑셀에서 표를 복사해 아래에 붙여넣기</span>
        </li>
      </ol>

      <textarea
        value={text}
        onChange={e => { setText(e.target.value); setFileName(''); setDone(null) }}
        rows={text ? 4 : 3}
        placeholder={TEMPLATE_HEADER.join('\t')}
        aria-label="장비 목록 붙여넣기"
        className="w-full px-3 py-2 rounded-lg bg-base-800 border border-base-700 text-sm text-base-100 font-mono focus:outline-none focus:border-brand-500"
      />
      {fileName && <p className="text-xs text-base-500">{fileName}에서 읽었습니다</p>}

      {(snmpError || assetsError || racksError) && <p role="alert" className="text-sm text-rose-300">기존 장비·자산 목록을 불러오지 못해 중복을 확인할 수 없습니다. 잠시 뒤 다시 열어 주세요</p>}
      {plan?.headerError && <p role="alert" className="text-sm text-rose-300">{plan.headerError}</p>}

      {plan && !plan.headerError && plan.rows.length > 0 && (
        <>
          <div className="overflow-x-auto">
            <table className="w-full min-w-[1000px] text-sm">
              <thead>
                <tr className="text-left text-xs text-base-500 border-b border-base-800">
                  <th className="py-2 pr-3 font-medium w-12">줄</th>
                  <th className="py-2 pr-3 font-medium">이름</th>
                  <th className="py-2 pr-3 font-medium w-20">종류</th>
                  <th className="py-2 pr-3 font-medium w-32">IP</th>
                  <th className="py-2 pr-3 font-medium">상위 장비</th>
                  <th className="py-2 pr-3 font-medium w-24">구역</th>
                  <th className="py-2 pr-3 font-medium">위치</th>
                  <th className="py-2 pr-3 font-medium w-28">랙</th>
                  <th className="py-2 pr-3 font-medium">용도</th>
                  <th className="py-2 font-medium">할 일 · 확인할 점</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-base-800/70">
                {plan.rows.map(r => {
                  const res = done?.results.find(x => x.line === r.line)
                  const todo = [r.snmp === 'add' && '감시 등록', r.snmp === 'exists' && '이미 감시 중', r.snmp === 'community' && '이미 감시 중 · 커뮤니티 변경', r.asset && '장비 정보 저장'].filter(Boolean).join(' · ')
                  return (
                    <tr key={r.line} className={cn(r.errors.length > 0 && 'bg-rose-500/5')}>
                      <td className="py-2 pr-3 text-base-500 tabular-nums">{r.line}</td>
                      <td className="py-2 pr-3 font-semibold text-base-100">{r.name || '—'}</td>
                      <td className="py-2 pr-3 text-base-300">{r.kind ? KIND_NAME[r.kind] : '—'}</td>
                      <td className="py-2 pr-3 text-base-300 tabular-nums">{r.ip}</td>
                      <td className="py-2 pr-3 text-base-300">{r.parent || <span className="text-base-500">맨 위</span>}</td>
                      <td className="py-2 pr-3 text-base-300">{r.zone}</td>
                      <td className="py-2 pr-3 text-base-300">{r.location}</td>
                      <td className="py-2 pr-3 text-base-300 whitespace-nowrap">{r.rack && r.unit >= 1 ? `${r.rack} ${unitRange(r.unit, r.size)}` : r.rack}</td>
                      <td className="py-2 pr-3 text-base-300">{r.purpose}</td>
                      <td className="py-2 text-base-300">
                        {res ? (
                          <span className={res.ok && !done?.layoutError ? 'text-emerald-300' : 'text-rose-300'}>
                            {[...res.messages, done?.layoutError ? '구성도 저장 안 됨' : '구성도 반영'].join(' · ')}
                          </span>
                        ) : (
                          <>
                            {todo && <span>{todo}</span>}
                            {r.errors.map(e => <p key={e} className="text-rose-300">{e}</p>)}
                            {r.warnings.map(w => <p key={w} className="text-xs text-amber-300">{w}</p>)}
                          </>
                        )}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>

          <div className="flex flex-wrap items-center gap-3">
            {done ? (
              <p role="status" className={cn('text-sm', failed.length || done.layoutError ? 'text-rose-300' : 'text-emerald-300')}>
                {done.layoutError ? `구성도 저장 실패: ${done.layoutError}` : '구성도에 반영했습니다.'}
                {failed.length > 0 ? ` ${failed.length}줄은 일부 실패했습니다 — 표에서 확인하세요` : !done.layoutError && ' 상황판에도 바로 보입니다'}
              </p>
            ) : (
              <p className="text-sm text-base-400">
                {plan.rows.length}대 · 감시 등록 {counts!.snmp} · 장비 정보 저장 {counts!.asset} · 구성도 반영 {plan.rows.length}
                {errors > 0 && <span className="text-rose-300"> · 오류 {errors}줄 — 고친 뒤 다시 올려 주세요</span>}
              </p>
            )}
            <button
              type="button" onClick={apply} disabled={busy || errors > 0 || !!done}
              className="ml-auto flex items-center gap-2 px-4 py-2 rounded-lg bg-brand-600 hover:bg-brand-500 text-white text-sm font-semibold disabled:opacity-50"
            >
              <Upload size={16} /> {busy ? '등록 중…' : `${plan.rows.length}대 등록하기`}
            </button>
          </div>
        </>
      )}
      {!ready && !(snmpError || assetsError || racksError) && <p className="text-sm text-base-500">기존 장비 목록을 불러오는 중…</p>}
    </section>
    </div>
  )
}
