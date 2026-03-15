# OWLmon - Monitoring System Project

## Project Overview
OWLmon은 한국 시장을 타겟으로 한 모니터링 솔루션 프로젝트입니다.
"IT 전담 없는 중소기업"을 주요 고객으로, Zabbix/Prometheus 기반 커스텀 대시보드 + 카카오톡 알림 연동을 핵심으로 합니다.

## Current Phase
- **Research & Planning** (Pre-development)
- `monitoring-enhanced.html`: 13개 섹션의 종합 리서치 문서 (기술스택, 아키텍처, 논문, 로드맵)

## Tech Stack (Planned)
- **Agent**: Python (MVP) → Go (Production)
- **Backend API**: Python FastAPI (MVP) → Go (Production)
- **Frontend**: React/Vue + TypeScript
- **TSDB**: Prometheus TSDB (MVP) → VictoriaMetrics/ClickHouse (Scale)
- **Alerting**: Kakao Talk API, Kakao Work Webhook, Slack, SMS
- **Protocols**: SNMP v2c/v3, OTLP/gRPC, REST API, WebSocket
- **Infra**: Docker, OpenTelemetry Collector

## Architecture Pattern
OpenTelemetry 통합 파이프라인 (패턴 C) 기반, Push 모델 에이전트 채용

## Key Principles
- 에이전트는 경량 (목표 10MB 이하 메모리, CPU 1% 이하)
- API 호출은 Service Layer에 집중 (MonitoringService 패턴)
- OpenTelemetry 호환 우선 (OTLP 엔드포인트)
- 4 Golden Signals 기반 대시보드 설계 (Latency, Traffic, Errors, Saturation)
- 알림: 디바운싱 + 중복 제거 + 심각도 분류 필수

## MVP Roadmap
1. **3개월**: 서버 에이전트(CPU/메모리/디스크) + 웹 대시보드 + 카카오톡 알림
2. **6개월**: 모바일 최적화 + 월간 보고서 + SNMP 네트워크 모니터링
3. **1년**: AI 이상 감지 + SaaS 멀티테넌트 + OTel 호환 + 로그 수집

## Business Model
- 초기: 납품형 (구축비 500만~수천만원)
- 전환: SaaS 구독형 (서버 10대 기준 월 5만원)
- 전략: Sentry 모델 (설치 간편 → 즉시 가치 → 무료 티어 → 유료 전환)

## File Structure
```
owlmon/
├── CLAUDE.md                    # 이 파일
├── monitoring-enhanced.html     # 종합 리서치 문서
├── docs/                        # 기술 문서
│   └── TECH-STACK.md           # 기술스택 상세 정리
└── .claude/
    ├── settings.local.json
    └── skills/                  # Claude Code custom skills
        ├── monitoring-research.md
        ├── agent-dev.md
        └── alert-design.md
```

## Conventions
- 문서는 한국어 기본, 기술 용어는 영문 병기
- 코드 주석: 한국어
- Commit message: 영문
