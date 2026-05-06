# OWLmon

IT 전담 없는 학교/공공기관을 위한 서버 모니터링 솔루션.

## 주요 기능

- **서버 모니터링** — CPU, 메모리, 디스크, 네트워크 실시간 수집 (30초 주기)
- **이메일 알림** — 임계값 초과, 서버 다운, 서비스 장애 시 자동 알림 (디바운싱 + 중복 제거)
- **이상탐지** — Z-score 기반 비정상 패턴 감지, 선형회귀 디스크 고갈 예측
- **SNMP 네트워크 장비** — 스위치/라우터 트래픽 모니터링
- **SSL 인증서 만료 알림** — 등록된 도메인 인증서 자동 체크 (6시간 주기, 30일/7일 전 알림)
- **로그 수집** — 에이전트가 로그 파일을 tail하여 서버로 전송, 키워드 검색 지원
- **월간 보고서** — 호스트별 가동률/메트릭 요약, 이메일 발송
- **자산 관리** — 장비 IP, 위치, 보증 만료일 관리
- **한글 UI** — 비전문가도 이해할 수 있는 대시보드

## 기술 스택

| 구분 | 기술 |
|------|------|
| 에이전트 | Go, gopsutil, OpenTelemetry |
| 서버 API | Go, chi router, JWT |
| 프론트엔드 | React + TypeScript, Vite, Recharts |
| 시계열 DB | Prometheus |
| 관계형 DB | PostgreSQL |
| 인프라 | Docker Compose (Prometheus, OTel Collector, PostgreSQL, Grafana) |

## 빠른 시작

```bash
# 필수: Docker, Go, Node.js
make dev

# 대시보드: http://localhost:5173
# API 서버: http://localhost:8080
# 기본 계정: admin / admin
```

## 프로젝트 구조

```
owlmon/
├── agent/          # Go 에이전트 (메트릭 수집 + 로그 tail)
├── server/         # Go 서버 (API + 알림 + 이상탐지 + SSL 체크)
├── web/            # React 프론트엔드 (사이드바 네비게이션)
├── infra/          # Docker + PostgreSQL 스키마
├── docs/           # 기술 문서
└── docker-compose.yml
```

## 환경 변수

| 변수 | 설명 | 기본값 |
|------|------|--------|
| `OWLMON_JWT_SECRET` | JWT 시크릿 | (자동 생성) |
| `OWLMON_PASSWORD_HASH` | 로그인 비밀번호 해시 | (필수) |
| `POSTGRES_DSN` | PostgreSQL 연결 문자열 | (파일 폴백) |
| `SMTP_HOST` | 이메일 알림 SMTP 서버 | (비활성화) |
| `OWLMON_AGENT_KEY` | 로그 수집 에이전트 인증 키 | (비활성화) |

## 에이전트 설정 (config.yaml)

```yaml
otlp_endpoint: "localhost:4317"

checks:
  - name: "학교 홈페이지"
    type: http
    target: "https://school.go.kr"
    interval: 60s

logs:
  enabled: true
  server_url: "http://owlmon-server:8080"
  agent_key: "your-agent-key"
  tails:
    - name: syslog
      path: /var/log/syslog
      include_patterns: ["ERROR", "FATAL", "panic"]
```
