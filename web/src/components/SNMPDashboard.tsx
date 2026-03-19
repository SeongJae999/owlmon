import { useEffect, useState } from 'react'
import {
  getSNMPDevices, getSNMPStatus, addSNMPDevice, deleteSNMPDevice,
  type SNMPDevice, type DeviceStatus,
} from '../api/snmp'

function formatBps(bps: number): string {
  if (bps >= 1024 * 1024) return `${(bps / 1024 / 1024).toFixed(1)} MB/s`
  if (bps >= 1024) return `${(bps / 1024).toFixed(1)} KB/s`
  return `${bps.toFixed(0)} B/s`
}

function formatUptime(sec: number): string {
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  if (d > 0) return `${d}일 ${h}시간`
  if (h > 0) return `${h}시간 ${m}분`
  return `${m}분`
}

function DeviceCard({ status, onDelete }: { status: DeviceStatus; onDelete: () => void }) {
  const activeIfs = status.Interfaces?.filter(i => i.OperUp) ?? []
  const downIfs = status.Interfaces?.filter(i => !i.OperUp) ?? []

  return (
    <div style={{
      background: '#1e293b', borderRadius: 12, padding: '20px 24px',
      border: `1px solid ${status.Up ? '#334155' : '#ef4444'}`,
    }}>
      {/* 헤더 */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 16 }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{
              width: 8, height: 8, borderRadius: '50%', flexShrink: 0,
              background: status.Up ? '#22c55e' : '#ef4444',
            }} />
            <span style={{ color: '#e2e8f0', fontWeight: 700, fontSize: 15 }}>{status.Device.Name}</span>
          </div>
          <div style={{ color: '#475569', fontSize: 12, marginTop: 3, marginLeft: 16 }}>
            {status.Device.IP}
            {status.Up && status.UptimeSec > 0 && (
              <span style={{ marginLeft: 8 }}>· 가동 {formatUptime(status.UptimeSec)}</span>
            )}
          </div>
        </div>
        <button
          onClick={onDelete}
          style={{ background: 'none', border: 'none', color: '#475569', cursor: 'pointer', fontSize: 16, padding: 4 }}
          title="장치 삭제"
        >✕</button>
      </div>

      {!status.Up && (
        <div style={{ color: '#ef4444', fontSize: 13, marginBottom: 12 }}>
          응답 없음 — 장비 오프라인 또는 SNMP 미설정
        </div>
      )}

      {status.Up && status.Interfaces && status.Interfaces.length > 0 && (
        <div>
          {/* 요약 */}
          <div style={{ display: 'flex', gap: 16, marginBottom: 12 }}>
            <span style={{ color: '#22c55e', fontSize: 12 }}>● UP {activeIfs.length}개</span>
            {downIfs.length > 0 && (
              <span style={{ color: '#ef4444', fontSize: 12 }}>● DOWN {downIfs.length}개</span>
            )}
          </div>

          {/* 활성 인터페이스 트래픽 */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            {activeIfs.filter(i => i.InBps > 0 || i.OutBps > 0).slice(0, 6).map(iface => (
              <div key={iface.Index} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span style={{ color: '#64748b', fontSize: 12, width: 120, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {iface.Name}
                </span>
                <div style={{ display: 'flex', gap: 12 }}>
                  <span style={{ color: '#7dd3fc', fontSize: 12 }}>↓ {formatBps(iface.InBps)}</span>
                  <span style={{ color: '#f472b6', fontSize: 12 }}>↑ {formatBps(iface.OutBps)}</span>
                </div>
              </div>
            ))}
            {activeIfs.filter(i => i.InBps === 0 && i.OutBps === 0).length > 0 && (
              <div style={{ color: '#334155', fontSize: 11 }}>
                트래픽 측정 중... (첫 폴링 후 표시)
              </div>
            )}
          </div>
        </div>
      )}

      <div style={{ color: '#1e3a5f', fontSize: 10, marginTop: 12 }}>
        {status.CollectedAt ? new Date(status.CollectedAt).toLocaleTimeString('ko-KR') : ''}
      </div>
    </div>
  )
}

function AddDeviceForm({ onAdd }: { onAdd: () => void }) {
  const [form, setForm] = useState({ Name: '', IP: '', Community: 'public', Port: 161 })
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!form.Name || !form.IP) { setError('이름과 IP를 입력하세요'); return }
    setLoading(true); setError('')
    try {
      await addSNMPDevice(form)
      setForm({ Name: '', IP: '', Community: 'public', Port: 161 })
      onAdd()
    } catch {
      setError('장치 추가 실패. IP와 Community String을 확인하세요.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} style={{
      background: '#1e293b', borderRadius: 12, padding: '20px 24px',
      border: '1px dashed #334155', display: 'flex', flexDirection: 'column', gap: 12,
    }}>
      <div style={{ color: '#94a3b8', fontSize: 13, fontWeight: 600 }}>장치 추가</div>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
        <input
          placeholder="장치 이름 (예: 교무실 스위치)"
          value={form.Name}
          onChange={e => setForm(f => ({ ...f, Name: e.target.value }))}
          style={inputStyle}
        />
        <input
          placeholder="IP 주소 (예: 192.168.1.1)"
          value={form.IP}
          onChange={e => setForm(f => ({ ...f, IP: e.target.value }))}
          style={inputStyle}
        />
        <input
          placeholder="Community String"
          value={form.Community}
          onChange={e => setForm(f => ({ ...f, Community: e.target.value }))}
          style={inputStyle}
        />
        <input
          type="number"
          placeholder="포트 (기본 161)"
          value={form.Port}
          onChange={e => setForm(f => ({ ...f, Port: Number(e.target.value) }))}
          style={inputStyle}
        />
      </div>
      {error && <div style={{ color: '#ef4444', fontSize: 12 }}>{error}</div>}
      <button type="submit" disabled={loading} style={{
        background: '#0ea5e9', border: 'none', color: '#fff',
        padding: '8px 20px', borderRadius: 7, cursor: 'pointer', fontSize: 13, fontWeight: 600,
        alignSelf: 'flex-start', opacity: loading ? 0.6 : 1,
      }}>
        {loading ? '추가 중...' : '추가'}
      </button>
    </form>
  )
}

