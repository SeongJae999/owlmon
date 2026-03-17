---
name: dashboard-ui
description: OWLmon 대시보드 프론트엔드 UI 개발 가이드
user_invocable: true
---

# Dashboard UI Skill

OWLmon 모니터링 대시보드 프론트엔드 개발을 지원합니다.

## 수행 범위

### 대시보드 설계
- **4 Golden Signals 패널**: Latency, Traffic, Errors, Saturation
- **서버 헬스 그리드**: 전체 서버 상태 한눈에
- **실시간 갱신**: WebSocket 기반 라이브 업데이트
- **타임라인**: 시계열 그래프 (줌/팬 지원)
- **알림 히스토리**: 발생/해소 타임라인

### 한국 시장 차별화 UI
- **한글 인터페이스**: 비전문가(공장 총무, 병원 원무과)도 이해 가능
- **서버 추가 마법사**: 단계별 가이드 (에이전트 설치 → 등록 → 설정)
- **월간 보고서**: 한글 요약 + 그래프, PDF 다운로드/인쇄 최적화
- **모바일 최적화**: PWA, 반응형 레이아웃

### 디자인 시스템
- `monitoring-enhanced.html`의 다크 테마 디자인 참고
- CSS Variables: `--bg`, `--accent`, `--accent2`, `--accent3`, `--warn`, `--danger`
- Fonts: Noto Sans KR (본문), JetBrains Mono (코드/수치), Bebas Neue (제목)
- 카드 기반 레이아웃, 그라데이션 보더

### 기술 선택
| 항목 | 선택지 | 비고 |
|------|--------|------|
| 프레임워크 | React 또는 Vue 3 | Grafana→React, Uptime Kuma→Vue |
| 언어 | TypeScript | 타입 안전성 |
| 차트 | Recharts / Visx / ECharts | 시계열 특화 |
| 상태관리 | Zustand / Pinia | 경량 |
| 실시간 | Socket.IO / native WebSocket | Uptime Kuma 참고 |
| 빌드 | Vite | 빠른 HMR |

### 코드 구조
```
src/web/
├── components/
│   ├── Dashboard/         # 메인 대시보드
│   ├── ServerHealth/      # 서버 상태 그리드
│   ├── Charts/            # 시계열 차트 컴포넌트
│   ├── AlertHistory/      # 알림 히스토리
│   └── SetupWizard/       # 서버 추가 마법사
├── hooks/                 # WebSocket, 데이터 페칭
├── services/
│   └── MonitoringService  # API 호출 집중 (Service Layer 패턴)
├── styles/                # 디자인 시스템
└── pages/                 # 라우트별 페이지
```

## 핵심 원칙
- API 호출은 반드시 `MonitoringService`를 통해 (직접 호출 금지)
- Zabbix → Prometheus 백엔드 전환 시 Service Layer만 수정
- 비전문가도 5분 안에 이해할 수 있는 UX
- 모바일 퍼스트 반응형
