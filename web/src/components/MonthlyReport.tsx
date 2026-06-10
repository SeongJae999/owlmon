import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getReportPreview, sendReport, type MonthlyReport, type HostReport } from '../api/report'
import { getEmailStatus } from '../api/alert'
import { FileBarChart, Mail, Calendar, Server, CheckCircle2, AlertTriangle, RefreshCcw, Info } from 'lucide-react'
import { cn } from '../utils/cn'

function ProgressBar({ value, colorClass }: { value: number; colorClass: string }) {
  return (
    <div className="space-y-1">
      <div className="h-1.5 w-full bg-slate-800 rounded-full overflow-hidden">
        <div 
          className={cn("h-full transition-all duration-500", colorClass)} 
          style={{ width: `${Math.min(value, 100)}%` }} 
        />
      </div>
    </div>
  )
}

function metricColorClass(value: number, warn: number, crit: number): string {
  if (value >= crit) return 'bg-rose-500'
  if (value >= warn) return 'bg-amber-500'
  return 'bg-emerald-500'
}

function HostCard({ h }: { h: HostReport }) {
  return (
    <div className="bg-slate-900 rounded-3xl border border-slate-800 p-5 shadow-premium hover:shadow-lg transition-all">
      <div className="flex items-center gap-2 mb-6">
        <div className="p-1.5 bg-slate-800 rounded-lg">
          <Server size={14} className="text-slate-400" />
        </div>
        <h4 className="font-bold text-slate-200">{h.host}</h4>
      </div>

      <div className="space-y-5">
        <div>
          <div className="flex justify-between text-[10px] font-bold uppercase tracking-wider mb-1.5">
            <span className="text-slate-400">가동률</span>
            <span className={cn(
              h.uptime_pct >= 99 ? "text-emerald-400" : h.uptime_pct >= 95 ? "text-amber-400" : "text-rose-400"
            )}>{h.uptime_pct.toFixed(1)}%</span>
          </div>
          <ProgressBar 
            value={h.uptime_pct} 
            colorClass={h.uptime_pct >= 99 ? 'bg-emerald-500' : h.uptime_pct >= 95 ? 'bg-amber-500' : 'bg-rose-500'} 
          />
        </div>

        <div>
          <div className="flex justify-between text-[10px] font-bold uppercase tracking-wider mb-1.5">
            <span className="text-slate-400">CPU 평균</span>
            <span className="text-slate-400">Max {h.cpu_max.toFixed(1)}%</span>
          </div>
          <div className="flex items-center gap-3">
            <div className="flex-1">
              <ProgressBar value={h.cpu_avg} colorClass={metricColorClass(h.cpu_avg, 70, 90)} />
            </div>
            <span className="text-[11px] font-bold text-slate-400 w-10 text-right">{h.cpu_avg.toFixed(1)}%</span>
          </div>
        </div>

        <div>
          <div className="flex justify-between text-[10px] font-bold uppercase tracking-wider mb-1.5">
            <span className="text-slate-400">메모리 평균</span>
            <span className="text-slate-400">Max {h.mem_max.toFixed(1)}%</span>
          </div>
          <div className="flex items-center gap-3">
            <div className="flex-1">
              <ProgressBar value={h.mem_avg} colorClass={metricColorClass(h.mem_avg, 80, 95)} />
            </div>
            <span className="text-[11px] font-bold text-slate-400 w-10 text-right">{h.mem_avg.toFixed(1)}%</span>
          </div>
        </div>

        <div>
          <div className="flex justify-between text-[10px] font-bold uppercase tracking-wider mb-1.5">
            <span className="text-slate-400">디스크 최대 사용률</span>
          </div>
          <div className="flex items-center gap-3">
            <div className="flex-1">
              <ProgressBar value={h.disk_max} colorClass={metricColorClass(h.disk_max, 85, 90)} />
            </div>
            <span className="text-[11px] font-bold text-slate-400 w-10 text-right">{h.disk_max.toFixed(1)}%</span>
          </div>
        </div>
      </div>
    </div>
  )
}

const MONTHS = ['1월', '2월', '3월', '4월', '5월', '6월', '7월', '8월', '9월', '10월', '11월', '12월']

