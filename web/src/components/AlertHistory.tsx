import { useEffect, useState } from 'react'
import { getAlertHistory, type AlertRecord } from '../api/alert'

interface Props {
  onClose: () => void
}

const SEVERITY_COLOR: Record<string, string> = {
  critical: '#ef4444',
  warning: '#f59e0b',
  info: '#7dd3fc',
}

export default function AlertHistory({ onClose }: Props) {
  const [records, setRecords] = useState<AlertRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    getAlertHistory(100)
      .then(setRecords)
      .catch(() => setError('히스토리를 불러올 수 없습니다. (PostgreSQL 미연결 또는 서버 오류)'))
      .finally(() => setLoading(false))
  }, [])

  return (
    <div style={{
      position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.7)',
      display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000,
    }}>
      <div style={{
        background: '#1e293b', borderRadius: 12, padding: 28, width: 760, maxWidth: '95vw',
        maxHeight: '80vh', display: 'flex', flexDirection: 'column',
        border: '1px solid #334155',
      }}>
        {/* 헤더 */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
          <h2 style={{ color: '#f1f5f9', fontSize: 18, fontWeight: 600, margin: 0 }}>알림 히스토리</h2>
          <button onClick={onClose} style={{ background: 'none', border: 'none', color: '#94a3b8', fontSize: 20, cursor: 'pointer', lineHeight: 1 }}>✕</button>
        </div>

        {/* 내용 */}
        <div style={{ overflowY: 'auto', flex: 1 }}>
          {loading && <p style={{ color: '#94a3b8', textAlign: 'center' }}>불러오는 중...</p>}
          {error && <p style={{ color: '#f87171', textAlign: 'center' }}>{error}</p>}
          {!loading && !error && records.length === 0 && (
            <p style={{ color: '#475569', textAlign: 'center', marginTop: 40 }}>알림 발송 이력이 없습니다.</p>
          )}
          {records.map((r) => (
            <div key={r.id} style={{
              borderBottom: '1px solid #334155', padding: '14px 0',
              display: 'grid', gridTemplateColumns: '140px 80px 80px 1fr', gap: 12, alignItems: 'start',
            }}>
              <span style={{ color: '#64748b', fontSize: 12, paddingTop: 2 }}>
                {new Date(r.sent_at).toLocaleString('ko-KR')}
              </span>
              <span style={{
                background: SEVERITY_COLOR[r.severity] ?? '#475569',
                color: '#fff', fontSize: 11, fontWeight: 600,
                padding: '2px 8px', borderRadius: 4, textAlign: 'center', alignSelf: 'start',
              }}>
                {r.severity.toUpperCase()}
              </span>
              <span style={{ color: '#94a3b8', fontSize: 12, paddingTop: 2 }}>{r.host}</span>
              <div>
                <p style={{ color: '#e2e8f0', fontSize: 13, fontWeight: 500, margin: '0 0 4px' }}>{r.subject}</p>
                <p style={{ color: '#64748b', fontSize: 12, margin: 0, whiteSpace: 'pre-wrap' }}>{r.body}</p>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
