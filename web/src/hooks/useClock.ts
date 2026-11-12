import { useEffect, useState } from 'react'

// 상황판 시계는 1초, 메인은 1분 — 매초 다시 그리면 화면 전체 집계를 매초 다시 계산하게 된다
export function useClock(ms: number) {
  const [now, setNow] = useState(() => new Date())
  useEffect(() => {
    const id = setInterval(() => setNow(new Date()), ms)
    return () => clearInterval(id)
  }, [ms])
  return now
}