export default function MonthlyReportModal() {
  const now = new Date()
  // 디폴트: 이번 달 (Prometheus 30일 retention 고려 — 지난달 데이터는 보존 안 될 수 있음)
  const [year, setYear] = useState(now.getFullYear())
  const [month, setMonth] = useState(now.getMonth() + 1)
  const [report, setReport] = useState<MonthlyReport | null>(null)
  const [loading, setLoading] = useState(false)
  const [sending, setSending] = useState(false)
  const [error, setError] = useState('')
  const [sent, setSent] = useState(false)

  // SMTP 설정 상태 — 발송 버튼 활성화 여부 결정
  const { data: emailStatus } = useQuery({
    queryKey: ['emailStatus'],
    queryFn: getEmailStatus,
    staleTime: 60_000,
  })
  const canSend = emailStatus?.smtp_configured && (emailStatus?.recipients_count ?? 0) > 0

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
    <div className="space-y-6">
      {/* Selection & Actions */}
      <div className="bg-slate-900 p-4 rounded-3xl border border-slate-800 shadow-sm flex flex-col sm:flex-row gap-4 items-center justify-between">
        <div className="flex items-center gap-3 w-full sm:w-auto">
          <div className="p-2 bg-indigo-500/10 text-indigo-400 rounded-lg">
            <Calendar size={20} />
          </div>
          <div className="flex gap-2">
            <select 
              className="bg-slate-800 border border-slate-800 rounded-lg px-3 py-1.5 text-sm font-bold text-slate-400 focus:outline-none focus:ring-2 focus:ring-indigo-500/20"
              value={year} 
              onChange={e => setYear(Number(e.target.value))}
            >
              {yearOptions.map(y => <option key={y} value={y}>{y}년</option>)}
            </select>
            <select 
              className="bg-slate-800 border border-slate-800 rounded-lg px-3 py-1.5 text-sm font-bold text-slate-400 focus:outline-none focus:ring-2 focus:ring-indigo-500/20"
              value={month} 
              onChange={e => setMonth(Number(e.target.value))}
            >
              {MONTHS.map((m, i) => <option key={i + 1} value={i + 1}>{m}</option>)}
            </select>
          </div>
        </div>

        <div className="flex items-center gap-4 w-full sm:w-auto">
          {sent && (
            <span className="text-xs font-bold text-emerald-400 flex items-center gap-1.5">
              <CheckCircle2 size={14} /> 이메일 발송 완료
            </span>
          )}
          {error && (
            <span className="text-xs font-bold text-rose-500 flex items-center gap-1.5">
              <AlertTriangle size={14} /> {error}
            </span>
          )}

          <button
            className={cn(
              "w-full sm:w-auto flex items-center justify-center gap-2 px-6 py-2.5 rounded-xl text-sm font-bold transition-all shadow-lg",
              sending || loading || !report || !canSend
                ? "bg-slate-800 text-slate-400 cursor-not-allowed shadow-none"
                : "bg-indigo-600 text-white hover:bg-indigo-700 shadow-indigo-500/20"
            )}
            onClick={handleSend}
            disabled={sending || loading || !report || !canSend}
            title={!canSend ? "SMTP 미설정 또는 수신자 없음 — '알림 설정'에서 등록" : "보고서를 이메일로 발송"}
          >
            {sending ? (
              <RefreshCcw size={18} className="animate-spin" />
            ) : (
              <Mail size={18} />
            )}
            {sending ? '발송 중...' : '이메일 발송'}
          </button>
        </div>
      </div>

      {/* SMTP/수신자 안내 (발송 불가 시) — 두 가지 원인 분리 표시 */}
      {!canSend && (
        <div className="flex items-start gap-2 px-3 py-2.5 rounded-lg bg-amber-500/10 border border-amber-500/30 text-xs text-amber-300">
          <AlertTriangle size={14} className="shrink-0 mt-0.5" />
          <div className="space-y-1">
            <div className="font-bold">이메일 발송 불가 — 미리보기는 정상 동작</div>
            {!emailStatus?.smtp_configured && (
              <div>
                · <strong>SMTP 서버 미설정</strong> — 서버 관리자가{' '}
                <code className="bg-slate-800 px-1 rounded text-[10px]">docker-compose .env</code>에
                {' '}<code className="bg-slate-800 px-1 rounded text-[10px]">SMTP_HOST/PORT/USERNAME/PASSWORD</code>
                {' '}설정 필요. <a href="/support#requirements" className="underline">설치 가이드 →</a>
              </div>
            )}
            {emailStatus?.smtp_configured && (emailStatus?.recipients_count ?? 0) === 0 && (
              <div>
                · <strong>수신자 미등록</strong> — <a href="/settings" className="underline font-bold">알림 설정</a>에서 이메일 주소 추가
              </div>
            )}
          </div>
        </div>
      )}

      {/* Report Content */}
      <div className="space-y-4">
        {loading ? (
          <div className="bg-slate-900 rounded-3xl border border-slate-800 border-dashed py-32 flex flex-col items-center justify-center text-slate-400 animate-pulse">
            <RefreshCcw size={48} className="mb-4 opacity-20 animate-spin" />
            <p className="font-medium">보고서 데이터를 집계하는 중...</p>
          </div>
        ) : report && report.hosts && report.hosts.length > 0 ? (
          <>
            <div className="flex items-center gap-2 px-1">
              <FileBarChart size={18} className="text-slate-400" />
              <h2 className="text-lg font-bold text-slate-200">
                {report.year}년 {report.month}월 월간 보고서 요약
                <span className="ml-2 text-xs font-medium text-slate-400">호스트 {report.hosts.length}대</span>
              </h2>
            </div>

            {/* 30일 retention 초과 안내 — 모든 호스트 값이 0이면 데이터 손실 가능성 */}
            {report.hosts.every(h => h.cpu_avg === 0 && h.mem_avg === 0 && h.uptime_pct === 0) && (
              <div className="flex items-start gap-2 px-3 py-2.5 rounded-lg bg-amber-500/10 border border-amber-500/30 text-xs text-amber-300">
                <Info size={14} className="shrink-0 mt-0.5" />
                <span>
                  모든 호스트의 수치가 0입니다 — Prometheus 보관 기간(30일) 초과 또는 데이터 미수집 시점일 가능성. 최근 달을 선택하세요.
                </span>
              </div>
            )}

            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
              {report.hosts.map(h => <HostCard key={h.host} h={h} />)}
            </div>
          </>
        ) : !error && (
          <div className="bg-slate-900 rounded-3xl border border-slate-800 border-dashed py-20 flex flex-col items-center justify-center text-slate-400 text-center px-6">
            <Info size={36} className="mb-3 opacity-30" />
            <p className="font-medium text-sm">해당 기간의 데이터가 없습니다.</p>
            <p className="text-xs opacity-70 mt-1 max-w-md leading-relaxed">
              Prometheus 보관 기간(30일)을 초과한 데이터는 자동 삭제됩니다.
              <br />
              {year}년 {month}월 시점에 등록되지 않았거나, 30일 이전 데이터는 조회할 수 없습니다.
            </p>
          </div>
        )}
      </div>
    </div>
  )
}
