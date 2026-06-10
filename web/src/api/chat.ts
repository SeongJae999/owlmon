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
  const res = await axios.post<ChatResult>('/api/chat', { messages })
  return res.data
}
