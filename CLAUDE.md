# OWLmon - Monitoring System Project

## Project Overview
OWLmon은 한국 시장을 타겟으로 한 모니터링 솔루션 프로젝트입니다.
"IT 전담 없는 학교/공공기관"을 주요 고객으로, Zabbix/Prometheus 기반 커스텀 대시보드 + 알림 연동을 핵심으로 합니다.

## Current Phase
- **MVP 개발 완료** — 서버 에이전트 + 웹 대시보드 + 이메일 알림 + SNMP + 이상탐지
- `docs/monitoring-enhanced.html`: 13개 섹션의 종합 리서치 문서 (기술스택, 아키텍처, 논문, 로드맵)

## Tech Stack
- **Agent**: Go (gopsutil, OTLP push)
- **Backend API**: Go (chi router, JWT 인증)
- **Frontend**: React + TypeScript (Vite, Recharts)
- **TSDB**: Prometheus
- **DB**: PostgreSQL (알림 히스토리, SNMP 장비, 자산 관리)
- **Alerting**: SMTP 이메일
- **Protocols**: SNMP v2c, REST API
- **Anomaly Detection**: Z-score + 이동평균, 선형회귀 디스크 예측 (순수 Go, 외부 의존성 zero)

## Architecture Pattern
Push 모델 에이전트 → Prometheus → OWLmon 서버 (알림 + 이상탐지 + API) → React 대시보드

## Key Principles
- 에이전트는 경량 (목표 10MB 이하 메모리, CPU 1% 이하)
- API 호출은 Service Layer에 집중 (MonitoringService 패턴)
- OpenTelemetry 호환 우선 (OTLP 엔드포인트)
- 4 Golden Signals 기반 대시보드 설계 (Latency, Traffic, Errors, Saturation)
- 알림: 디바운싱 + 중복 제거 + 심각도 분류 필수

## MVP Roadmap
1. ~~**3개월**: 서버 에이전트 + 웹 대시보드 + 알림~~ ✅ 완료
2. ~~**6개월**: 월간 보고서 + SNMP 네트워크 모니터링~~ ✅ 완료
3. **현재**: 이상탐지 Phase 1 (Z-score, 디스크 예측) ✅ 완료 → Phase 2 (Isolation Forest) 대기
4. **다음**: SSL 인증서 만료 알림 + 로그 수집

## Business Model
- 초기: 납품형 (구축비 500만~수천만원)
- 전환: SaaS 구독형 (서버 10대 기준 월 5만원)
- 전략: Sentry 모델 (설치 간편 → 즉시 가치 → 무료 티어 → 유료 전환)

## File Structure
```
owlmon/
├── CLAUDE.md                       # 이 파일
├── agent/                          # 에이전트 (Go)
│   ├── main.go                     # 엔트리포인트
│   ├── collector/                  # 메트릭 수집 (CPU, 메모리, 디스크, 네트워크)
│   └── service/                    # Windows/Unix 서비스
├── server/                         # 서버 (Go)
│   ├── main.go                     # 엔트리포인트, 라우팅
│   ├── alert/                      # 알림 (Checker, State, Config, Email)
│   ├── anomaly/                    # 이상탐지 엔진
│   │   ├── detector.go             #   Z-score + 이동평균 (계절성 보정)
│   │   └── predictor.go            #   선형회귀 디스크 예측
│   ├── auth/                       # JWT 인증
│   ├── db/                         # PostgreSQL 저장소
│   ├── handler/                    # HTTP 핸들러
│   │   ├── anomaly.go              #   이상탐지 API
│   │   └── ...
│   ├── report/                     # 월간 보고서
│   ├── snmp/                       # SNMP 폴러
│   └── service/                    # Windows/Unix 서비스
├── web/                            # 프론트엔드 (React + TypeScript)
│   └── src/
│       ├── api/                    # API 클라이언트
│       │   ├── anomaly.ts          #   이상탐지 API
│       │   └── ...
│       └── components/             # UI 컴포넌트
│           ├── AnomalyPanel.tsx    #   이상탐지 패널
│           └── ...
├── docs/                           # 기술 문서
└── .claude/
    └── skills/                     # Claude Code custom skills
```

## Conventions
- 문서는 한국어 기본, 기술 용어는 영문 병기
- 코드 주석: 한국어
- Commit message: Conventional Commits (한국어)
  - 형식: `<type>(<scope>): <subject>`
  - type: feat, fix, docs, style, refactor, test, chore, perf, ci, build, revert
  - scope: agent, api, web, infra, ci 등 (선택)
  - subject: 한국어, 마침표 없음
  - body: 선택, "왜" 변경했는지
