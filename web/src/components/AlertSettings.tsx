import React, { useState, useEffect } from 'react'
import { getAlertConfig, setAlertConfig, type AlertConfig } from '../api/alert'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Mail, Bell, AlertTriangle, Plus, X, Save, CheckCircle2, RefreshCcw } from 'lucide-react'
import { cn } from '../utils/cn'

interface Props {
  onSave?: () => void
}

export default function AlertSettings({ onSave }: Props) {
  const queryClient = useQueryClient()
  const [recipientInput, setRecipientInput] = useState('')
  const [localCfg, setLocalCfg] = useState<AlertConfig | null>(null)

  const { data: cfg, isLoading, error: loadError } = useQuery({
    queryKey: ['alertConfig'],
    queryFn: getAlertConfig,
  })

  // Sync local config when data is loaded
  React.useEffect(() => {
    if (cfg) setLocalCfg(cfg)
  }, [cfg])

  const mutation = useMutation({
    mutationFn: setAlertConfig,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['alertConfig'] })
      if (onSave) onSave()
    }
  })

  // 저장 성공 메시지 3초 후 자동 사라짐
  const [showSuccess, setShowSuccess] = useState(false)
  useEffect(() => {
    if (mutation.isSuccess) {
      setShowSuccess(true)
      const t = setTimeout(() => setShowSuccess(false), 3000)
      return () => clearTimeout(t)
    }
  }, [mutation.isSuccess])

  if (isLoading && !localCfg) return (
    <div className="flex flex-col items-center justify-center py-20 text-slate-400 animate-pulse">
      <RefreshCcw size={48} className="mb-4 opacity-20 animate-spin" />
      <p className="font-medium">알림 설정을 불러오는 중...</p>
    </div>
  )

  if (loadError) return (
    <div className="bg-rose-500/10 border border-rose-500/20 rounded-xl p-6 text-center text-rose-300">
      <AlertTriangle size={32} className="mx-auto mb-3 opacity-50" />
      <p className="font-bold">설정을 불러오지 못했습니다.</p>
    </div>
  )

  if (!localCfg) return null

  const addRecipient = () => {
    const email = recipientInput.trim()
    if (!email) return
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) return
    if (!localCfg.recipients.includes(email)) {
      setLocalCfg({ ...localCfg, recipients: [...localCfg.recipients, email] })
    }
    setRecipientInput('')
  }

  const removeRecipient = (email: string) => {
    setLocalCfg({ ...localCfg, recipients: localCfg.recipients.filter(r => r !== email) })
  }

  const handleSave = () => {
    mutation.mutate(localCfg)
  }

  return (
    <div className="max-w-3xl space-y-6">
      <div className="bg-slate-900 rounded-3xl border border-slate-800 shadow-sm overflow-hidden">
        {/* Header */}
        <div className="p-6 border-b border-slate-800 bg-slate-800/30 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="p-2 bg-indigo-600 text-white rounded-lg shadow-sm shadow-indigo-500/20">
              <Bell size={20} />
            </div>
            <div>
              <h3 className="font-bold text-slate-100">알림 정책 설정</h3>
              <p className="text-xs text-slate-500 font-medium">장애 발생 시 알림 방식 및 임계값 관리</p>
            </div>
          </div>

          <label className="flex items-center gap-2 cursor-pointer group">
            <div className="relative inline-flex items-center cursor-pointer">
              <input
                type="checkbox"
                className="sr-only peer"
                checked={localCfg.enabled}
                onChange={e => setLocalCfg({ ...localCfg, enabled: e.target.checked })}
                aria-label="알림 발송 활성화"
              />
              <div className="w-11 h-6 bg-slate-700 peer-focus:outline-none peer-focus:ring-2 peer-focus:ring-indigo-500/30 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-slate-900 after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-indigo-600"></div>
            </div>
            <span className={cn(
              "text-xs font-bold tracking-wide",
              localCfg.enabled ? "text-indigo-400" : "text-slate-400"
            )}>
              {localCfg.enabled ? '활성' : '비활성'}
            </span>
          </label>
        </div>

        <div className="p-6 space-y-8">
          {/* Email Recipients */}
          <div className="space-y-4">
            <div className="flex items-center gap-2 px-1">
              <Mail size={16} className="text-indigo-400" />
              <label className="text-sm font-bold text-slate-200">이메일 수신자</label>
            </div>

            <div className="flex flex-wrap gap-2 mb-2">
              {localCfg.recipients.map(r => (
                <div key={r} className="inline-flex items-center gap-1.5 px-2.5 py-1 bg-slate-800 text-slate-300 rounded-lg text-xs font-semibold border border-slate-700 transition-all hover:border-slate-600">
                  {r}
                  <button
                    className="p-0.5 hover:bg-slate-700 rounded-md text-slate-400 hover:text-rose-500 transition-colors"
                    onClick={() => removeRecipient(r)}
                    aria-label={`이메일 수신자 삭제: ${r}`}
                  >
                    <X size={14} />
                  </button>
                </div>
              ))}
              {localCfg.recipients.length === 0 && (
                <p className="text-xs text-slate-500 italic py-1">등록된 수신자가 없습니다.</p>
              )}
            </div>

            <div className="flex gap-2">
              <input
                className="flex-1 bg-slate-800 border border-slate-700 rounded-xl px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition-all"
                placeholder="예: admin@school.go.kr"
                value={recipientInput}
                onChange={e => setRecipientInput(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && addRecipient()}
              />
              <button
                className="bg-slate-900 border border-slate-700 text-slate-300 hover:bg-slate-800 px-4 py-2.5 rounded-xl text-sm font-bold transition-all shadow-sm flex items-center gap-2"
                onClick={addRecipient}
              >
                <Plus size={18} />
                추가
              </button>
            </div>
          </div>

          {/* Thresholds */}
          <div className="space-y-4">
            <div className="flex items-center gap-2 px-1">
              <AlertTriangle size={16} className="text-amber-400" />
              <label className="text-sm font-bold text-slate-200">임계값 설정</label>
              <span className="text-[11px] text-slate-500 font-normal">(1–100 %)</span>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 p-4 bg-slate-800/40 rounded-2xl border border-slate-700">
              <ThresholdField label="CPU 위험" value={localCfg.cpu_threshold}
                onChange={v => setLocalCfg({ ...localCfg, cpu_threshold: v })} />
              <ThresholdField label="메모리 위험" value={localCfg.mem_threshold}
                onChange={v => setLocalCfg({ ...localCfg, mem_threshold: v })} />
              <ThresholdField label="디스크 경고" value={localCfg.disk_warn}
                onChange={v => setLocalCfg({ ...localCfg, disk_warn: v })} />
              <ThresholdField label="디스크 위험" value={localCfg.disk_crit}
                onChange={v => setLocalCfg({ ...localCfg, disk_crit: v })} />
            </div>
          </div>
        </div>

        {/* Footer Actions */}
        <div className="p-6 bg-slate-800/50 border-t border-slate-800 flex items-center justify-between">
          <div className="flex-1 mr-4">
            {mutation.isPending && (
              <p className="text-xs text-slate-400 flex items-center gap-2">
                <RefreshCcw size={12} className="animate-spin" /> 저장 중...
              </p>
            )}
            {showSuccess && (
              <p className="text-xs text-emerald-400 font-bold flex items-center gap-2">
                <CheckCircle2 size={14} /> 설정이 저장되었습니다.
              </p>
            )}
            {mutation.isError && (
              <p className="text-xs text-rose-500 font-bold flex items-center gap-2">
                <AlertTriangle size={14} /> 저장 실패: 다시 시도해 주세요.
              </p>
            )}
          </div>
          
          <button 
            className={cn(
              "flex items-center gap-2 px-8 py-3 rounded-xl text-sm font-bold transition-all shadow-lg",
              mutation.isPending ? "bg-slate-700 text-slate-400 cursor-not-allowed" : "bg-indigo-600 text-white hover:bg-indigo-700 shadow-indigo-500/20"
            )}
            onClick={handleSave}
            disabled={mutation.isPending}
          >
            <Save size={18} />
            설정 저장하기
          </button>
        </div>
      </div>
    </div>
  )
}

