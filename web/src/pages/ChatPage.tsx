import { useState, useRef, useEffect } from 'react'
import { sendChat, type ChatMessage } from '../api/chat'
import { getLLMStatus } from '../api/llm'
import { useQuery } from '@tanstack/react-query'
import Markdown from '../components/Markdown'
import { Sparkles, Send, User, Bot, AlertCircle } from 'lucide-react'

// 서버의 chipRoutes와 글자까지 같아야 AI 없이 조회된다
const DATA_CHIPS = ['지금 서버 상태', '디스크 언제 차?', '인증서 만료 확인', '어제 알림 내역', 'AI 로그 분석 결과']

const SUGGESTIONS = [
  '서버 응답이 느릴 때 뭘 먼저 확인해?',
  '디스크가 가득 차면 어떻게 대처해?',
  'SSH 무차별 대입 공격을 막으려면?',
  'nginx 502 오류는 보통 왜 생겨?',
]

export default function ChatPage({ floating = false }: { floating?: boolean }) {
  // 메뉴 이동 후 돌아와도 대화 유지 (탭을 닫으면 초기화)
  const [messages, setMessages] = useState<ChatMessage[]>(() => {
    try {
      return JSON.parse(sessionStorage.getItem('owlmon-chat') || '[]')
    } catch {
      return []
    }
  })
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
    try {
      sessionStorage.setItem('owlmon-chat', JSON.stringify(messages))
    } catch {
      // 저장 실패(용량·사생활 보호 모드)는 무시 — 대화 자체는 계속 동작
    }
  }, [messages])

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: 'smooth' })
  }, [messages, loading])

  const send = async (text: string) => {
    const q = text.trim()
    if (!q || loading) return
    setError(null)
    const next: ChatMessage[] = [...messages, { role: 'user', content: q }]
    setMessages(next)
    setLoading(true)
    try {
      const res = await sendChat(next)
      setMessages([...next, { role: 'assistant', content: res.reply }])
    } catch (e: any) {
      setError(
        e?.code === 'ECONNABORTED'
          ? 'AI 응답이 너무 오래 걸립니다. 잠시 후 다시 시도하세요.'
          : 'AI 응답을 받지 못했습니다. 잠시 후 다시 시도하세요.',
      )
      setMessages(next)
    } finally {
      setLoading(false)
    }
  }

  const onSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    send(input)
    setInput('')
  }

  const disabled = llmStatus && !llmStatus.enabled

  return (
    <div className={floating ? 'h-full flex flex-col' : 'space-y-6'}>
      {/* Hero */}
      {!floating && <div className="flex items-center gap-3">
        <div className="w-10 h-10 rounded-xl bg-brand-600/20 flex items-center justify-center shrink-0">
          <Sparkles size={20} className="text-brand-400" />
        </div>
        <div>
          <h1 className="text-xl font-bold text-base-100">OWL AI</h1>
          <p className="text-base-400 text-sm mt-0.5">
            모니터링·서버·로그·보안 관련 질문에 한국어로 답해드립니다.
          </p>
        </div>
      </div>}

      {disabled && (
        <div className={`flex items-center gap-2 px-4 py-3 bg-amber-500/10 border border-amber-500/30 text-amber-300 text-sm ${floating ? 'm-3 rounded-lg' : 'rounded-xl'}`}>
          <AlertCircle size={16} className="shrink-0" />
          OWL AI가 꺼져 있습니다. 관리자(설치 업체)에게 활성화를 요청하세요.
        </div>
      )}

      {/* Chat window */}
      <div className={floating
        ? 'flex-1 min-h-0 flex flex-col'
        : 'bg-base-900 rounded-xl border border-base-800 flex flex-col h-[calc(100dvh-260px)] min-h-[420px]'}>
        <div ref={scrollRef} aria-live="polite" className="flex-1 overflow-y-auto p-5 space-y-4">
          {messages.length === 0 && !loading && (
            <div className="h-full flex flex-col items-center justify-center text-center gap-5">
              <Bot size={40} className="text-base-600" />
              <p className="text-base-500 text-sm">
                무엇이든 물어보세요. 모르는 용어도 풀어서 설명해 드립니다.
              </p>
              <div className="flex flex-wrap gap-2 justify-center max-w-lg">
                {SUGGESTIONS.map(s => (
                  <button
                    key={s}
                    onClick={() => send(s)}
                    disabled={disabled || loading}
                    className="px-3 py-1.5 rounded-full text-xs font-medium bg-base-800 text-base-300 border border-base-700 hover:border-brand-500 hover:text-base-50 transition-colors disabled:opacity-40"
                  >
                    {s}
                  </button>
                ))}
              </div>
            </div>
          )}

          {messages.map((m, i) => (
            <div key={i} className={m.role === 'user' ? 'flex justify-end' : 'flex justify-start'}>
              <div className={`flex gap-2.5 ${floating ? 'max-w-[90%]' : 'max-w-[80%]'} ${m.role === 'user' ? 'flex-row-reverse' : ''}`}>
                <div className={`w-7 h-7 rounded-lg flex items-center justify-center shrink-0 ${
                  m.role === 'user' ? 'bg-brand-600' : 'bg-base-700'
                }`}>
                  {m.role === 'user' ? <User size={15} className="text-white" /> : <Bot size={15} className="text-brand-300" />}
                </div>
                <div className={`px-4 py-2.5 rounded-xl text-sm leading-relaxed ${
                  m.role === 'user'
                    ? 'bg-brand-600 text-white rounded-tr-sm whitespace-pre-wrap'
                    : 'bg-base-800 text-base-200 rounded-tl-sm'
                }`}>
                  {m.role === 'user' ? m.content : <Markdown text={m.content} />}
                </div>
              </div>
            </div>
          ))}

          {loading && (
            <div className="flex justify-start">
              <div className="flex gap-2.5">
                <div className="w-7 h-7 rounded-lg bg-base-700 flex items-center justify-center shrink-0">
                  <Bot size={15} className="text-brand-300" />
                </div>
                <div className="px-4 py-3 rounded-xl rounded-tl-sm bg-base-800">
                  <div className="flex gap-1">
                    <span className="w-1.5 h-1.5 rounded-full bg-base-500 animate-pulse" style={{ animationDelay: '0ms' }} />
                    <span className="w-1.5 h-1.5 rounded-full bg-base-500 animate-pulse" style={{ animationDelay: '150ms' }} />
                    <span className="w-1.5 h-1.5 rounded-full bg-base-500 animate-pulse" style={{ animationDelay: '300ms' }} />
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
        <div className="border-t border-base-800 px-3 pt-3 flex flex-wrap gap-1.5 [@media(max-height:500px)]:flex-nowrap [@media(max-height:500px)]:overflow-x-auto">
          {DATA_CHIPS.map(c => (
            <button
              key={c}
              type="button"
              onClick={() => send(c)}
              disabled={disabled || loading}
              className="shrink-0 px-2.5 py-1 rounded-full text-xs font-medium bg-base-800 text-base-300 border border-base-700 hover:border-brand-500 hover:text-base-50 transition-colors disabled:opacity-40"
            >
              {c}
            </button>
          ))}
        </div>
        <form onSubmit={onSubmit} className="p-3 flex gap-2">
          <input
            value={input}
            onChange={e => setInput(e.target.value)}
            disabled={disabled || loading}
            aria-label="OWL AI에게 질문"
            placeholder={disabled ? 'OWL AI 비활성화 상태' : '질문을 입력하세요…'}
            className="flex-1 bg-base-800 text-base-100 placeholder-base-500 rounded-xl px-4 py-2.5 text-sm outline-none border border-base-700 focus:border-brand-500 transition-colors disabled:opacity-50"
          />
          <button
            type="submit"
            disabled={disabled || loading || !input.trim()}
            className="px-4 py-2.5 rounded-xl bg-brand-600 text-white font-semibold text-sm flex items-center gap-1.5 hover:bg-brand-500 transition-colors disabled:opacity-40"
          >
            <Send size={15} /> 전송
          </button>
        </form>
      </div>

      <p className={`text-xs text-base-500 text-center ${floating ? 'pb-2 [@media(max-height:500px)]:hidden' : ''}`}>
        서버 상태·사용률·디스크 예측·인증서·알림 기록·AI 로그 분석을 조회할 수 있습니다. 중요한 조치는 반드시 직접 확인 후 진행하세요.
      </p>
    </div>
  )
}
