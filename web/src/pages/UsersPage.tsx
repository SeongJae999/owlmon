import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Users, Plus, KeyRound, Ban, RotateCcw, ShieldCheck, Eye, X } from 'lucide-react'
import ConfirmDialog from '../components/ConfirmDialog'
import PageToolbar from '../components/PageToolbar'
import { listUsers, createUser, updateUser, errorText, type User } from '../api/users'
import { getMe } from '../api/auth'
import { TONES } from '../utils/severity'
import { cn } from '../utils/cn'

const ROLE_LABEL: Record<User['role'], string> = { admin: '관리자', viewer: '조회 전용' }
const inputCls = 'w-full px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-sm text-slate-100 focus:outline-none focus:border-indigo-500'

export default function UsersPage() {
  const qc = useQueryClient()
  const { data: me } = useQuery({ queryKey: ['me'], queryFn: getMe })
  const { data: users = [], isLoading, isError } = useQuery({ queryKey: ['users'], queryFn: listUsers })
  const [adding, setAdding] = useState(false)
  const [form, setForm] = useState({ username: '', password: '', role: 'viewer' as User['role'] })
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  const refresh = () => qc.invalidateQueries({ queryKey: ['users'] })

  const run = async (fn: () => Promise<unknown>, ok: string, fail: string) => {
    setError('')
    setNotice('')
    try {
      await fn()
      setNotice(ok)
      refresh()
      return true
    } catch (e) {
      setError(errorText(e, fail))
      return false
    }
  }

  const submitAdd = async (e: React.FormEvent) => {
    e.preventDefault()
    if (await run(() => createUser(form), `${form.username} 계정을 추가했습니다`, '계정을 추가하지 못했습니다')) {
      setForm({ username: '', password: '', role: 'viewer' })
      setAdding(false)
    }
  }

  // 위험 동작(사용 중지·내 권한 낮추기)은 확인을 한 번 더 받음
  const [confirm, setConfirm] = useState<{ title: string; message: string; label: string; action: () => void } | null>(null)
  const [resetFor, setResetFor] = useState<User | null>(null)
  const [newPw, setNewPw] = useState('')

  const setRole = (u: User, role: User['role']) => {
    if (role === u.role) return
    const apply = () => run(() => updateUser(u.id, { role, disabled: u.disabled }), `${u.username} 역할을 ${role === 'admin' ? '관리자로' : '조회 전용으로'} 바꿨습니다`, '역할을 바꾸지 못했습니다')
    if (u.username === me?.username && role === 'viewer') {
      setConfirm({ title: '내 권한 낮추기', message: '조회 전용으로 바꾸면 이 화면을 포함한 설정을 더 이상 바꿀 수 없습니다.', label: '조회 전용으로 변경', action: apply })
    } else apply()
  }

  const toggleDisabled = (u: User) => {
    const apply = () => run(() => updateUser(u.id, { role: u.role, disabled: !u.disabled }),
      u.disabled ? `${u.username} 계정을 다시 사용합니다` : `${u.username} 계정을 중지했습니다 — 즉시 로그아웃됩니다`, '변경하지 못했습니다')
    if (u.disabled) apply()
    else setConfirm({ title: '계정 사용 중지', message: `${u.username} 계정은 즉시 로그아웃되고 다시 로그인할 수 없습니다. 기록은 그대로 남고 언제든 다시 사용할 수 있습니다.`, label: '사용 중지', action: apply })
  }

  const submitReset = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!resetFor) return
    const u = resetFor
    if (await run(() => updateUser(u.id, { role: u.role, disabled: u.disabled, password: newPw }), `${u.username} 비밀번호를 초기화했습니다 — 본인에게 전달 후 변경하도록 안내하세요`, '비밀번호를 초기화하지 못했습니다')) {
      setResetFor(null)
      setNewPw('')
    }
  }

  if (me && !me.multi_user) {
    return (
      <div className="space-y-5">
        <PageToolbar icon={Users} title="사용자 관리" />
        <p className="text-sm text-slate-400">DB 없이 실행 중이라 환경변수의 관리자 계정 하나만 사용할 수 있습니다.</p>
      </div>
    )
  }

  return (
    <div className="space-y-5">
      <PageToolbar icon={Users} title="사용자 관리" description="담당자·업체·상급자에게 각자 계정을 주고, 보기만 할 사람은 조회 전용으로 만드세요">
        <button
          type="button"
          onClick={() => setAdding(a => !a)}
          className="flex items-center gap-2 px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-semibold"
        >
          <Plus size={16} /> 계정 추가
        </button>
      </PageToolbar>

      {notice && <div className={cn('rounded-xl border px-4 py-2.5 text-sm', TONES.emerald.bg, TONES.emerald.border, TONES.emerald.text)}>{notice}</div>}
      {error && <div role="alert" className={cn('rounded-xl border px-4 py-2.5 text-sm', TONES.rose.bg, TONES.rose.border, TONES.rose.text)}>{error}</div>}

      {adding && (
        <form onSubmit={submitAdd} className="rounded-xl border border-slate-800 bg-slate-900 p-4 grid grid-cols-1 sm:grid-cols-4 gap-3 items-end">
          <label className="space-y-1">
            <span className="text-xs font-semibold text-slate-400">아이디</span>
            <input value={form.username} onChange={e => setForm({ ...form, username: e.target.value })} className={inputCls} placeholder="영문·숫자 3~32자" required />
          </label>
          <label className="space-y-1">
            <span className="text-xs font-semibold text-slate-400">초기 비밀번호</span>
            <input type="password" autoComplete="new-password" value={form.password} onChange={e => setForm({ ...form, password: e.target.value })} className={inputCls} placeholder="9자 이상, 2종류 이상" required />
          </label>
          <label className="space-y-1">
            <span className="text-xs font-semibold text-slate-400">역할</span>
            <select value={form.role} onChange={e => setForm({ ...form, role: e.target.value as User['role'] })} className={inputCls}>
              <option value="viewer">조회 전용 — 보기·알림 확인만</option>
              <option value="admin">관리자 — 설정 변경 가능</option>
            </select>
          </label>
          <button type="submit" className="py-2 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-semibold">추가</button>
        </form>
      )}

      {isLoading ? (
        <div className="py-12 text-center text-sm text-slate-500">불러오는 중...</div>
      ) : isError ? (
        <div role="alert" className="py-12 text-center text-sm text-rose-300">계정 목록을 불러오지 못했습니다</div>
      ) : (
        <ul className="rounded-xl border border-slate-800 bg-slate-900/50 divide-y divide-slate-800">
          {users.map(u => {
            const isMe = u.username === me?.username
            return (
              <li key={u.id} className="px-4 py-3.5 flex flex-col md:flex-row md:items-center gap-3">
                <div className={cn('flex items-center gap-3 flex-1 min-w-0', u.disabled && 'opacity-50')}>
                  <div className={cn(
                    'w-9 h-9 rounded-full flex items-center justify-center text-sm font-bold uppercase shrink-0',
                    u.role === 'admin' ? 'bg-indigo-500/15 text-indigo-300' : 'bg-slate-800 text-slate-400',
                  )}>
                    {u.username[0]}
                  </div>
                  <div className="min-w-0">
                    <div className="flex items-center gap-1.5">
                      <span className="text-sm font-semibold text-slate-100 truncate">{u.username}</span>
                      {isMe && <span className="px-1.5 py-0.5 rounded-md bg-indigo-500/15 text-indigo-300 text-[11px] font-semibold">나</span>}
                      {u.disabled && <span className="px-1.5 py-0.5 rounded-md bg-slate-800 text-slate-400 text-[11px] font-semibold">사용 중지</span>}
                    </div>
                    <p className="text-xs text-slate-500 mt-0.5">{new Date(u.created_at).toLocaleDateString('ko-KR')} 등록</p>
                  </div>
                </div>

                <div className="flex flex-wrap items-center gap-2">
                  {/* 역할 — 셀렉트 대신 두 칸 토글: 현재 값이 한눈에 보이고 브라우저 기본 화살표가 없음 */}
                  <div role="radiogroup" aria-label={`${u.username} 역할`} className="inline-flex h-8 p-0.5 rounded-lg bg-slate-800 border border-slate-700">
                    {(['admin', 'viewer'] as const).map(role => {
                      const Icon = role === 'admin' ? ShieldCheck : Eye
                      const on = u.role === role
                      return (
                        <button
                          key={role}
                          type="button"
                          role="radio"
                          aria-checked={on}
                          disabled={u.disabled}
                          onClick={() => setRole(u, role)}
                          className={cn(
                            'inline-flex items-center gap-1 px-2.5 rounded-md text-xs font-semibold transition-colors disabled:cursor-not-allowed',
                            on ? 'bg-indigo-600 text-white shadow-sm' : 'text-slate-400 hover:text-slate-200 disabled:hover:text-slate-400',
                          )}
                        >
                          <Icon size={13} /> {ROLE_LABEL[role]}
                        </button>
                      )
                    })}
                  </div>

                  <button
                    type="button"
                    onClick={() => { setResetFor(u); setNewPw('') }}
                    disabled={u.disabled}
                    className="inline-flex items-center gap-1.5 h-8 px-3 rounded-lg border border-slate-700 bg-slate-800/60 text-xs font-semibold text-slate-200 hover:bg-slate-700 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                  >
                    <KeyRound size={13} /> 비밀번호 초기화
                  </button>

                  {u.disabled ? (
                    <button
                      type="button"
                      onClick={() => toggleDisabled(u)}
                      className="inline-flex items-center gap-1.5 h-8 px-3 rounded-lg border border-emerald-500/30 bg-emerald-500/10 text-xs font-semibold text-emerald-300 hover:bg-emerald-500/20 transition-colors"
                    >
                      <RotateCcw size={13} /> 다시 사용
                    </button>
                  ) : (
                    <button
                      type="button"
                      onClick={() => toggleDisabled(u)}
                      disabled={isMe}
                      title={isMe ? '내 계정은 중지할 수 없습니다 — 다른 관리자가 중지해야 합니다' : undefined}
                      className="inline-flex items-center gap-1.5 h-8 px-3 rounded-lg border border-rose-500/30 text-xs font-semibold text-rose-300 hover:bg-rose-500/10 transition-colors disabled:opacity-30 disabled:cursor-not-allowed disabled:hover:bg-transparent"
                    >
                      <Ban size={13} /> 사용 중지
                    </button>
                  )}
                </div>
              </li>
            )
          })}
        </ul>
      )}
      <p className="text-xs text-slate-500">계정은 기록 보존을 위해 삭제하지 않고 사용 중지합니다. 모든 변경은 사용 기록(감사 로그)에 남습니다.</p>

      <ConfirmDialog
        open={!!confirm}
        title={confirm?.title}
        message={confirm?.message ?? ''}
        confirmLabel={confirm?.label}
        onConfirm={() => { confirm?.action(); setConfirm(null) }}
        onCancel={() => setConfirm(null)}
      />

      {resetFor && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4" onClick={() => setResetFor(null)}>
          <form
            onSubmit={submitReset}
            role="dialog"
            aria-label="비밀번호 초기화"
            onClick={e => e.stopPropagation()}
            onKeyDown={e => e.key === 'Escape' && setResetFor(null)}
            className="w-full max-w-sm bg-slate-900 border border-slate-800 rounded-2xl shadow-2xl p-5 space-y-4"
          >
            <div className="flex items-center justify-between">
              <h2 className="text-base font-bold text-slate-100">{resetFor.username} 비밀번호 초기화</h2>
              <button type="button" onClick={() => setResetFor(null)} aria-label="닫기" className="p-1 rounded-md text-slate-500 hover:text-slate-300 hover:bg-slate-800"><X size={16} /></button>
            </div>
            <label className="block space-y-1">
              <span className="text-xs font-semibold text-slate-400">새 비밀번호</span>
              <input type="password" autoComplete="new-password" autoFocus value={newPw} onChange={e => setNewPw(e.target.value)} className={inputCls} required />
              <span className="text-[11px] text-slate-500">9자 이상, 영문·숫자·특수문자 중 2종류 이상. 본인에게 전달한 뒤 바로 바꾸도록 안내하세요.</span>
            </label>
            <div className="flex justify-end gap-2">
              <button type="button" onClick={() => setResetFor(null)} className="h-9 px-4 rounded-lg text-sm font-semibold bg-slate-800 text-slate-300 border border-slate-700 hover:bg-slate-700">취소</button>
              <button type="submit" className="h-9 px-4 rounded-lg text-sm font-semibold bg-indigo-600 text-white hover:bg-indigo-500">초기화</button>
            </div>
          </form>
        </div>
      )}
    </div>
  )
}
