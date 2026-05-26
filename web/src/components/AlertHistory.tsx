import { getAlertHistory } from '../api/alert'
import { useQuery } from '@tanstack/react-query'
import { History, AlertCircle, AlertTriangle, Info, Server, RefreshCcw, Bell } from 'lucide-react'
import { cn } from '../utils/cn'

const SEVERITY_CONFIG: Record<string, { bg: string, text: string, icon: any }> = {
  critical: { bg: 'bg-rose-500/15', text: 'text-rose-300', icon: AlertCircle },
  warning: { bg: 'bg-amber-500/15', text: 'text-amber-300', icon: AlertTriangle },
  info: { bg: 'bg-blue-500/15', text: 'text-blue-300', icon: Info },
}

export default function AlertHistory() {
  const { data: records = [], isLoading, error } = useQuery({
    queryKey: ['alertHistory'],
    queryFn: () => getAlertHistory(100),
  })

  return (
    <div className="space-y-6">
      {/* Page Header Info */}
      <div className="bg-indigo-500/100/10 border border-indigo-500/20 rounded-2xl p-4 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-slate-900 rounded-lg shadow-sm">
            <Bell size={18} className="text-indigo-400" />
          </div>
          <p className="text-xs font-semibold text-indigo-800">최근 100개의 알림 발송 이력을 표시합니다.</p>
        </div>
        <div className="text-[10px] font-bold text-indigo-400 uppercase tracking-widest">
          PostgreSQL Storage
        </div>
      </div>

      <div className="bg-slate-900 rounded-3xl border border-slate-800 shadow-sm overflow-hidden">
        {isLoading ? (
          <div className="flex flex-col items-center justify-center py-32 text-slate-400 animate-pulse">
            <RefreshCcw size={48} className="mb-4 opacity-20 animate-spin" />
            <p className="font-medium">알림 이력을 불러오는 중...</p>
          </div>
        ) : error ? (
          <div className="flex flex-col items-center justify-center py-32 text-rose-500 text-center px-6">
            <AlertCircle size={48} className="mb-4 opacity-20" />
            <p className="font-bold">히스토리를 불러올 수 없습니다.</p>
            <p className="text-xs opacity-70 mt-1">PostgreSQL 미연결 또는 서버 오류일 수 있습니다.</p>
          </div>
        ) : records.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-32 text-slate-400 text-center px-6">
            <History size={48} className="mb-4 opacity-20" />
            <p className="font-medium">알림 발송 이력이 없습니다.</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse">
              <thead>
                <tr className="bg-slate-800/50 border-b border-slate-800">
                  <th className="px-6 py-4 text-[10px] font-bold text-slate-400 uppercase tracking-wider w-48 shrink-0">발송 시각</th>
                  <th className="px-6 py-4 text-[10px] font-bold text-slate-400 uppercase tracking-wider w-32 shrink-0">심각도</th>
                  <th className="px-6 py-4 text-[10px] font-bold text-slate-400 uppercase tracking-wider w-40 shrink-0">호스트</th>
                  <th className="px-6 py-4 text-[10px] font-bold text-slate-400 uppercase tracking-wider">내용</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-50">
                {records.map((r) => {
                  const cfg = SEVERITY_CONFIG[r.severity] ?? { bg: 'bg-slate-800', text: 'text-slate-400', icon: Info }
                  return (
                    <tr key={r.id} className="hover:bg-slate-800/50 transition-colors group">
                      <td className="px-6 py-4 whitespace-nowrap">
                        <div className="text-xs font-medium text-slate-500">
                          {new Date(r.sent_at).toLocaleString('ko-KR', {
                            year: 'numeric', month: '2-digit', day: '2-digit', 
                            hour: '2-digit', minute: '2-digit', second: '2-digit'
                          })}
                        </div>
                      </td>
                      <td className="px-6 py-4">
                        <span className={cn(
                          "inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[10px] font-bold uppercase tracking-tight",
                          cfg.bg, cfg.text
                        )}>
                          <cfg.icon size={10} />
                          {r.severity}
                        </span>
                      </td>
                      <td className="px-6 py-4">
                        <div className="flex items-center gap-2 text-xs font-bold text-slate-400">
                          <Server size={12} className="opacity-30" />
                          {r.host}
                        </div>
                      </td>
                      <td className="px-6 py-4">
                        <div className="space-y-1">
                          <div className="text-sm font-bold text-slate-200 group-hover:text-indigo-400 transition-colors">
                            {r.subject}
                          </div>
                          <div className="text-xs text-slate-500 leading-relaxed max-w-2xl">
                            {r.body}
                          </div>
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}
