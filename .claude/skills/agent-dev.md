---
name: agent-dev
description: OWLmon 모니터링 에이전트 및 백엔드 개발 가이드
user_invocable: true
---

# Agent Development Skill

OWLmon 모니터링 에이전트 및 백엔드 시스템 개발을 지원합니다.

## 수행 범위

### 에이전트 개발
- **시스템 메트릭 수집**: CPU, 메모리, 디스크, 네트워크 (psutil/gopsutil)
- **서비스 체크**: TCP/HTTP 포트 확인, SSL 인증서 만료, DNS 응답
- **SNMP 폴링**: 네트워크 장비 메트릭 수집 (pysnmp)
- **버퍼링/재전송**: 연결 끊김 대응, 로컬 큐
- **경량화**: 메모리 10MB 이하, CPU 1% 이하 목표

### 백엔드 API
- **FastAPI 기반**: 메트릭 수신 엔드포인트
- **OTLP 호환**: gRPC/HTTP OTLP 수신
- **데이터 파이프라인**: 수집 → 저장 → 쿼리
- **Zabbix API 연동**: JSON-RPC 래퍼

### 데이터 저장
- **Prometheus 연동**: Remote Write/Read
- **TSDB 최적화**: 카디널리티 관리, 리텐션 정책
- **쿼리 API**: PromQL 호환 쿼리 엔드포인트

## 코드 규칙
- Python MVP: `src/agent/`, `src/api/`, `src/storage/`
- Go Production: `cmd/agent/`, `internal/collector/`, `internal/api/`
- 테스트 필수: 수집 로직, API 엔드포인트
- 에러 핸들링: 에이전트 크래시 방지 (graceful degradation)
- 로깅: structured logging (JSON)

## 에이전트 설계 원칙
```
1. 가벼워야 한다 (모니터링이 서버를 느리게 하면 안 됨)
2. 수집 주기: 시스템 30초 / 서비스 60초
3. 버퍼링: 서버 연결 끊어져도 로컬 저장 → 복구 시 전송
4. 보안: TLS + API 키 인증
5. 자동 업데이트: 중앙 관리
```

## 참고 벤치마크
- Uptime Kuma (Node.js + SQLite): 코드 구조 참고
- Zabbix Agent (C): 성능 벤치마크
- Prometheus node_exporter (Go): 메트릭 수집 패턴
- Grafana Agent (Go): OTel 통합 패턴
