import { useState } from 'react'
import { X } from 'lucide-react'
import { changePassword } from '../api/auth'
import { errorText } from '../api/users'

const inputCls = 'w-full px-3 py-2 rounded-lg bg-base-800 border border-base-700 text-sm text-base-100 focus:outline-none focus:border-brand-500'

export default function PasswordModal({ onClose }: { onClose: () => void }) {
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [done, setDone] = useState(false)
  const [saving, setSaving] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (next !== confirm) {
      setError('새 비밀번호가 서로 다릅니다')
      return
    }
    setSaving(true)
    setError('')
    try {
      await changePassword(current, next)
      setDone(true)
    } catch (err) {
      setError(errorText(err, '비밀번호를 바꾸지 못했습니다'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="fixed inset-0 z-[60] bg-black/50 flex items-center justify-center p-4" onClick={onClose}>
      <div role="dialog" aria-label="비밀번호 변경" className="w-full max-w-sm bg-base-900 border border-base-700 rounded-xl p-5 space-y-4" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between">
          <h2 className="text-base font-bold text-base-100">비밀번호 변경</h2>
          <button type="button" onClick={onClose} aria-label="닫기" className="p-1 rounded-lg text-base-400 hover:bg-base-800"><X size={18} /></button>
        </div>
        {done ? (
          <div className="space-y-4">
            <p className="text-sm text-emerald-300">비밀번호를 바꿨습니다. 다음 로그인부터 새 비밀번호를 쓰세요.</p>
            <button type="button" onClick={onClose} className="w-full py-2 rounded-lg bg-brand-600 hover:bg-brand-500 text-white text-sm font-semibold">확인</button>
          </div>
        ) : (
          <form onSubmit={submit} className="space-y-3">
            <label className="block space-y-1">
              <span className="text-xs font-semibold text-base-400">현재 비밀번호</span>
              <input type="password" autoComplete="current-password" value={current} onChange={e => setCurrent(e.target.value)} className={inputCls} required />
            </label>
            <label className="block space-y-1">
              <span className="text-xs font-semibold text-base-400">새 비밀번호</span>
              <input type="password" autoComplete="new-password" value={next} onChange={e => setNext(e.target.value)} className={inputCls} required />
              <span className="text-xs text-base-500">9자 이상, 영문·숫자·특수문자 중 2종류 이상</span>
            </label>
            <label className="block space-y-1">
              <span className="text-xs font-semibold text-base-400">새 비밀번호 확인</span>
              <input type="password" autoComplete="new-password" value={confirm} onChange={e => setConfirm(e.target.value)} className={inputCls} required />
            </label>
            {error && <p role="alert" className="text-sm text-rose-300">{error}</p>}
            <button type="submit" disabled={saving} className="w-full py-2 rounded-lg bg-brand-600 hover:bg-brand-500 text-white text-sm font-semibold disabled:opacity-50">
              {saving ? '바꾸는 중…' : '변경'}
            </button>
          </form>
        )}
      </div>
    </div>
  )
}
