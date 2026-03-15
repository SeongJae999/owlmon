# OWLmon 기술스택 상세 정리

## 1. 모니터링 도구 생태계 정리

### Tier 1: 핵심 학습 대상
| 도구 | 카테고리 | 언어 | OTel | 왜 배워야 하나 |
|------|----------|------|------|----------------|
| **Prometheus** | 메트릭 | Go | 완벽 | 클라우드 메트릭 수집의 표준, PromQL 필수 |
| **Grafana** | 시각화 | Go/TS | 완벽 | 대시보드 시각화 표준, 커스텀 패널 개발 참고 |
| **Zabbix** | 인프라 | C/PHP | 제한적 | 국내 공공/제조/금융 납품 시 필수 |
| **Uptime Kuma** | 업타임 | Node.js | X | MVP 코드 구조 벤치마크 (Node.js+Vue.js+SQLite) |

### Tier 2: 확장 시 고려
| 도구 | 카테고리 | 언어 | 언제 필요한가 |
|------|----------|------|---------------|
| **SigNoz** | 올인원 | Go/TS | OTel 네이티브 올인원 필요 시 |
| **VictoriaMetrics** | TSDB | Go | Prometheus 스케일 한계 도달 시 |
| **Loki** | 로그 | Go | Grafana 스택 로그 분석 추가 시 |
| **Checkmk** | 인프라 | Python/C++ | Zabbix 대안 제안 필요 시 |

### Tier 3: 참고/벤치마크
| 도구 | 참고 포인트 |
|------|-------------|
| **OpenObserve** | Rust 기반 초경량, S3 스토리지 활용 아이디어 |
| **Gatus** | Go 기반 YAML 설정, 합성 모니터링 구현 참고 |
| **OneUptime** | 모니터링+상태페이지+인시던트 올인원 UX 참고 |

---

## 2. OWLmon 개발 기술스택

### Phase 1: MVP (3개월)
```
[수집]     Python + psutil → CPU/메모리/디스크 수집
[전송]     REST API (FastAPI)
[저장]     Prometheus TSDB (내장)
[시각화]   React + TypeScript + Recharts/Visx
[알림]     카카오워크 Webhook + Telegram Bot API
[배포]     Docker Compose
```

### Phase 2: 성숙 (6개월)
```
[수집]     Go 에이전트 (크로스 컴파일, 단일 바이너리)
[전송]     OTLP/gRPC (OpenTelemetry 호환)
[저장]     VictoriaMetrics (장기 보관)
[시각화]   + 모바일 반응형 PWA
[알림]     + 카카오톡 알림톡 API + SMS
[보고서]   월간 PDF 자동 생성
[네트워크] SNMP v2c/v3 폴링
```

### Phase 3: SaaS (1년)
```
[수집]     + 로그 수집 파이프라인
[저장]     ClickHouse (고카디널리티, 로그+트레이스)
[AI]       이상 패턴 감지 (시계열 anomaly detection)
[멀티테넌트] 고객사별 격리된 데이터/대시보드
[OTel]     완전한 OTel Collector 호환
```

---

## 3. 핵심 프로토콜

### SNMP (네트워크 장비)
- v2c: GetBulk 지원, 보안 약함 → 내부망 전용
- v3: 인증+암호화 → 권장
- 수집 대상: 라우터, 스위치, UPS, 프린터
- Python 라이브러리: `pysnmp`, `easysnmp`

### OTLP (OpenTelemetry Protocol)
- gRPC (port 4317) / HTTP (port 4318)
- Metrics + Logs + Traces 통합
- Protobuf 직렬화 (JSON 대비 10x 효율)

### PromQL (메트릭 쿼리)
- Prometheus/VictoriaMetrics/Grafana 공용
- 핵심 함수: `rate()`, `histogram_quantile()`, `increase()`
- 4 Golden Signals 쿼리 패턴 숙지 필수

### WebSocket (실시간 대시보드)
- 대시보드 실시간 갱신용
- Uptime Kuma 참고: Socket.IO 사용

---

## 4. 시계열 DB 선택 가이드

| 규모 | 추천 TSDB | 이유 |
|------|-----------|------|
| MVP (서버 ~10대) | Prometheus 내장 | 설치 간편, 충분한 성능 |
| 중규모 (서버 ~100대) | VictoriaMetrics | 높은 압축률, PromQL 호환 |
| 대규모 (서버 100대+) | ClickHouse | 고카디널리티, SQL 지원, 로그 통합 |

### 카디널리티 주의사항
- 라벨에 user_id, session_id 등 무한 증가 값 금지
- 서버별/서비스별/리전별 라벨만 사용
- Prometheus: 고카디널리티 취약 → ClickHouse 전환 고려

---

## 5. 에이전트 설계 스펙

```
메모리:     < 10MB (목표)
CPU:        < 1%
수집 주기:  시스템 메트릭 30초 / 서비스 체크 60초
버퍼링:     연결 끊김 시 로컬 버퍼 → 복구 후 전송
보안:       TLS 통신 + API 키 인증
업데이트:   중앙 관리 자동 업데이트
```

---

## 6. 핵심 논문/문서 (읽기 순서)

1. **Google SRE Book Ch.6** — 4 Golden Signals, 알림 원칙 (sre.google/sre-book)
2. **Google Dapper (2010)** — Span/Trace 개념, OTel의 기원
3. **Facebook Gorilla (VLDB 2015)** — Delta-of-Delta + XOR 압축, TSDB 설계
4. **OTel Future (IJCET 2025)** — 도입 실증 데이터, 샘플링 전략
5. **Observability 2.0** — AIOps, OTel Profiling, LLM 모니터링 트렌드

---

## 7. 보완이 필요한 영역

### 즉시 보완
- [ ] Git 초기화 및 .gitignore 설정
- [ ] package.json / go.mod 등 프로젝트 설정 파일
- [ ] 에이전트 프로토타입 (Python psutil 기반)
- [ ] API 스펙 정의 (OpenAPI/Swagger)

### 아키텍처 결정 필요
- [ ] Frontend 프레임워크 확정 (React vs Vue)
  - Uptime Kuma 벤치마크 → Vue.js
  - SigNoz/Grafana 참고 → React
  - 팀 역량/선호도에 따라 결정
- [ ] DB 선택: MVP에서 SQLite vs PostgreSQL
- [ ] 인증/인가 방식 (JWT, API Key, OAuth)
- [ ] 배포 전략 (Docker Compose → K8s)

### 리서치 보강
- [ ] 카카오톡 알림톡 API 실제 연동 테스트
- [ ] SNMP v3 실제 장비 테스트
- [ ] 경쟁사 분석 (국내: 와탭, 제니퍼, 데이터독 코리아)
- [ ] 가격 정책 벤치마크
