import { useState } from 'react'
import { fetchAssets, upsertAsset, deleteAsset, type Asset } from '../api/asset'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Server, MapPin, Calendar, Edit2, Trash2, Plus, X, Save, RefreshCcw, Info } from 'lucide-react'
import { cn } from '../utils/cn'
import ConfirmDialog from './ConfirmDialog'

const emptyForm = (): Omit<Asset, 'id' | 'updated_at'> => ({
  host_name: '',
  ip: '',
  location: '',
  description: '',
  purchase_date: '',
  warranty_expires: '',
  notes: '',
})

export default function AssetManagement() {
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState<Omit<Asset, 'id' | 'updated_at'> | null>(null)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<{ id: number; name: string } | null>(null)

  const { data: assets = [], isLoading } = useQuery({
    queryKey: ['assets'],
    queryFn: fetchAssets,
  })

  const saveMutation = useMutation({
    mutationFn: upsertAsset,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['assets'] })
      cancel()
    }
  })

  const deleteMutation = useMutation({
    mutationFn: deleteAsset,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['assets'] })
    }
  })

  const startEdit = (asset: Asset) => {
    setEditingId(asset.id)
    setEditing({
      host_name: asset.host_name,
      ip: asset.ip,
      location: asset.location,
      description: asset.description,
      purchase_date: asset.purchase_date,
      warranty_expires: asset.warranty_expires,
      notes: asset.notes,
    })
  }

  const startNew = () => {
    setEditingId(null)
    setEditing(emptyForm())
  }

  const cancel = () => {
    setEditing(null)
    setEditingId(null)
  }

  const handleSave = () => {
    if (!editing || !editing.host_name.trim()) return
    saveMutation.mutate(editing)
  }

  const handleRemove = (id: number, name: string) => {
    setDeleteTarget({ id, name })
  }

  const warrantyStatus = (expires: string) => {
    if (!expires) return null
    const days = Math.ceil((new Date(expires).getTime() - Date.now()) / 86400000)
    if (days < 0) return { label: '만료됨', bg: 'bg-rose-500/15', text: 'text-rose-300' }
    if (days <= 30) return { label: `D-${days}`, bg: 'bg-amber-500/15', text: 'text-amber-300' }
    return { label: '보증 중', bg: 'bg-emerald-500/15', text: 'text-emerald-300' }
  }

  return (
    <div className="space-y-6">
      {/* Page Actions */}
      <div className="flex justify-between items-center bg-slate-900 p-4 rounded-3xl border border-slate-800 shadow-sm">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-indigo-500/10 text-indigo-400 rounded-lg">
            <Server size={20} />
          </div>
          <div>
            <h3 className="font-bold text-slate-100 leading-tight">IT 자산 관리</h3>
            <p className="text-xs text-slate-500 font-medium">등록된 호스트 및 네트워크 장비 목록</p>
          </div>
        </div>
        {!editing && (
          <button 
            className="flex items-center gap-2 px-6 py-2.5 bg-indigo-600 text-white rounded-xl text-sm font-bold hover:bg-indigo-700 transition-all shadow-lg shadow-indigo-500/20"
            onClick={startNew}
          >
            <Plus size={18} />
            새 자산 등록
          </button>
        )}
      </div>

      {/* Editor Form */}
      {editing && (
        <div className="bg-slate-900 rounded-3xl border border-indigo-500/30 shadow-xl overflow-hidden animate-in fade-in slide-in-from-top-4 duration-300">
          <div className="p-6 border-b border-slate-800 bg-slate-800/50 flex items-center justify-between">
            <h3 className="font-bold text-slate-200 flex items-center gap-2 text-lg">
              {editingId ? <Edit2 size={20} className="text-indigo-500" /> : <Plus size={20} className="text-indigo-500" />}
              {editingId ? '자산 정보 편집' : '신규 자산 등록'}
            </h3>
            <button onClick={cancel} className="p-2 text-slate-400 hover:bg-slate-900 rounded-lg hover:text-rose-500 transition-all">
              <X size={20} />
            </button>
          </div>
          
          <div className="p-6 space-y-6">
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
              <FormField label="호스트명 *" value={editing.host_name} onChange={v => setEditing({...editing, host_name: v})} placeholder="서버 식별자" />
              <FormField label="IP 주소" value={editing.ip} onChange={v => setEditing({...editing, ip: v})} placeholder="0.0.0.0" />
              <FormField label="설치 위치" value={editing.location} onChange={v => setEditing({...editing, location: v})} placeholder="예: 2층 전산실" />
              <FormField label="장비 상세 설명" value={editing.description} onChange={v => setEditing({...editing, description: v})} placeholder="모델명 등" />
              <FormField label="도입 일자" type="date" value={editing.purchase_date} onChange={v => setEditing({...editing, purchase_date: v})} />
              <FormField label="보증 만료일" type="date" value={editing.warranty_expires} onChange={v => setEditing({...editing, warranty_expires: v})} />
            </div>
            <FormField label="추가 메모" value={editing.notes} onChange={v => setEditing({...editing, notes: v})} placeholder="기타 특이사항" />
          </div>

          <div className="p-6 bg-slate-800/50 border-t border-slate-800 flex justify-end gap-3">
            <button 
              className="px-6 py-2.5 bg-slate-900 border border-slate-800 text-slate-400 rounded-xl text-sm font-bold hover:bg-slate-800 transition-all"
              onClick={cancel}
            >
              취소
            </button>
            <button 
              className={cn(
                "px-10 py-2.5 rounded-xl text-sm font-bold shadow-lg transition-all flex items-center gap-2",
                saveMutation.isPending ? "bg-slate-700 text-slate-400 cursor-not-allowed" : "bg-indigo-600 text-white hover:bg-indigo-700 shadow-indigo-500/20"
              )}
              onClick={handleSave}
              disabled={saveMutation.isPending}
            >
              {saveMutation.isPending ? <RefreshCcw size={18} className="animate-spin" /> : <Save size={18} />}
              정보 저장하기
            </button>
          </div>
        </div>
      )}

      {/* Asset Table */}
      <div className="bg-slate-900 rounded-3xl border border-slate-800 shadow-sm overflow-hidden">
        {isLoading ? (
          <div className="flex flex-col items-center justify-center py-32 text-slate-400 animate-pulse">
            <RefreshCcw size={48} className="mb-4 opacity-20 animate-spin" />
            <p className="font-medium">자산 목록을 불러오는 중...</p>
          </div>
        ) : assets.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-32 text-slate-400 text-center px-6">
            <Info size={48} className="mb-4 opacity-20" />
            <p className="font-medium">등록된 자산이 없습니다.</p>
            <p className="text-xs opacity-70 mt-1">상단의 '새 자산 등록' 버튼을 눌러 정보를 추가하세요.</p>
          </div>
        ) : (
          <>
          {/* 데스크탑: 테이블 (모바일은 아래 카드뷰 — AlertHistory와 동일 분기 패턴) */}
          <div className="hidden md:block overflow-x-auto">
            <table className="w-full text-left border-collapse">
              <thead>
                <tr className="bg-slate-800/50 border-b border-slate-800">
                  <th className="px-6 py-4 text-[10px] font-bold text-slate-400 uppercase tracking-wider">호스트 정보</th>
                  <th className="px-6 py-4 text-[10px] font-bold text-slate-400 uppercase tracking-wider">위치 / 설명</th>
                  <th className="px-6 py-4 text-[10px] font-bold text-slate-400 uppercase tracking-wider">도입일</th>
                  <th className="px-6 py-4 text-[10px] font-bold text-slate-400 uppercase tracking-wider">보증 현황</th>
                  <th className="px-6 py-4 text-[10px] font-bold text-slate-400 uppercase tracking-wider text-right">관리</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-800">
                {assets.map(a => {
                  const ws = warrantyStatus(a.warranty_expires)
                  return (
                    <tr key={a.id} className="hover:bg-slate-800/30 transition-colors group">
                      <td className="px-6 py-4">
                        <div className="font-bold text-slate-200 group-hover:text-indigo-400 transition-colors">{a.host_name}</div>
                        <div className="text-[11px] font-bold text-slate-400 font-mono tracking-tight">{a.ip || '0.0.0.0'}</div>
                      </td>
                      <td className="px-6 py-4">
                        <div className="flex items-center gap-1.5 text-xs font-bold text-slate-400">
                          <MapPin size={10} className="text-slate-400" />
                          {a.location || '-'}
                        </div>
                        <div className="text-[11px] text-slate-400 mt-0.5 line-clamp-1">{a.description || '상세 설명 없음'}</div>
                      </td>
                      <td className="px-6 py-4">
                        <div className="flex items-center gap-1.5 text-xs font-semibold text-slate-500">
                          <Calendar size={10} className="text-slate-400" />
                          {a.purchase_date || '-'}
                        </div>
                      </td>
                      <td className="px-6 py-4">
                        {ws ? (
                          <div className="flex items-center gap-2">
                            <span className="text-xs font-semibold text-slate-500">{a.warranty_expires}</span>
                            <span className={cn(
                              "px-2 py-0.5 rounded text-[10px] font-bold uppercase tracking-tight",
                              ws.bg, ws.text
                            )}>
                              {ws.label}
                            </span>
                          </div>
                        ) : (
                          <span className="text-xs text-slate-400 italic">설정 안 됨</span>
                        )}
                      </td>
                      <td className="px-6 py-4">
                        <div className="flex justify-end gap-2">
                          <button 
                            className="p-2 bg-slate-800 text-slate-400 hover:bg-indigo-500/10 hover:text-indigo-400 rounded-lg transition-all"
                            onClick={() => startEdit(a)}
                            title="정보 수정"
                            aria-label={`${a.host_name} 정보 수정`}
                          >
                            <Edit2 size={16} />
                          </button>
                          <button 
                            className="p-2 bg-slate-800 text-slate-400 hover:bg-rose-500/10 hover:text-rose-400 rounded-lg transition-all"
                            onClick={() => handleRemove(a.id, a.host_name)}
                            title="정보 삭제"
                            aria-label={`${a.host_name} 정보 삭제`}
                          >
                            <Trash2 size={16} />
                          </button>
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>

          {/* 모바일: 카드뷰 — 5컬럼 테이블 가로 스크롤 대신 세로 스택 */}
          <div className="md:hidden divide-y divide-slate-800">
            {assets.map(a => {
              const ws = warrantyStatus(a.warranty_expires)
              return (
                <div key={a.id} className="p-4 space-y-2">
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0">
                      <div className="font-bold text-slate-200">{a.host_name}</div>
                      <div className="text-[11px] font-bold text-slate-400 font-mono">{a.ip || '0.0.0.0'}</div>
                    </div>
                    <div className="flex gap-2 shrink-0">
                      <button
                        className="p-2 bg-slate-800 text-slate-400 hover:bg-indigo-500/10 hover:text-indigo-400 rounded-lg transition-all"
                        onClick={() => startEdit(a)}
                        aria-label={`${a.host_name} 정보 수정`}
                      >
                        <Edit2 size={16} />
                      </button>
                      <button
                        className="p-2 bg-slate-800 text-slate-400 hover:bg-rose-500/10 hover:text-rose-400 rounded-lg transition-all"
                        onClick={() => handleRemove(a.id, a.host_name)}
                        aria-label={`${a.host_name} 정보 삭제`}
                      >
                        <Trash2 size={16} />
                      </button>
                    </div>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs font-bold text-slate-400">
                    <MapPin size={10} />
                    {a.location || '-'}
                    <span className="font-normal text-slate-500 truncate">· {a.description || '상세 설명 없음'}</span>
                  </div>
                  <div className="flex items-center justify-between text-xs">
                    <span className="flex items-center gap-1.5 font-semibold text-slate-500">
                      <Calendar size={10} /> {a.purchase_date || '도입일 미설정'}
                    </span>
                    {ws ? (
                      <span className={cn('px-2 py-0.5 rounded text-[10px] font-bold uppercase', ws.bg, ws.text)}>
                        {ws.label}
                      </span>
                    ) : (
                      <span className="text-slate-400 italic">보증 미설정</span>
                    )}
                  </div>
                </div>
              )
            })}
          </div>
          </>
        )}
      </div>

      <ConfirmDialog
        open={deleteTarget !== null}
        title="자산 정보 삭제"
        message={`'${deleteTarget?.name}' 자산 정보를 삭제하시겠습니까?`}
        onConfirm={() => {
          if (deleteTarget) deleteMutation.mutate(deleteTarget.id)
          setDeleteTarget(null)
        }}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  )
}

function FormField({ label, value, onChange, type = "text", placeholder = "" }: { label: string, value: string, onChange: (v: string) => void, type?: string, placeholder?: string }) {
  return (
    <div className="space-y-1.5">
      <label className="text-[10px] font-bold text-slate-400 uppercase ml-1 tracking-widest">{label}</label>
      <input
        type={type}
        className="w-full bg-slate-800 border border-slate-800 rounded-xl px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition-all font-medium text-slate-400"
        value={value || ''}
        onChange={e => onChange(e.target.value)}
        placeholder={placeholder}
      />
    </div>
  )
}
