import React, { useState } from 'react'
import { login } from '../api/auth'
import { Lock, User, LogIn, AlertCircle, RefreshCcw } from 'lucide-react'
import { cn } from '../utils/cn'

interface Props {
  onLogin: () => void
}

export default function LoginPage({ onLogin }: Props) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await login(username, password)
      onLogin()
    } catch {
      setError('아이디 또는 비밀번호가 올바르지 않습니다.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="fixed inset-0 flex items-center justify-center bg-slate-950">
      {/* Abstract Background Elements */}
      <div className="absolute top-1/4 -left-20 w-96 h-96 bg-indigo-600/20 rounded-full blur-[120px]" />
      <div className="absolute bottom-1/4 -right-20 w-96 h-96 bg-purple-600/10 rounded-full blur-[120px]" />

      <div className="w-full max-w-md px-6 z-10">
        <div className="bg-slate-900 rounded-[32px] shadow-2xl overflow-hidden border border-slate-800">
          <div className="p-8 sm:p-12">
            {/* Logo & Header */}
            <div className="flex flex-col items-center text-center mb-10">
              <img
                src="/favicon.svg"
                alt="Willkomo OWLmon"
                className="w-14 h-14 mb-4 shadow-xl shadow-indigo-500/30 rounded-2xl hover:scale-105 hover:shadow-indigo-500/50 transition-all duration-300"
              />
              <h1 className="text-2xl font-bold text-slate-100 mb-1">Willkomo</h1>
              <p className="text-sm font-medium text-slate-400">통합 모니터링</p>
            </div>

            <form onSubmit={handleSubmit} className="space-y-6">
              <div className="space-y-4">
                {/* Username */}
                <div className="space-y-1.5 group">
                  <label className="text-xs font-semibold text-slate-400 ml-1 group-focus-within:text-indigo-400 transition-colors">아이디</label>
                  <div className="relative">
                    <User className="absolute left-4 top-3 text-slate-500 group-focus-within:text-indigo-400 transition-colors" size={18} />
                    <input
                      type="text"
                      className="w-full bg-slate-800 border border-slate-800 rounded-2xl pl-12 pr-4 py-3 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition-all font-medium text-slate-100 placeholder:text-slate-500"
                      placeholder="관리자 아이디"
                      value={username}
                      onChange={(e) => setUsername(e.target.value)}
                      autoComplete="username"
                      required
                    />
                  </div>
                </div>

                {/* Password */}
                <div className="space-y-1.5 group">
                  <label className="text-xs font-semibold text-slate-400 ml-1 group-focus-within:text-indigo-400 transition-colors">비밀번호</label>
                  <div className="relative">
                    <Lock className="absolute left-4 top-3 text-slate-500 group-focus-within:text-indigo-400 transition-colors" size={18} />
                    <input
                      type="password"
                      className="w-full bg-slate-800 border border-slate-800 rounded-2xl pl-12 pr-4 py-3 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 transition-all font-medium text-slate-100 placeholder:text-slate-500"
                      placeholder="비밀번호"
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      autoComplete="current-password"
                      required
                    />
                  </div>
                </div>
              </div>

              {error && (
                <div className="flex items-center gap-3 p-3 bg-rose-500/10 border border-rose-500/30 rounded-xl text-rose-300 text-xs font-semibold">
                  <AlertCircle size={16} className="shrink-0" />
                  <p>{error}</p>
                </div>
              )}

              <button
                type="submit"
                disabled={loading}
                className={cn(
                  "w-full flex items-center justify-center gap-3 py-4 rounded-2xl text-sm font-bold transition-all shadow-xl",
                  loading ? "bg-slate-800 text-slate-400 cursor-not-allowed" : "bg-indigo-600 text-white hover:bg-indigo-700 hover:-translate-y-0.5 active:translate-y-0 shadow-indigo-500/30"
                )}
              >
                {loading ? (
                  <RefreshCcw size={18} className="animate-spin" />
                ) : (
                  <LogIn size={18} />
                )}
                {loading ? '인증 중...' : '시스템 로그인'}
              </button>
            </form>
          </div>

          <div className="p-5 bg-slate-900/60 text-center border-t border-slate-800">
            <p className="text-xs font-medium text-slate-500">
              &copy; 2026 OWLmon — Open Source Monitoring
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}
