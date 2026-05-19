# OWLmon

IT 전담 없는 학교/공공기관을 위한 서버 모니터링 솔루션.

## 주요 기능

- **서버 모니터링** — CPU, 메모리, 디스크, 네트워크 실시간 수집 (기본 15초 주기, `OWLMON_COLLECT_INTERVAL`로 조정)
- **호스트 인벤토리** — 에이전트 시작 시 CPU 모델/RAM/디스크(SSD·HDD)/OS/가상화 자동 수집·표시
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

## 지원 OS

OWLmon 에이전트는 **Zabbix Agent 2가 공식 지원하는 OS와 동일한 범위**를 지원 목표로 한다. Go + gopsutil 기반이라 빌드/실행은 대부분 OS에서 가능하며, 검증 상태만 OS별로 다르다.

### 에이전트 (모니터링 대상)

| OS / 배포판 | 상태 | 메트릭 | 호스트 스펙 | 검증 환경 |
|------------|------|--------|------------|----------|
| **Ubuntu** 18.04 / 20.04 / 22.04 / 24.04 LTS | ✅ 검증 | 풀 지원 | 풀 지원 | Docker (Ubuntu 24는 실 VM도 검증) |
| **Debian** 10 / 11 / 12 | ✅ 검증 | 풀 지원 | 풀 지원 | Docker (Debian 12는 Proxmox 실 호스트도) |
| **CentOS** 7 / Stream 8 | ✅ 검증 | 풀 지원 | 풀 지원 | Docker (Stream 8은 실 호스트 will-server도) |
| **RHEL** 9 | ✅ 검증 | 풀 지원 | 풀 지원 | UBI minimal 컨테이너 검증, 7/8은 코드 호환 |
| **Rocky Linux** 8 / 9 | ✅ 검증 | 풀 지원 | 풀 지원 | Docker (Rocky 8/9 둘 다) |
| **AlmaLinux** 8 / 9 | ✅ 검증 | 풀 지원 | 풀 지원 | Docker (Alma 8/9 둘 다) |
| **Oracle Linux** 7 / 8 / 9 | ✅ 검증 | 풀 지원 | 풀 지원 | Docker (Oracle 7/8/9 셋 다) |
| **OpenSUSE Leap** 15 / SUSE Linux Enterprise | ✅ 검증 (Leap 15) | 풀 지원 | 풀 지원 | Docker (Leap 15.5), SLES는 미검증 |
| **macOS** 13+ (Intel / Apple Silicon) | ✅ 검증 | 풀 지원 | 디스크 개수/크기 OK, 모델명은 fstype 대체 | gopsutil disk.Partitions fallback |
| **Windows Server** 2012 R2 / 2016 / 2019 / 2022, **Win 10 / 11** | ⚠️ 동작 가능 | 풀 지원 예상 | 디스크 개수/크기 예상, 모델명 fstype 대체 | `install-agent-windows.ps1` 제공, 미검증 |
| **FreeBSD** 12 / 13 / 14 | ⚠️ 동작 가능 | 풀 지원 예상 | 풀 지원 예상 | gopsutil 공식 지원, 미검증 |
| **OpenBSD** 6 / 7 | ⚠️ 부분 동작 | CPU/RAM 예상 | 일부만 | gopsutil 제한적 지원, 미검증 |
| **Solaris** 11 / illumos | ⚠️ 부분 동작 | 일부 | 일부 | gopsutil plan9stats 일부 지원, 미검증 |
| **AIX** 7.1+ | ⚠️ 부분 동작 | 일부 | 일부 | gopsutil perfstat 일부 지원, cgo 필요, 미검증 |

> **검증 정의:**
> ✅ **검증** — 실제 운영 호스트 또는 사내 인프라에서 동작 확인.
> ⚠️ **동작 가능** — gopsutil 코드 경로 존재. 빌드/실행 예상되나 실서버 미검증. 고객 요청 시 우선 검증.
> ⚠️ **부분 동작** — 일부 메트릭만 수집 가능. 디스크 모델/회전속도 등 일부 정보 누락 예상.

### 아키텍처

| 아키텍처 | 상태 |
|---------|-----|
| **x86_64 (amd64)** | ✅ 모든 검증 OS |
| **ARM64 (aarch64)** | ✅ macOS (Apple Silicon), Linux ARM64 빌드 가능 |
| **ARMv7** | ⚠️ 빌드 가능, 미검증 |

### 서버 / 대시보드

| 구성 | 지원 환경 |
|------|---------|
| **OWLmon 서버 + 웹** | Docker Compose가 동작하는 모든 OS (Linux 권장, x86_64 / ARM64) |
| **PostgreSQL** | 16 (Alpine 컨테이너) |
| **Prometheus** | v2.51+ |

## 빠른 시작

```bash
# 필수: Docker, Go, Node.js
make dev

# 대시보드: http://localhost:3001
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
