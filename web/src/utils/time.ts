export function lasting(iso: string, now: Date) {
  const m = Math.max(0, Math.floor((now.getTime() - new Date(iso).getTime()) / 60000))
  if (m < 1) return '방금 시작'
  if (m < 60) return `${m}분째`
  if (m < 1440) return `${Math.floor(m / 60)}시간 ${m % 60}분째`
  return `${Math.floor(m / 1440)}일째`
}

export function ago(iso: string, now: Date) {
  const m = Math.max(0, Math.round((now.getTime() - new Date(iso).getTime()) / 60000))
  if (m < 1) return '방금'
  if (m < 60) return `${m}분 전`
  if (m < 1440) return `${Math.floor(m / 60)}시간 전`
  return `${Math.floor(m / 1440)}일 전`
}