function ThresholdField({ label, value, onChange }: { label: string; value: number; onChange: (v: number) => void }) {
  // 빈 칸 허용을 위해 로컬 string 상태로 관리 (Number('') = 0 버그 회피)
  const [raw, setRaw] = useState(String(value))
  useEffect(() => { setRaw(String(value)) }, [value])

  return (
    <div className="flex items-center justify-between p-3 bg-slate-900 rounded-xl border border-slate-700 shadow-sm">
      <span className="text-xs font-bold text-slate-300">{label}</span>
      <div className="flex items-center gap-2">
        <input
          type="number" min={1} max={100} inputMode="numeric"
          value={raw}
          onChange={e => {
            const v = e.target.value
            setRaw(v)
            const n = Number(v)
            if (v !== '' && Number.isFinite(n) && n >= 1 && n <= 100) onChange(n)
          }}
          onBlur={e => {
            if (!e.target.value.trim() || Number(e.target.value) <= 0) {
              setRaw(String(value)) // 잘못된 입력 시 이전 값 복구
            }
          }}
          className="w-16 bg-slate-800 border border-slate-700 rounded-lg px-2 py-1.5 text-sm font-bold text-slate-200 text-right tabular-nums focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500"
        />
        <span className="text-xs font-bold text-slate-400">%</span>
      </div>
    </div>
  )
}
