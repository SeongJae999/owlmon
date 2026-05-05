# OWLmon - Monitoring System Project

## Project Overview
OWLmon은 한국 시장을 타겟으로 한 모니터링 솔루션 프로젝트입니다.
"IT 전담 없는 학교/공공기관"을 주요 고객으로, Zabbix/Prometheus 기반 커스텀 대시보드 + 알림 연동을 핵심으로 합니다.

## Current Phase
- **MVP 개발 완료** — 서버 에이전트 + 웹 대시보드 + 이메일 알림 + SNMP + 이상탐지
- **SSL 인증서 만료 알림** ✅ — 서버 측 TLS 핸드쉐이크, 6시간 주기, 30일/7일/만료 알림
- **로그 수집** ✅ — 에이전트 tail → 서버 HTTP 전송 → PostgreSQL 저장/검색
- **UI/UX 개선** ✅ — 사이드바 네비게이션, CSS 디자인 시스템, 반응형
- `docs/monitoring-enhanced.html`: 13개 섹션의 종합 리서치 문서 (기술스택, 아키텍처, 논문, 로드맵)

## Tech Stack
- **Agent**: Go (gopsutil, OTLP push)
- **Backend API**: Go (chi router, JWT 인증)
- **Frontend**: React + TypeScript (Vite, Recharts)
- **TSDB**: Prometheus
- **DB**: PostgreSQL (알림 히스토리, SNMP 장비, 자산 관리, SSL 도메인, 로그)
- **Alerting**: SMTP 이메일
- **Protocols**: SNMP v2c, REST API, TLS (SSL 인증서 체크)
- **Anomaly Detection**: Z-score + 이동평균, 선형회귀 디스크 예측 (순수 Go, 외부 의존성 zero)
- **Log Collection**: 에이전트 파일 tail → HTTP 배치 전송 → PostgreSQL ILIKE 검색

## Architecture Pattern
```
에이전트 (메트릭) → OTLP → OTel Collector → Prometheus → 서버 (알림 + 이상탐지)
에이전트 (로그)   → HTTP POST /api/logs/ingest → 서버 → PostgreSQL
서버 (SSL 체크)   → TLS 핸드쉐이크 → 외부 도메인
서버 (API)        → React 대시보드
```

## Key Principles
- 에이전트는 경량 (목표 10MB 이하 메모리, CPU 1% 이하)
- API 호출은 Service Layer에 집중 (MonitoringService 패턴)
- OpenTelemetry 호환 우선 (OTLP 엔드포인트)
- 4 Golden Signals 기반 대시보드 설계 (Latency, Traffic, Errors, Saturation)
- 알림: 디바운싱 + 중복 제거 + 심각도 분류 필수

## MVP Roadmap
1. ~~**3개월**: 서버 에이전트 + 웹 대시보드 + 알림~~ ✅ 완료
2. ~~**6개월**: 월간 보고서 + SNMP 네트워크 모니터링~~ ✅ 완료
3. ~~이상탐지 Phase 1 (Z-score, 디스크 예측)~~ ✅ 완료
4. ~~SSL 인증서 만료 알림 + 로그 수집 + UI/UX 개선~~ ✅ 완료
5. **다음**: 모바일 최적화 + 카카오톡/슬랙 알림 연동

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
│   ├── logtail/                    # 로그 수집
│   │   ├── tailer.go               #   파일 tail + 패턴 필터 + 레벨 감지
│   │   └── sender.go               #   HTTP 배치 전송 (Agent Key 인증)
│   ├── config/                     # YAML 설정 (체크, 로그)
│   └── service/                    # Windows/Unix 서비스
├── server/                         # 서버 (Go)
│   ├── main.go                     # 엔트리포인트, 라우팅
│   ├── alert/                      # 알림 (Checker, State, Config, Email)
│   ├── anomaly/                    # 이상탐지 엔진
│   │   ├── detector.go             #   Z-score + 이동평균 (계절성 보정)
│   │   └── predictor.go            #   선형회귀 디스크 예측
│   ├── ssl/                        # SSL 인증서 체크
│   │   └── checker.go              #   TLS 핸드쉐이크, 6시간 주기
│   ├── auth/                       # JWT 인증
│   ├── db/                         # PostgreSQL 저장소
│   │   ├── ssl_domain_store.go     #   SSL 도메인 CRUD
│   │   └── log_store.go            #   로그 저장/검색/정리
│   ├── handler/                    # HTTP 핸들러
│   │   ├── ssl.go                  #   SSL 인증서 API
│   │   ├── log.go                  #   로그 수집/검색 API
│   │   └── ...
│   ├── report/                     # 월간 보고서
│   ├── snmp/                       # SNMP 폴러
│   └── service/                    # Windows/Unix 서비스
├── web/                            # 프론트엔드 (React + TypeScript)
│   └── src/
│       ├── api/                    # API 클라이언트
│       │   ├── ssl.ts              #   SSL 인증서 API
│       │   ├── logs.ts             #   로그 검색 API
│       │   └── ...
│       └── components/             # UI 컴포넌트 (사이드바 네비게이션)
│           ├── SSLDashboard.tsx    #   SSL 인증서 상태 페이지
│           ├── LogViewer.tsx       #   로그 검색/뷰어 페이지
│           └── ...
├── infra/                          # Docker + DB 스키마
│   └── postgres/init.sql           #   테이블: alert_*, assets, snmp_devices, ssl_domains, logs
├── docs/                           # 기술 문서
└── .claude/
    └── skills/                     # Claude Code custom skills
```

## Conventions

문서/코드/커밋/브랜치 규칙은 상위 디렉토리 `~/mango/CLAUDE.md`에 통합되어 있습니다.
이 프로젝트는 그 규칙을 그대로 따릅니다.
