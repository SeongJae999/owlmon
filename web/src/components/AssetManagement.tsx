import { useEffect, useState } from 'react'
import { fetchAssets, upsertAsset, deleteAsset, type Asset } from '../api/asset'

const emptyForm = (): Omit<Asset, 'id' | 'updated_at'> => ({
  host_name: '',
  ip: '',
  location: '',
  description: '',
  purchase_date: '',
  warranty_expires: '',
  notes: '',
})

export default function AssetManagement({ onClose }: { onClose: () => void }) {
  const [assets, setAssets] = useState<Asset[]>([])
  const [editing, setEditing] = useState<Omit<Asset, 'id' | 'updated_at'> | null>(null)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  const load = async () => {
    try {
      setAssets(await fetchAssets())
    } catch {
      setError('자산 목록 로드 실패')
    }
  }

  useEffect(() => { load() }, [])

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
    setError('')
  }

  const startNew = () => {
    setEditingId(null)
    setEditing(emptyForm())
    setError('')
  }

  const cancel = () => {
    setEditing(null)
    setEditingId(null)
    setError('')
  }

  const save = async () => {
    if (!editing) return
    if (!editing.host_name.trim()) { setError('호스트명은 필수입니다'); return }
    setSaving(true)
    try {
      await upsertAsset(editing)
      setEditing(null)
      setEditingId(null)
      await load()
    } catch (e: any) {
      setError(e.message)
    } finally {
      setSaving(false)
    }
  }

  const remove = async (id: number) => {
    if (!confirm('이 자산 정보를 삭제하시겠습니까?')) return
    try {
      await deleteAsset(id)
      await load()
    } catch {
      setError('삭제 실패')
    }
  }

  // 보증 만료 임박 여부 (30일 이내)
  const warrantyStatus = (expires: string) => {
    if (!expires) return null
    const days = Math.ceil((new Date(expires).getTime() - Date.now()) / 86400000)
    if (days < 0) return { label: '만료됨', color: '#ef4444' }
    if (days <= 30) return { label: `D-${days}`, color: '#f59e0b' }
    return null
  }

  const field = (label: string, key: keyof typeof emptyForm, type = 'text', placeholder = '') => (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
      <label style={{ color: '#94a3b8', fontSize: 12 }}>{label}</label>
      <input
        type={type}
        value={(editing as any)?.[key] ?? ''}
        placeholder={placeholder}
        onChange={e => setEditing(prev => prev ? { ...prev, [key]: e.target.value } : prev)}
        style={{
          background: '#0f1117', border: '1px solid #334155', color: '#e2e8f0',
          borderRadius: 6, padding: '6px 10px', fontSize: 13, width: '100%', boxSizing: 'border-box',
        }}
      />
    </div>
  )

  return (
    <div style={{
      position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.7)',
      display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100,
    }} onClick={e => e.target === e.currentTarget && onClose()}>
      <div style={{
        background: '#1e293b', borderRadius: 12, padding: 28, width: '100%', maxWidth: 760,
        maxHeight: '85vh', overflowY: 'auto', border: '1px solid #334155',
      }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
          <h2 style={{ color: '#e2e8f0', fontSize: 18, fontWeight: 700, margin: 0 }}>자산 관리</h2>
          <div style={{ display: 'flex', gap: 8 }}>
            <button onClick={startNew} style={{
              background: '#0ea5e9', color: '#fff', border: 'none',
              borderRadius: 6, padding: '6px 14px', cursor: 'pointer', fontSize: 13,
            }}>
              + 새 자산
            </button>
            <button onClick={onClose} style={{
              background: 'none', border: '1px solid #334155', color: '#64748b',
              borderRadius: 6, padding: '6px 14px', cursor: 'pointer', fontSize: 13,
            }}>
              닫기
            </button>
          </div>
        </div>

        {error && (
          <div style={{ background: '#450a0a', border: '1px solid #ef4444', borderRadius: 6, padding: '8px 12px', color: '#fca5a5', fontSize: 13, marginBottom: 16 }}>
            {error}
          </div>
        )}

        {/* 편집 폼 */}
        {editing && (
          <div style={{ background: '#0f1117', border: '1px solid #334155', borderRadius: 8, padding: 20, marginBottom: 20 }}>
            <h3 style={{ color: '#94a3b8', fontSize: 13, fontWeight: 600, margin: '0 0 16px', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
              {editingId ? '자산 편집' : '새 자산 등록'}
            </h3>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              {field('호스트명 *', 'host_name', 'text', '모니터링 호스트명과 동일하게')}
              {field('IP 주소', 'ip', 'text', '192.168.1.10')}
              {field('위치', 'location', 'text', '2층 서버실')}
              {field('장비 설명', 'description', 'text', '메인 파일 서버')}
              {field('도입일', 'purchase_date', 'date')}
              {field('보증 만료일', 'warranty_expires', 'date')}
            </div>
            <div style={{ marginTop: 12 }}>
              {field('메모', 'notes', 'text', '추가 정보')}
            </div>
            <div style={{ display: 'flex', gap: 8, marginTop: 16, justifyContent: 'flex-end' }}>
              <button onClick={cancel} style={{
                background: 'none', border: '1px solid #334155', color: '#64748b',
                borderRadius: 6, padding: '6px 14px', cursor: 'pointer', fontSize: 13,
              }}>
                취소
              </button>
              <button onClick={save} disabled={saving} style={{
                background: '#0ea5e9', color: '#fff', border: 'none',
                borderRadius: 6, padding: '6px 14px', cursor: 'pointer', fontSize: 13,
                opacity: saving ? 0.7 : 1,
              }}>
                {saving ? '저장 중...' : '저장'}
              </button>
            </div>
          </div>
        )}

        {/* 자산 목록 */}
        {assets.length === 0 ? (
          <div style={{ color: '#475569', textAlign: 'center', padding: '40px 0', fontSize: 14 }}>
            등록된 자산이 없습니다. 새 자산을 추가하세요.
          </div>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
            <thead>
              <tr style={{ borderBottom: '1px solid #334155' }}>
                {['호스트명', 'IP', '위치', '설명', '도입일', '보증만료'].map(h => (
                  <th key={h} style={{ color: '#475569', fontWeight: 600, padding: '8px 10px', textAlign: 'left', fontSize: 11, textTransform: 'uppercase' }}>
                    {h}
                  </th>
                ))}
                <th />
              </tr>
            </thead>
            <tbody>
              {assets.map(a => {
                const ws = warrantyStatus(a.warranty_expires)
                return (
                  <tr key={a.id} style={{ borderBottom: '1px solid #1e293b' }}>
                    <td style={{ padding: '10px', color: '#e2e8f0', fontWeight: 600 }}>{a.host_name}</td>
                    <td style={{ padding: '10px', color: '#94a3b8' }}>{a.ip || '-'}</td>
                    <td style={{ padding: '10px', color: '#94a3b8' }}>{a.location || '-'}</td>
                    <td style={{ padding: '10px', color: '#94a3b8' }}>{a.description || '-'}</td>
                    <td style={{ padding: '10px', color: '#94a3b8' }}>{a.purchase_date || '-'}</td>
                    <td style={{ padding: '10px' }}>
                      <span style={{ color: ws ? ws.color : '#94a3b8' }}>
                        {a.warranty_expires || '-'}
                        {ws && <span style={{ marginLeft: 6, fontSize: 11, fontWeight: 700 }}>[{ws.label}]</span>}
                      </span>
                    </td>
                    <td style={{ padding: '10px', display: 'flex', gap: 6, justifyContent: 'flex-end' }}>
                      <button onClick={() => startEdit(a)} style={{
                        background: '#1e293b', border: '1px solid #334155', color: '#94a3b8',
                        borderRadius: 4, padding: '3px 8px', cursor: 'pointer', fontSize: 12,
                      }}>
                        편집
                      </button>
                      <button onClick={() => remove(a.id)} style={{
                        background: 'none', border: '1px solid #7f1d1d', color: '#ef4444',
                        borderRadius: 4, padding: '3px 8px', cursor: 'pointer', fontSize: 12,
                      }}>
                        삭제
                      </button>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
