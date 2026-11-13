import { useEffect } from 'react'
import { useSearchParams } from 'react-router-dom'
import { cn } from '../utils/cn'

// 처리할 문제에서 장비를 눌렀을 때 목록 화면이 아니라 그 장비 카드로 바로 가게 (?focus=장비키)
export function useFocus(ready: boolean) {
  const [params] = useSearchParams()
  const focus = params.get('focus')
  useEffect(() => {
    if (ready && focus) document.getElementById(`focus-${focus}`)?.scrollIntoView({ block: 'center' })
  }, [ready, focus])
  // grid로 감싸야 카드가 그리드 칸 높이를 그대로 채움
  return (key: string | number) => ({
    id: `focus-${key}`,
    className: cn('grid', focus === String(key) && 'rounded-xl ring-2 ring-brand-400 ring-offset-2 ring-offset-base-950'),
  })
}