const inputStyle: React.CSSProperties = {
  background: '#0f1117', border: '1px solid #334155', color: '#e2e8f0',
  padding: '7px 12px', borderRadius: 6, fontSize: 13, width: '100%', boxSizing: 'border-box',
}

export default function SNMPDashboard() {
  const [statuses, setStatuses] = useState<DeviceStatus[]>([])
  const [devices, setDevices] = useState<SNMPDevice[]>([])
  const [showAdd, setShowAdd] = useState(false)

  async function refresh() {
    const [devs, stats] = await Promise.all([
      getSNMPDevices().catch(() => [] as SNMPDevice[]),
      getSNMPStatus().catch(() => [] as DeviceStatus[]),
    ])
    setDevices(devs)
    setStatuses(stats)
  }

  useEffect(() => {
    refresh()
    const t = setInterval(refresh, 60_000)
    return () => clearInterval(t)
  }, [])

  async function handleDelete(id: number) {
    await deleteSNMPDevice(id)
    refresh()
  }

  // 장치는 있지만 아직 상태가 없는 경우도 표시
  const statusMap = new Map(statuses.map(s => [s.Device.ID, s]))

  return (
    <div style={{ marginBottom: 32 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
        <h2 style={{ color: '#94a3b8', fontSize: 13, fontWeight: 600, letterSpacing: '0.05em', textTransform: 'uppercase', margin: 0 }}>
          네트워크 장비 (SNMP)
        </h2>
        <button
          onClick={() => setShowAdd(v => !v)}
          style={{ background: '#1e293b', border: '1px solid #334155', color: '#94a3b8', padding: '5px 14px', borderRadius: 6, cursor: 'pointer', fontSize: 12 }}
        >
          {showAdd ? '닫기' : '+ 장치 추가'}
        </button>
      </div>

      {showAdd && <div style={{ marginBottom: 16 }}><AddDeviceForm onAdd={() => { setShowAdd(false); refresh() }} /></div>}

      {devices.length === 0 && !showAdd && (
        <div style={{ color: '#334155', fontSize: 13, padding: '20px 0' }}>
          등록된 네트워크 장비가 없습니다. 장치 추가 버튼으로 스위치/라우터를 등록하세요.
        </div>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(320px, 1fr))', gap: 16 }}>
        {devices.map(dev => {
          const status = statusMap.get(dev.ID)
          if (!status) return (
            <div key={dev.ID} style={{ background: '#1e293b', borderRadius: 12, padding: '20px 24px', border: '1px solid #334155' }}>
              <div style={{ color: '#e2e8f0', fontWeight: 700 }}>{dev.Name}</div>
              <div style={{ color: '#475569', fontSize: 12 }}>{dev.IP} — 폴링 중...</div>
            </div>
          )
          return (
            <DeviceCard
              key={dev.ID}
              status={status}
              onDelete={() => handleDelete(dev.ID)}
            />
          )
        })}
      </div>
    </div>
  )
}
