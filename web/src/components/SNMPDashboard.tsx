import React, { useState } from 'react'
import {
  getSNMPDevices, getSNMPStatus, addSNMPDevice, deleteSNMPDevice,
  type DeviceStatus,
} from '../api/snmp'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Network, ArrowDown, ArrowUp, Clock, Trash2, Plus, X, Save, AlertTriangle, RefreshCcw } from 'lucide-react'
import { cn } from '../utils/cn'
import PageToolbar from './PageToolbar'

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

// 시스템/관리용 인터페이스 — 운영자에게 의미 없음 (필터)
function isSystemInterface(name: string): boolean {
  return /CPU Interface|Loopback|Null|Vlan|VLAN|mgmt/i.test(name)
}

// "8 Gigabit - Level" → "Port 8 (Gigabit)" 같이 사람 친화적 변환
function prettifyInterfaceName(raw: string): string {
  // NETGEAR 패턴: "N Gigabit - Level" (Smart Switch 표준 ifDescr)
  const netgear = raw.match(/^(\d+)\s+(Gigabit|FastEthernet|TenGigabit)\s*-\s*Level/i)
  if (netgear) return `Port ${netgear[1]} (${netgear[2]})`
  // Cisco/일반 패턴은 그대로 두되 길이만 truncate (상위에서 title 속성으로 풀텍스트 노출됨)
  return raw
}

