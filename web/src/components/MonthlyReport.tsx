import { useEffect, useState } from 'react'
import { getReportPreview, sendReport, type MonthlyReport, type HostReport } from '../api/report'

interface Props {
  onClose: () => void
}

function ProgressBar({ value, color }: { value: number; color: string }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
      <div style={{ flex: 1, height: 6, background: '#334155', borderRadius: 3, overflow: 'hidden' }}>
        <div style={{ width: `${Math.min(value, 100)}%`, height: '100%', background: color, borderRadius: 3 }} />
      </div>
      <span style={{ color: '#e2e8f0', fontSize: 12, minWidth: 42, textAlign: 'right' }}>
        {value.toFixed(1)}%
      </span>
    </div>
  )
}

function metricColor(value: number, warn: number, crit: number): string {
  if (value >= crit) return '#ef4444'
  if (value >= warn) return '#f59e0b'
  return '#22c55e'
}

function HostCard({ h }: { h: HostReport }) {
  return (
    <div style={{ background: '#0f1117', borderRadius: 10, padding: '16px 20px', border: '1px solid #1e293b' }}>
      <div style={{ color: '#e2e8f0', fontWeight: 700, fontSize: 15, marginBottom: 14 }}>{h.host}</div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
        <div>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
            <span style={{ color: '#64748b', fontSize: 11 }}>가동률</span>
          </div>
          <ProgressBar value={h.uptime_pct} color={h.uptime_pct >= 99 ? '#22c55e' : h.uptime_pct >= 95 ? '#f59e0b' : '#ef4444'} />
        </div>

        <div>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
            <span style={{ color: '#64748b', fontSize: 11 }}>CPU 평균</span>
            <span style={{ color: '#64748b', fontSize: 11 }}>최대 {h.cpu_max.toFixed(1)}%</span>
          </div>
          <ProgressBar value={h.cpu_avg} color={metricColor(h.cpu_avg, 70, 90)} />
        </div>

        <div>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
            <span style={{ color: '#64748b', fontSize: 11 }}>메모리 평균</span>
            <span style={{ color: '#64748b', fontSize: 11 }}>최대 {h.mem_max.toFixed(1)}%</span>
          </div>
          <ProgressBar value={h.mem_avg} color={metricColor(h.mem_avg, 80, 95)} />
        </div>

        <div>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
            <span style={{ color: '#64748b', fontSize: 11 }}>디스크 최대</span>
          </div>
          <ProgressBar value={h.disk_max} color={metricColor(h.disk_max, 85, 90)} />
        </div>
      </div>
    </div>
  )
}

const MONTHS = ['1월', '2월', '3월', '4월', '5월', '6월', '7월', '8월', '9월', '10월', '11월', '12월']

export default function MonthlyReportModal({ onClose }: Props) {
  const now = new Date()
  const defaultYear = now.getMonth() === 0 ? now.getFullYear() - 1 : now.getFullYear()
  const defaultMonth = now.getMonth() === 0 ? 12 : now.getMonth()

  const [year, setYear] = useState(defaultYear)
  const [month, setMonth] = useState(defaultMonth)
  const [report, setReport] = useState<MonthlyReport | null>(null)
  const [loading, setLoading] = useState(false)
  const [sending, setSending] = useState(false)
  const [error, setError] = useState('')
  const [sent, setSent] = useState(false)

  useEffect(() => {
    loadPreview()
  }, [year, month])

  async function loadPreview() {
    setLoading(true)
    setError('')
    try {
      const data = await getReportPreview(year, month)
      setReport(data)
    } catch {
      setError('보고서 데이터를 불러오지 못했습니다.')
      setReport(null)
    } finally {
      setLoading(false)
    }
  }

  async function handleSend() {
    setSending(true)
    setSent(false)
    setError('')
    try {
      await sendReport(year, month)
      setSent(true)
      setTimeout(() => setSent(false), 4000)
    } catch {
      setError('이메일 발송에 실패했습니다.')
    } finally {
      setSending(false)
    }
  }

  const yearOptions = Array.from({ length: 3 }, (_, i) => now.getFullYear() - i)

  return (
    <div style={{
      position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.7)',
      display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100,
    }} onClick={onClose}>
      <div style={{
        background: '#1e293b', borderRadius: 14, padding: '28px 32px',
        width: '100%', maxWidth: 680, maxHeight: '85vh', overflowY: 'auto',
        border: '1px solid #334155',
      }} onClick={e => e.stopPropagation()}>

        {/* 헤더 */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
          <div>
            <h2 style={{ color: '#f1f5f9', fontSize: 18, fontWeight: 700, margin: 0 }}>월간 보고서</h2>
            <p style={{ color: '#475569', fontSize: 12, margin: '4px 0 0' }}>호스트별 한 달 통계 요약</p>
          </div>
          <button onClick={onClose} style={{ background: 'none', border: 'none', color: '#475569', fontSize: 20, cursor: 'pointer' }}>✕</button>
        </div>

        {/* 기간 선택 */}
        <div style={{ display: 'flex', gap: 12, marginBottom: 24, alignItems: 'center' }}>
          <select
            value={year}
            onChange={e => setYear(Number(e.target.value))}
            style={{ background: '#0f1117', border: '1px solid #334155', color: '#e2e8f0', padding: '6px 12px', borderRadius: 6, fontSize: 13 }}
          >
            {yearOptions.map(y => <option key={y} value={y}>{y}년</option>)}
          </select>
          <select
            value={month}
            onChange={e => setMonth(Number(e.target.value))}
            style={{ background: '#0f1117', border: '1px solid #334155', color: '#e2e8f0', padding: '6px 12px', borderRadius: 6, fontSize: 13 }}
          >
            {MONTHS.map((m, i) => <option key={i + 1} value={i + 1}>{m}</option>)}
          </select>

          <div style={{ flex: 1 }} />

          {sent && (
            <span style={{ color: '#22c55e', fontSize: 13 }}>✓ 이메일 발송 완료</span>
          )}
          {error && (
            <span style={{ color: '#ef4444', fontSize: 13 }}>{error}</span>
          )}

          <button
            onClick={handleSend}
            disabled={sending || loading || !report}
            style={{
              background: '#0ea5e9', border: 'none', color: '#fff',
              padding: '7px 18px', borderRadius: 7, cursor: 'pointer', fontSize: 13, fontWeight: 600,
              opacity: (sending || loading || !report) ? 0.5 : 1,
            }}
          >
            {sending ? '발송 중...' : '이메일 발송'}
          </button>
        </div>

        {/* 내용 */}
        {loading && (
          <div style={{ textAlign: 'center', color: '#475569', padding: 40 }}>데이터 로딩 중...</div>
        )}

        {!loading && report && (
          <>
            <div style={{ color: '#64748b', fontSize: 12, marginBottom: 16 }}>
              {report.year}년 {report.month}월 · 호스트 {report.hosts.length}대
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', gap: 12 }}>
              {report.hosts.map(h => <HostCard key={h.host} h={h} />)}
            </div>
          </>
        )}

        {!loading && !report && !error && (
          <div style={{ textAlign: 'center', color: '#475569', padding: 40 }}>데이터 없음</div>
        )}
      </div>
    </div>
  )
}
