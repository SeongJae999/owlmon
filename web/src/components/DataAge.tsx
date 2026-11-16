import { dataAge, type Tile } from '../hooks/useFleet'
import { cn } from '../utils/cn'

// 초로 먼저 반올림해야 '1분 60초' 같은 표시가 안 나옴
const fmt = (ms: number) => { const s = Math.round(ms / 1000); return s < 60 ? `${s}초` : `${Math.floor(s / 60)}분 ${s % 60}초` }

// 화면을 받아 온 시각이 아니라 장비에서 실제로 수집된 시각 기준 — 수집이 밀려도 '방금 갱신'으로 보이지 않게.
// 늦은 장비 수는 종류 카드에만 — 여기에도 두면 같은 숫자가 두 군데 보임
export default function DataAge({ tiles, now, className }: { tiles: Tile[]; now: number; className?: string }) {
  const { oldestMs } = dataAge(tiles, now)
  if (oldestMs == null) return null
  return (
    <span className={cn('inline-flex items-center gap-2 text-xs tabular-nums text-base-400', className)}
      title="서버·네트워크 장비 중 가장 오래된 수집 시각. 화면을 받아 온 시각이 아니라 장비에서 값이 수집된 시각입니다">
      수집 최대 {fmt(oldestMs)} 전
    </span>
  )
}
