// 시각 토큰 단일 소스 — severity/level/status 계열 배지·차트가 모두 이 톤을 사용한다.
// 페이지마다 opacity(/10 vs /15)나 텍스트 톤(-300 vs -400)이 갈라지는 것을 방지.
// 도메인별 의미(라벨·아이콘)는 각 컴포넌트가 갖고, 색만 여기서 가져온다.
export const TONES = {
  rose:    { bg: 'bg-rose-500/15',    text: 'text-rose-300',    border: 'border-rose-500/30',    ring: 'ring-rose-500/30',    hex: '#f87171' },
  amber:   { bg: 'bg-amber-500/15',   text: 'text-amber-300',   border: 'border-amber-500/30',   ring: 'ring-amber-500/30',   hex: '#fbbf24' },
  emerald: { bg: 'bg-emerald-500/15', text: 'text-emerald-300', border: 'border-emerald-500/30', ring: 'ring-emerald-500/30', hex: '#34d399' },
  blue:    { bg: 'bg-blue-500/15',    text: 'text-blue-300',    border: 'border-blue-500/30',    ring: 'ring-blue-500/30',    hex: '#60a5fa' },
  sky:     { bg: 'bg-sky-500/15',     text: 'text-sky-300',     border: 'border-sky-500/30',     ring: 'ring-sky-500/30',     hex: '#38bdf8' },
  slate:   { bg: 'bg-slate-800',      text: 'text-slate-400',   border: 'border-slate-700',      ring: 'ring-slate-600/30',   hex: '#64748b' },
} as const

export type Tone = keyof typeof TONES

// 알림/메트릭 severity → tone 표준 매핑 (critical=rose, warning=amber, normal=emerald)
export const SEVERITY_TONE: Record<string, Tone> = {
  critical: 'rose',
  warning: 'amber',
  normal: 'emerald',
  info: 'blue',
  unknown: 'slate',
}
