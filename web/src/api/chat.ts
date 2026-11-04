import axios from 'axios'

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
}

export interface ChatResult {
  reply: string
  model: string
}

/** POST /api/chat — OWLmon 도우미 챗봇. 대화 히스토리 전체를 보내 맥락 유지(stateless). */
export async function sendChat(messages: ChatMessage[]): Promise<ChatResult> {
  const res = await axios.post<ChatResult>('/api/chat', { messages }, { timeout: 70_000 }) // 서버 LLM 타임아웃(60초)보다 약간 길게
  return res.data
}
