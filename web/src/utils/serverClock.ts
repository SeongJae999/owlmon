// 서버 시계 − 브라우저 시계(ms). 망분리 관제 PC는 NTP가 없어 시계가 수십 초~수 분 틀어짐 —
// 서버가 찍은 시각(점검·수집 시각)을 브라우저 시계로 바로 빼면 그 차이가 '수집 지연'으로 보임
let offset = 0

// 응답 Date 헤더(초 단위)로 맞춤 — ±1초 정도면 지연 판정(수십 초 단위)에 충분
export function noteServerDate(date: string | undefined) {
  const t = date ? Date.parse(date) : NaN
  if (!Number.isNaN(t)) offset = t - Date.now()
}

// 서버 시계 기준 시각 → 브라우저 시계 기준 시각
export const serverToLocal = (serverMs: number) => serverMs - offset
