import { useState, useRef, useEffect } from 'react'
import { sendChat, type ChatMessage } from '../api/chat'
import { getLLMStatus } from '../api/llm'
import { useQuery } from '@tanstack/react-query'
import { Sparkles, Send, User, Bot, AlertCircle } from 'lucide-react'

const SUGGESTIONS = [
  'QMP 통신 지연이 뭐야?',
  '디스크가 가득 차면 어떻게 대처해?',
  'SSH 무차별 대입 공격을 막으려면?',
  'nginx 502 오류는 보통 왜 생겨?',
]

export default function ChatPage() {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [input, setInput] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const scrollRef = useRef<HTMLDivElement>(null)

  const { data: llmStatus } = useQuery({
    queryKey: ['llmStatus'],
    queryFn: getLLMStatus,
    staleTime: 5 * 60_000,
  })

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: 'smooth' })
  }, [messages, loading])

  const send = async (text: string) => {
    const q = text.trim()
    if (!q || loading) return
    setError(null)
    const next: ChatMessage[] = [...messages, { role: 'user', content: q }]
    setMessages(next)
    setInput('')
    setLoading(true)
    try {
      const res = await sendChat(next)
      setMessages([...next, { role: 'assistant', content: res.reply }])
    } catch (e: any) {
      setError(e?.response?.data?.toString?.() || e?.message || 'AI 응답 실패')
      setMessages(next)
    } finally {
      setLoading(false)
    }
  }

  const onSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    send(input)
  }

  const disabled = llmStatus && !llmStatus.enabled

  return (
    <div className="space-y-6">
      {/* Hero */}
      <div className="flex items-center gap-3">
        <div className="w-10 h-10 rounded-xl bg-indigo-600/20 flex items-center justify-center shrink-0">
          <Sparkles size={20} className="text-indigo-400" />
        </div>
        <div>
          <h1 className="text-xl font-bold text-slate-100">AI 도우미</h1>
          <p className="text-slate-400 text-sm mt-0.5">
            모니터링·서버·로그·보안 관련 질문에 한국어로 답해드립니다.
          </p>
        </div>
      </div>

      {disabled && (
        <div className="flex items-center gap-2 px-4 py-3 rounded-xl bg-amber-500/10 border border-amber-500/30 text-amber-300 text-sm">
          <AlertCircle size={16} />
          AI 도우미가 비활성화되어 있습니다 (서버에 OWLMON_LLM_PROVIDER 설정 필요).
        </div>
      )}

      {/* Chat window */}
      <div className="bg-slate-900 rounded-2xl border border-slate-800 flex flex-col h-[calc(100vh-260px)] min-h-[420px]">
        <div ref={scrollRef} className="flex-1 overflow-y-auto p-5 space-y-4">
          {messages.length === 0 && !loading && (
            <div className="h-full flex flex-col items-center justify-center text-center gap-5">
              <Bot size={40} className="text-slate-600" />
              <p className="text-slate-500 text-sm">
                무엇이든 물어보세요. 모르는 용어도 풀어서 설명해 드립니다.
              </p>
              <div className="flex flex-wrap gap-2 justify-center max-w-lg">
                {SUGGESTIONS.map(s => (
                  <button
                    key={s}
                    onClick={() => send(s)}
                    disabled={disabled || loading}
                    className="px-3 py-1.5 rounded-full text-xs font-medium bg-slate-800 text-slate-300 border border-slate-700 hover:border-indigo-500 hover:text-white transition-colors disabled:opacity-40"
                  >
                    {s}
                  </button>
                ))}
              </div>
              <p className="text-[11px] text-slate-600">
                ⚠ 실시간 서버 상태·수치는 아직 조회할 수 없어요. 그런 정보는 각 대시보드 메뉴를 봐주세요.
              </p>
            </div>
          )}

          {messages.map((m, i) => (
            <div key={i} className={m.role === 'user' ? 'flex justify-end' : 'flex justify-start'}>
              <div className={`flex gap-2.5 max-w-[80%] ${m.role === 'user' ? 'flex-row-reverse' : ''}`}>
                <div className={`w-7 h-7 rounded-lg flex items-center justify-center shrink-0 ${
                  m.role === 'user' ? 'bg-indigo-600' : 'bg-slate-700'
                }`}>
                  {m.role === 'user' ? <User size={15} className="text-white" /> : <Bot size={15} className="text-indigo-300" />}
                </div>
                <div className={`px-4 py-2.5 rounded-2xl text-sm leading-relaxed whitespace-pre-wrap ${
                  m.role === 'user'
                    ? 'bg-indigo-600 text-white rounded-tr-sm'
                    : 'bg-slate-800 text-slate-200 rounded-tl-sm'
                }`}>
                  {m.content}
                </div>
              </div>
            </div>
          ))}

          {loading && (
            <div className="flex justify-start">
              <div className="flex gap-2.5">
                <div className="w-7 h-7 rounded-lg bg-slate-700 flex items-center justify-center shrink-0">
                  <Bot size={15} className="text-indigo-300" />
                </div>
                <div className="px-4 py-3 rounded-2xl rounded-tl-sm bg-slate-800">
                  <div className="flex gap-1">
                    <span className="w-1.5 h-1.5 rounded-full bg-slate-500 animate-bounce" style={{ animationDelay: '0ms' }} />
                    <span className="w-1.5 h-1.5 rounded-full bg-slate-500 animate-bounce" style={{ animationDelay: '150ms' }} />
                    <span className="w-1.5 h-1.5 rounded-full bg-slate-500 animate-bounce" style={{ animationDelay: '300ms' }} />
                  </div>
                </div>
              </div>
            </div>
          )}

          {error && (
            <div className="flex items-center gap-2 px-4 py-2.5 rounded-xl bg-rose-500/10 border border-rose-500/30 text-rose-300 text-sm">
              <AlertCircle size={15} /> {error}
            </div>
          )}
        </div>

        {/* Input */}
        <form onSubmit={onSubmit} className="border-t border-slate-800 p-3 flex gap-2">
          <input
            value={input}
            onChange={e => setInput(e.target.value)}
            disabled={disabled || loading}
            placeholder={disabled ? 'AI 도우미 비활성화 상태' : '질문을 입력하세요…'}
            className="flex-1 bg-slate-800 text-slate-100 placeholder-slate-500 rounded-xl px-4 py-2.5 text-sm outline-none border border-slate-700 focus:border-indigo-500 transition-colors disabled:opacity-50"
          />
          <button
            type="submit"
            disabled={disabled || loading || !input.trim()}
            className="px-4 py-2.5 rounded-xl bg-indigo-600 text-white font-semibold text-sm flex items-center gap-1.5 hover:bg-indigo-500 transition-colors disabled:opacity-40"
          >
            <Send size={15} /> 전송
          </button>
        </form>
      </div>

      <p className="text-[11px] text-slate-600 text-center">
        AI 답변은 참고용입니다. 중요한 조치는 반드시 직접 확인 후 진행하세요.
      </p>
    </div>
  )
}
