# OWLmon

IT 전담 없는 중소기업을 위한 서버 모니터링 솔루션.

## 목표
- 설치 5분 만에 대시보드가 뜨는 간편함
- 카카오톡으로 장애 알림
- 비전문가도 이해할 수 있는 한글 UI

## 프로젝트 구조
```
owlmon/
├── src/
│   ├── agent/     # 서버에 설치하는 수집 에이전트
│   ├── api/       # 메트릭 수신/조회 백엔드 서버
│   └── web/       # 대시보드 프론트엔드
├── docs/          # 기술 문서
└── monitoring-enhanced.html  # 리서치 자료
```

## 개발 현황
- [x] Step 0: 프로젝트 기반 세팅
- [ ] Step 1: Uptime Kuma 설치 & 분석
- [ ] Step 2: Python 에이전트 (CPU/메모리/디스크 수집)
- [ ] Step 3: FastAPI 서버 + 웹 대시보드