function DeviceCard({ status, onDelete }: { status: DeviceStatus; onDelete: () => void }) {
  const [showDown, setShowDown] = useState(false)
  // 시스템 인터페이스 제거 + UP/DOWN 분리
  const realIfs = (status.Interfaces ?? []).filter(i => !isSystemInterface(i.Name))
  const activeIfs = realIfs.filter(i => i.OperUp)
  const downIfs = realIfs.filter(i => !i.OperUp)

  return (
    <div className={cn(
      "bg-slate-900 rounded-3xl border p-5 shadow-premium transition-all relative overflow-hidden",
      status.Up ? "border-slate-800" : "border-rose-500/40 ring-1 ring-rose-500/20"
    )}>

      {/* Header */}
      <div className="flex items-start justify-between mb-6">
        <div className="flex items-center gap-3">
          <div className={cn(
            "w-2.5 h-2.5 rounded-full ring-4 ring-offset-2 ring-offset-slate-900",
            status.Up ? "bg-emerald-500 ring-emerald-500/20 animate-pulse" : "bg-rose-500 ring-rose-500/20"
          )} />
          <div>
            <h4 className="font-bold text-slate-100 leading-tight">{status.Device.Name}</h4>
            <div className="flex items-center gap-2 mt-0.5 flex-wrap">
              <span className="text-[11px] font-bold text-slate-400 font-mono">{status.Device.IP}</span>
              {status.Up && status.UptimeSec > 0 && (
                <div className="flex items-center gap-1 text-[10px] text-slate-400 font-medium">
                  <Clock size={10} />
                  {formatUptime(status.UptimeSec)}
                </div>
              )}
              {status.ResponseMs !== undefined && status.ResponseMs > 0 && (
                <span className={cn(
                  "text-[10px] font-bold tabular-nums px-1.5 py-0.5 rounded",
                  status.ResponseMs < 200 ? "bg-emerald-500/10 text-emerald-300"
                    : status.ResponseMs < 1000 ? "bg-amber-500/10 text-amber-300"
                    : "bg-rose-500/10 text-rose-300"
                )}>
                  {status.ResponseMs}ms
                </span>
              )}
            </div>
          </div>
        </div>
        <button
          className="p-1.5 rounded-lg text-slate-400 hover:text-rose-300 hover:bg-rose-500/10 transition-all"
          onClick={onDelete}
        >
          <Trash2 size={16} />
        </button>
      </div>

      {!status.Up ? (
        <div className="bg-rose-500/10 border border-rose-500/30 rounded-xl p-4 flex items-start gap-3 text-rose-200 text-xs font-medium">
          <AlertTriangle size={16} className="shrink-0 mt-0.5" />
          <div className="space-y-1 min-w-0">
            <p className="font-bold">SNMP 응답 없음</p>
            {status.LastError ? (
              <p className="text-[11px] text-rose-300/90 break-words font-mono">{status.LastError}</p>
            ) : (
              <p className="text-[11px] text-rose-300/80">
                다음 폴링(30초 주기)을 기다리거나, 장비의 SNMP 활성화/community/방화벽을 확인하세요.
              </p>
            )}
          </div>
        </div>
      ) : (
        <div className="space-y-4">
          {/* Status Summary */}
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-1.5">
              <div className="w-1.5 h-1.5 rounded-full bg-emerald-500" />
              <span className="text-[10px] font-bold text-slate-400 uppercase tracking-tight">UP: {activeIfs.length}</span>
            </div>
            {downIfs.length > 0 && (
              <button
                onClick={() => setShowDown(s => !s)}
                className="flex items-center gap-1.5 hover:opacity-80 transition-opacity"
                title="케이블 안 꽂힌 빈 포트 — 클릭하면 펼침"
              >
                <div className="w-1.5 h-1.5 rounded-full bg-slate-500" />
                <span className="text-[10px] font-bold text-slate-500 uppercase tracking-tight">
                  미사용: {downIfs.length} {showDown ? '▴' : '▾'}
                </span>
              </button>
            )}
          </div>

          {/* Interface Traffic — UP만 + 트래픽 있는 것 우선 */}
          <div className="space-y-2">
            {activeIfs
              .slice()
              .sort((a, b) => (b.InBps + b.OutBps) - (a.InBps + a.OutBps))
              .slice(0, 8)
              .map(iface => {
                const hasTraffic = iface.InBps > 0 || iface.OutBps > 0
                return (
                  <div key={iface.Index} className="p-2 bg-slate-800/50 rounded-lg border border-slate-800/50 flex items-center justify-between gap-4">
                    <span className="text-[11px] font-bold text-slate-300 truncate flex-1 min-w-0" title={iface.Name}>
                      {prettifyInterfaceName(iface.Name)}
                    </span>
                    {hasTraffic ? (
                      <div className="flex gap-3 shrink-0">
                        <div className="flex items-center gap-1 text-[10px] font-bold text-sky-400">
                          <ArrowDown size={10} />
                          {formatBps(iface.InBps)}
                        </div>
                        <div className="flex items-center gap-1 text-[10px] font-bold text-rose-300">
                          <ArrowUp size={10} />
                          {formatBps(iface.OutBps)}
                        </div>
                      </div>
                    ) : (
                      <span className="text-[10px] text-slate-600 shrink-0">유휴</span>
                    )}
                  </div>
                )
              })}
            {activeIfs.length === 0 && (
              <div className="py-4 text-center text-[11px] font-bold text-slate-400 italic">
                활성 인터페이스 없음
              </div>
            )}
          </div>

          {/* DOWN 포트 펼침 영역 */}
          {showDown && downIfs.length > 0 && (
            <div className="space-y-1 pt-2 border-t border-slate-800">
              <div className="text-[10px] font-semibold text-slate-500 mb-1">미사용 포트 (케이블 없음)</div>
              <div className="flex flex-wrap gap-1">
                {downIfs.map(iface => (
                  <span
                    key={iface.Index}
                    className="px-1.5 py-0.5 rounded text-[10px] bg-slate-800/50 text-slate-500 border border-slate-800"
                    title={iface.Name}
                  >
                    {prettifyInterfaceName(iface.Name)}
                  </span>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {status.CollectedAt && (
        <div className="mt-4 pt-3 border-t border-slate-800 flex justify-end">
          <div className="text-[10px] font-semibold text-slate-500 tabular-nums">
            마지막 확인: {new Date(status.CollectedAt).toLocaleTimeString('ko-KR')}
          </div>
        </div>
      )}
    </div>
  )
}

export default function SNMPDashboard() {
  const queryClient = useQueryClient()
  const [showAdd, setShowAdd] = useState(false)
  const [form, setForm] = useState({ Name: '', IP: '', Community: 'public', Port: 161 })
  const [fastRefresh, setFastRefresh] = useState(false) // 등록 직후 짧은 갱신 모드

  const { data: devices = [], isLoading: devicesLoading } = useQuery({
    queryKey: ['snmpDevices'],
    queryFn: getSNMPDevices,
  })

  const { data: statuses = [] } = useQuery({
    queryKey: ['snmpStatus'],
    queryFn: getSNMPStatus,
    refetchInterval: fastRefresh ? 2000 : 15000, // 등록 직후 2초, 평소 15초 (서버 ticker는 30초)
  })

  const addMutation = useMutation({
    mutationFn: addSNMPDevice,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['snmpDevices'] })
      queryClient.invalidateQueries({ queryKey: ['snmpStatus'] })
      setShowAdd(false)
      setForm({ Name: '', IP: '', Community: 'public', Port: 161 })
      // 등록 후 10초간 빠른 새로고침 → 첫 폴링 결과 즉시 표시
      setFastRefresh(true)
      setTimeout(() => setFastRefresh(false), 10000)
    }
  })

  const deleteMutation = useMutation({
    mutationFn: deleteSNMPDevice,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['snmpDevices'] })
      queryClient.invalidateQueries({ queryKey: ['snmpStatus'] })
    }
  })

  const handleAdd = (e: React.FormEvent) => {
    e.preventDefault()
    if (!form.Name || !form.IP) return
    addMutation.mutate(form)
  }

  const handleDelete = (id: number) => {
    if (!confirm('이 장치를 삭제하시겠습니까?')) return
    deleteMutation.mutate(id)
  }

  const statusMap = new Map(statuses.map(s => [s.Device.ID, s]))

  return (
    <div className="space-y-6">
      <PageToolbar icon={Network} title="네트워크 장비 (SNMP)" description="스위치 및 라우터 인터페이스 모니터링">
        <button
          className={cn(
            "flex items-center gap-2 px-6 py-2.5 rounded-xl text-sm font-bold transition-all shadow-lg",
            showAdd ? "bg-slate-800 text-slate-400 shadow-none" : "bg-indigo-600 text-white hover:bg-indigo-700 shadow-indigo-500/20"
          )}
          onClick={() => setShowAdd(!showAdd)}
        >
          {showAdd ? <X size={18} /> : <Plus size={18} />}
          {showAdd ? '닫기' : '장치 추가'}
        </button>
      </PageToolbar>

      {/* Add Device Form */}
      {showAdd && (
        <form onSubmit={handleAdd} className="bg-slate-900 rounded-3xl border border-indigo-500/30 p-6 shadow-xl animate-in fade-in slide-in-from-top-4 duration-300 space-y-6">
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
            <div className="space-y-1.5">
              <label className="text-[10px] font-bold text-slate-400 uppercase tracking-widest ml-1">Device Name</label>
              <input
                className="w-full bg-slate-800 border border-slate-800 rounded-xl px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition-all font-medium"
                placeholder="예: 2층 메인 스위치"
                value={form.Name}
                onChange={e => setForm({...form, Name: e.target.value})}
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-[10px] font-bold text-slate-400 uppercase tracking-widest ml-1">IP Address</label>
              <input
                className="w-full bg-slate-800 border border-slate-800 rounded-xl px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition-all font-medium"
                placeholder="192.168.1.1"
                value={form.IP}
                onChange={e => setForm({...form, IP: e.target.value})}
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-[10px] font-bold text-slate-400 uppercase tracking-widest ml-1">SNMP Community</label>
              <input
                className="w-full bg-slate-800 border border-slate-800 rounded-xl px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition-all font-medium"
                placeholder="public"
                value={form.Community}
                onChange={e => setForm({...form, Community: e.target.value})}
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-[10px] font-bold text-slate-400 uppercase tracking-widest ml-1">UDP Port</label>
              <input
                className="w-full bg-slate-800 border border-slate-800 rounded-xl px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition-all font-medium"
                type="number"
                placeholder="161"
                value={form.Port}
                onChange={e => setForm({...form, Port: Number(e.target.value)})}
              />
            </div>
          </div>
          <div className="flex justify-end gap-3 pt-2">
            {addMutation.isError && (
              <p className="text-xs text-rose-500 font-bold flex items-center gap-2 mr-auto">
                <AlertTriangle size={14} /> 장치 추가 실패: 설정값을 확인하세요.
              </p>
            )}
            <button 
              type="submit" 
              disabled={addMutation.isPending}
              className="flex items-center gap-2 px-10 py-3 bg-indigo-600 text-white rounded-xl text-sm font-bold hover:bg-indigo-700 shadow-lg shadow-indigo-500/20 transition-all disabled:opacity-50"
            >
              {addMutation.isPending ? <RefreshCcw size={18} className="animate-spin" /> : <Save size={18} />}
              장치 등록하기
            </button>
          </div>
        </form>
      )}

      {/* Device Grid */}
      {devicesLoading ? (
        <div className="flex flex-col items-center justify-center py-32 text-slate-400 animate-pulse">
          <RefreshCcw size={48} className="mb-4 opacity-20 animate-spin" />
          <p className="font-medium">네트워크 장비 정보를 불러오는 중...</p>
        </div>
      ) : devices.length === 0 && !showAdd ? (
        <div className="bg-slate-900 rounded-3xl border border-slate-800 border-dashed py-32 flex flex-col items-center justify-center text-slate-400 text-center px-6">
          <Network size={48} className="mb-4 opacity-20" />
          <p className="font-medium">등록된 네트워크 장비가 없습니다.</p>
          <p className="text-xs opacity-70 mt-1">상단의 '장치 추가' 버튼으로 스위치/라우터를 등록하세요.</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4 gap-4">
          {devices.map(dev => {
            const status = statusMap.get(dev.ID)
            if (!status) return (
              <div key={dev.ID} className="bg-slate-900 rounded-3xl border border-slate-800 p-5 shadow-sm">
                <div className="flex items-start justify-between mb-4">
                  <div className="flex items-center gap-3 min-w-0">
                    <RefreshCcw size={14} className="text-indigo-400 animate-spin shrink-0" />
                    <div className="min-w-0">
                      <h4 className="font-bold text-slate-100 leading-tight truncate">{dev.Name}</h4>
                      <span className="text-[11px] font-bold text-slate-400 font-mono">{dev.IP}</span>
                    </div>
                  </div>
                </div>
                <div className="text-[11px] text-slate-400 leading-relaxed">
                  📡 첫 SNMP 폴링 대기 중...
                  <div className="text-[10px] text-slate-500 mt-1">
                    응답 도착 시 즉시 갱신됨 (최대 30초)
                  </div>
                </div>
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
      )}
    </div>
  )
}
