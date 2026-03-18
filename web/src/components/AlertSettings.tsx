import { useEffect, useState } from 'react'
import { getAlertConfig, setAlertConfig, type AlertConfig } from '../api/alert'

interface Props {
  onClose: () => void
}

export default function AlertSettings({ onClose }: Props) {
  const [cfg, setCfg] = useState<AlertConfig | null>(null)
  const [recipientInput, setRecipientInput] = useState('')
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    getAlertConfig().then(setCfg).catch(() => setError('설정을 불러오지 못했습니다.'))
  }, [])

  if (!cfg) return (
    <div style={styles.overlay}>
      <div style={styles.modal}>
        <p style={{ color: '#94a3b8' }}>{error || '불러오는 중...'}</p>
      </div>
    </div>
  )

  const addRecipient = () => {
    const email = recipientInput.trim()
    if (!email) return
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
      setError('올바른 이메일 형식이 아닙니다.')
      return
    }
    if (!cfg.recipients.includes(email)) {
      setCfg({ ...cfg, recipients: [...cfg.recipients, email] })
    }
    setRecipientInput('')
    setError('')
  }

  const removeRecipient = (email: string) => {
    setCfg({ ...cfg, recipients: cfg.recipients.filter(r => r !== email) })
  }

  const handleSave = async () => {
    setSaving(true)
    setError('')
    try {
      await setAlertConfig(cfg)
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    } catch {
      setError('저장에 실패했습니다.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div style={styles.overlay} onClick={onClose}>
      <div style={styles.modal} onClick={e => e.stopPropagation()}>
        <div style={styles.header}>
          <h2 style={styles.title}>알림 설정</h2>
          <button style={styles.closeBtn} onClick={onClose}>✕</button>
        </div>

        {/* 알림 활성화 */}
        <div style={styles.row}>
          <label style={styles.label}>이메일 알림</label>
          <label style={styles.toggle}>
            <input type="checkbox" checked={cfg.enabled}
              onChange={e => setCfg({ ...cfg, enabled: e.target.checked })} />
            <span style={{ marginLeft: 8, color: cfg.enabled ? '#86efac' : '#64748b' }}>
              {cfg.enabled ? '활성화' : '비활성화'}
            </span>
          </label>
        </div>

        {/* 수신자 */}
        <div style={styles.section}>
          <label style={styles.label}>수신자</label>
          <div style={styles.tagList}>
            {cfg.recipients.map(r => (
              <span key={r} style={styles.tag}>
                {r}
                <button style={styles.tagRemove} onClick={() => removeRecipient(r)}>✕</button>
              </span>
            ))}
          </div>
          <div style={styles.inputRow}>
            <input
              style={styles.input}
              placeholder="이메일 추가..."
              value={recipientInput}
              onChange={e => setRecipientInput(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && addRecipient()}
            />
            <button style={styles.addBtn} onClick={addRecipient}>추가</button>
          </div>
        </div>

        {/* 임계값 */}
        <div style={styles.section}>
          <label style={styles.label}>임계값</label>
          <div style={styles.thresholdGrid}>
            <ThresholdRow label="CPU 위험" value={cfg.cpu_threshold}
              onChange={v => setCfg({ ...cfg, cpu_threshold: v })} />
            <ThresholdRow label="메모리 위험" value={cfg.mem_threshold}
              onChange={v => setCfg({ ...cfg, mem_threshold: v })} />
            <ThresholdRow label="디스크 경고" value={cfg.disk_warn}
              onChange={v => setCfg({ ...cfg, disk_warn: v })} />
            <ThresholdRow label="디스크 위험" value={cfg.disk_crit}
              onChange={v => setCfg({ ...cfg, disk_crit: v })} />
          </div>
        </div>

        {error && <p style={{ color: '#f87171', fontSize: 13 }}>{error}</p>}

        <div style={styles.footer}>
          <button style={styles.cancelBtn} onClick={onClose}>취소</button>
          <button style={styles.saveBtn} onClick={handleSave} disabled={saving}>
            {saved ? '저장됨 ✓' : saving ? '저장 중...' : '저장'}
          </button>
        </div>
      </div>
    </div>
  )
}

function ThresholdRow({ label, value, onChange }: { label: string; value: number; onChange: (v: number) => void }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 8 }}>
      <span style={{ color: '#94a3b8', fontSize: 13 }}>{label}</span>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
        <input
          type="number" min={1} max={100}
          value={value}
          onChange={e => onChange(Number(e.target.value))}
          style={{ width: 60, background: '#0f1117', border: '1px solid #334155', borderRadius: 4, color: '#e2e8f0', padding: '4px 8px', textAlign: 'right' }}
        />
        <span style={{ color: '#64748b', fontSize: 13 }}>%</span>
      </div>
    </div>
  )
}

const styles: Record<string, React.CSSProperties> = {
  overlay: {
    position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)',
    display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100,
  },
  modal: {
    background: '#1e293b', border: '1px solid #334155', borderRadius: 12,
    padding: 28, width: 480, maxWidth: '90vw',
  },
  header: { display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 },
  title: { color: '#fff', fontSize: '1.1rem', fontWeight: 700 },
  closeBtn: { background: 'none', border: 'none', color: '#64748b', cursor: 'pointer', fontSize: 18 },
  row: { display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 20 },
  section: { marginBottom: 20 },
  label: { color: '#e2e8f0', fontSize: 13, fontWeight: 600, display: 'block', marginBottom: 8 },
  toggle: { display: 'flex', alignItems: 'center', cursor: 'pointer' },
  tagList: { display: 'flex', flexWrap: 'wrap', gap: 6, marginBottom: 8 },
  tag: {
    background: '#0f172a', border: '1px solid #334155', borderRadius: 4,
    color: '#7dd3fc', fontSize: 12, padding: '3px 8px',
    display: 'flex', alignItems: 'center', gap: 6,
  },
  tagRemove: { background: 'none', border: 'none', color: '#475569', cursor: 'pointer', padding: 0, fontSize: 11 },
  inputRow: { display: 'flex', gap: 8 },
  input: {
    flex: 1, background: '#0f1117', border: '1px solid #334155',
    borderRadius: 6, color: '#e2e8f0', padding: '7px 12px', fontSize: 13,
  },
  addBtn: {
    background: '#1e3a5f', border: '1px solid #3b82f6', borderRadius: 6,
    color: '#7dd3fc', padding: '7px 14px', cursor: 'pointer', fontSize: 13,
  },
  thresholdGrid: { background: '#0f172a', borderRadius: 8, padding: '12px 16px' },
  footer: { display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 24 },
  cancelBtn: {
    background: 'none', border: '1px solid #334155', borderRadius: 6,
    color: '#94a3b8', padding: '8px 18px', cursor: 'pointer',
  },
  saveBtn: {
    background: '#1d4ed8', border: 'none', borderRadius: 6,
    color: '#fff', padding: '8px 18px', cursor: 'pointer', fontWeight: 600,
  },
}
