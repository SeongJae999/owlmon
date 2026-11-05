import type { ReactNode } from 'react'

// ponytail: LLM 답변에 나오는 마크다운 일부(제목·굵게·목록·구분선·코드)만 처리. 표·링크가 필요해지면 react-markdown 도입
// HTML 문자열을 만들지 않고 React 요소로만 그려서 LLM 출력에 스크립트가 섞여도 실행되지 않음

function inline(text: string): ReactNode[] {
  return text.split(/(\*\*[^*]+\*\*|`[^`]+`)/g).filter(Boolean).map((part, i) => {
    if (part.startsWith('**') && part.endsWith('**')) {
      return <strong key={i} className="font-semibold text-slate-50">{part.slice(2, -2)}</strong>
    }
    if (part.startsWith('`') && part.endsWith('`')) {
      return <code key={i} className="px-1 py-0.5 rounded bg-slate-950/60 text-[0.9em] font-mono text-indigo-200">{part.slice(1, -1)}</code>
    }
    return part
  })
}

export default function Markdown({ text, className = '' }: { text: string; className?: string }) {
  const blocks: ReactNode[] = []
  let list: { ordered: boolean; items: string[] } | null = null
  let para: string[] = []

  const flushPara = () => {
    if (para.length) blocks.push(<p key={blocks.length}>{inline(para.join(' '))}</p>)
    para = []
  }
  const flushList = () => {
    if (!list) return
    const Tag = list.ordered ? 'ol' : 'ul'
    blocks.push(
      <Tag key={blocks.length} className={list.ordered ? 'list-decimal pl-5 space-y-1' : 'list-disc pl-5 space-y-1'}>
        {list.items.map((it, i) => <li key={i}>{inline(it)}</li>)}
      </Tag>,
    )
    list = null
  }

  for (const raw of text.split('\n')) {
    const line = raw.trim()
    const bullet = line.match(/^[*\-•]\s+(.*)$/)
    const numbered = line.match(/^\d+[.)]\s+(.*)$/)
    const heading = line.match(/^#{1,6}\s+(.*)$/) ?? line.match(/^\*\*([^*]+)\*\*:?$/) // 한 줄 전체가 굵게면 소제목으로

    if (!line) { flushPara(); flushList(); continue }
    if (/^(-{3,}|\*{3,}|_{3,})$/.test(line)) {
      flushPara(); flushList()
      blocks.push(<hr key={blocks.length} className="border-slate-700/60" />)
    } else if (heading) {
      flushPara(); flushList()
      blocks.push(<p key={blocks.length} className="font-semibold text-slate-50 pt-1">{inline(heading[1])}</p>)
    } else if (bullet || numbered) {
      flushPara()
      const ordered = !!numbered
      if (list && list.ordered !== ordered) flushList()
      list ??= { ordered, items: [] }
      list.items.push((bullet ?? numbered)![1])
    } else if (list && raw.startsWith(' ')) {
      // 들여쓴 줄은 직전 목록 항목의 이어지는 내용
      list.items[list.items.length - 1] += ' ' + line
    } else {
      flushList()
      para.push(line)
    }
  }
  flushPara(); flushList()

  return <div className={`space-y-2.5 break-words ${className}`}>{blocks}</div>
}
