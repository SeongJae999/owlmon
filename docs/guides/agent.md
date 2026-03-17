# OWLmon 에이전트 개발 가이드

모니터링 대상 서버에 설치되어 시스템 메트릭을 수집하고 OTel Collector로 전송하는 Go 에이전트입니다.

---

## 왜 Go인가?

Python으로 MVP를 만들 수도 있지만, 에이전트는 **항상 떠 있는 프로세스**라 언어 선택이 중요합니다.

| 항목 | Python | Go |
|------|--------|----|
| 메모리 | ~50MB | ~10MB |
| 바이너리 | 런타임 필요 | 단일 실행파일 |
| 배포 | pip + 의존성 관리 | 파일 하나 복사 |
| 성능 | 상대적으로 느림 | 네이티브 수준 |

**목표**: 메모리 10MB 이하, CPU 1% 이하 — Go가 유리합니다.

---

## 코드 구조

```
agent/
├── main.go              # 진입점: MeterProvider 초기화, 수집기 등록, 종료 처리
├── collector/
│   ├── cpu.go          # CPU 사용률 수집 (전체 평균 %)
│   ├── memory.go       # 메모리 사용률 + 사용량(bytes) 수집
│   └── disk.go         # 마운트포인트별 디스크 사용률 수집
└── exporter/
    └── otlp.go         # OTel Collector로 gRPC 전송
```

### 데이터 흐름

```
[서버 OS]
    ↓ gopsutil (30초마다)
[collector/cpu, memory, disk]
    ↓ OTel SDK (메트릭 누적)
[MeterProvider → PeriodicReader]
    ↓ gRPC (OTLP)
[OTel Collector :4317]
    ↓
[Prometheus / 대시보드]
```

---

## 수집 메트릭

| 메트릭 이름 | 단위 | 설명 |
|-------------|------|------|
| `system.cpu.usage` | % | 전체 CPU 평균 사용률 |
| `system.memory.usage_percent` | % | 메모리 사용률 |
| `system.memory.used_bytes` | bytes | 메모리 사용량 |
| `system.disk.usage_percent` | % | 마운트포인트별 디스크 사용률 |

디스크 메트릭에는 `mountpoint`, `device` 레이블이 붙어 파티션별로 구분됩니다.

---

## 환경변수

| 변수 | 기본값 | 설명 |
|------|--------|------|
| `OWLMON_OTLP_ENDPOINT` | `localhost:4317` | OTel Collector gRPC 주소 |

---

## 빌드 및 실행

### 의존성 패키지

```bash
# 시스템 메트릭 수집
go get github.com/shirou/gopsutil/v3

# OTel SDK + OTLP gRPC exporter
go get go.opentelemetry.io/otel/sdk/metric
go get go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc
```

### 빌드

```bash
cd agent/
go build -o owlmon-agent .
```

### 실행

```bash
# 기본 실행 (OTel Collector가 localhost:4317에 있어야 함)
./owlmon-agent

# OTel Collector 주소 지정
OWLMON_OTLP_ENDPOINT=192.168.1.10:4317 ./owlmon-agent
```

### 로컬 테스트 (OTel Collector 없이)

`docker-compose.yml`에 OTel Collector가 포함되어 있습니다.

```bash
# 프로젝트 루트에서
docker-compose up -d otel-collector
./owlmon-agent
```

---

## 수집 주기

- **시스템 메트릭** (CPU, 메모리, 디스크): 30초
- 주기 변경은 `main.go`의 `metric.WithInterval` 값을 수정합니다.

---

## 향후 추가 예정

- [ ] 네트워크 I/O 수집 (`net.IOCounters`)
- [ ] 서비스 체크 (TCP 포트, HTTP 응답)
- [ ] 버퍼링 — OTel Collector 연결 끊김 시 로컬 큐 저장 후 재전송
- [ ] TLS 인증 (`otlpmetricgrpc.WithTLSClientConfig`)
- [ ] 설정 파일 (`config.yaml`) 지원
