---
name: owlmon-plan
description: OWLmon 프로젝트 전략 수립, 로드맵 관리, 의사결정 지원
user_invocable: true
---

# OWLmon Planning Skill

OWLmon 프로젝트의 전략 수립, 로드맵 관리, 아키텍처 의사결정을 지원합니다.

## 수행 범위

### 전략 기획
- **시장 분석**: 국내 모니터링 시장 (공공/제조/금융 vs IT/스타트업)
- **경쟁사 분석**: 와탭, 제니퍼, Datadog Korea, 국내 SI 업체
- **차별화 전략**: Sentry 모델 적용 (한가지 특화 → 무료 티어 → 유료 전환)
- **가격 정책**: 납품형 vs SaaS 구독형 하이브리드
- **타겟 고객**: IT 전담 없는 중소기업 (공장, 병원, 학교 등)

### 로드맵 관리
- **Phase 1 (3개월 MVP)**: 에이전트 + 대시보드 + 카카오톡 알림
- **Phase 2 (6개월)**: 모바일 + 보고서 + SNMP + 서버 추가 자동화
- **Phase 3 (1년 SaaS)**: AI 이상감지 + 멀티테넌트 + OTel + 로그

### 아키텍처 의사결정 (ADR)
결정이 필요한 주요 사항:
1. Frontend: React vs Vue
2. Agent 언어: Python MVP → Go 전환 시점
3. DB: SQLite vs PostgreSQL (MVP)
4. 인증: JWT vs API Key vs OAuth
5. 배포: Docker Compose → K8s 전환 시점
6. TSDB: Prometheus → VictoriaMetrics 전환 기준

### 학습 로드맵
1. Uptime Kuma 설치 & 코드 분석
2. Grafana + Prometheus 연동
3. OpenTelemetry Collector 구축
4. Zabbix JSON-RPC API 활용
5. Python 에이전트 제작
6. 카카오톡 알림 연동
7. IT 담당자 인터뷰 (불편사항 발굴)
8. 핵심 불편 1개 해결하는 도구 제작

## 의사결정 프레임워크
```
1. 문제 정의: 무엇을 결정해야 하는가?
2. 선택지 나열: 가능한 옵션들
3. 평가 기준: MVP 속도 / 확장성 / 팀 역량 / 국내 시장 적합도
4. 결정 및 근거 기록
5. docs/decisions/ 디렉토리에 ADR 문서화
```

## 수익 모델 분석
| 모델 | 초기 매출 | 장기 매출 | 리스크 |
|------|-----------|-----------|--------|
| 납품형 | 건당 500만~수천만 | 불안정 | 영업 의존 |
| SaaS 구독 | 월 5만/10대 | 고정 반복 | 이탈률 관리 |
| 하이브리드 | 구축비 + 월 구독 | 안정적 | 양쪽 역량 필요 |
