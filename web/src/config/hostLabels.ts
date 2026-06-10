// 호스트 친화 라벨 — host_name → 역할 설명.
// 호스트 카드에 "willdev (개발/GitLab)"처럼 괄호로 표시한다.
//
// 참고: 자산관리(assets.description) 필드로 일반화하기 전까지 쓰는 간단 매핑.
// 새 호스트 별칭은 여기 한 줄만 추가하면 된다.
export const HOST_LABELS: Record<string, string> = {
  willdev: '개발/GitLab',
  willkomo: '운영 k3s',
  will: '사내 진입점/홈페이지',
  'hi-solution': 'HI사업/영상 GPU',
}

// hostLabel은 호스트의 역할 라벨을 반환한다 (없으면 undefined).
export function hostLabel(host: string): string | undefined {
  return HOST_LABELS[host]
}
