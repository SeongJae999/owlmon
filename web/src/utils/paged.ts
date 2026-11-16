export const PAGE_SIZE = 50

// 목록이 줄면 보던 페이지가 끝을 넘을 수 있어 늘 마지막 페이지 안으로 당긴다
export function paged<T>(rows: T[], page: number) {
  const p = Math.min(page, Math.max(0, Math.ceil(rows.length / PAGE_SIZE) - 1))
  return { page: p, rows: rows.slice(p * PAGE_SIZE, (p + 1) * PAGE_SIZE) }
}
