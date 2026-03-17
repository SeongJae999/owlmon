---
name: monitoring-research
description: 모니터링 도구/기술/논문 리서치 및 비교 분석
user_invocable: true
---

# Monitoring Research Skill

OWLmon 프로젝트의 모니터링 기술 리서치를 수행합니다.

## 수행 범위
1. **도구 비교 분석**: 오픈소스 모니터링 도구 (Prometheus, Grafana, Zabbix, SigNoz, Uptime Kuma 등) 기능/성능/적합도 비교
2. **프로토콜 분석**: SNMP, OTLP, PromQL, WebSocket 등 모니터링 관련 프로토콜 조사
3. **논문/문서 분석**: Google Dapper, Facebook Gorilla, SRE Book 등 핵심 논문 요약 및 OWLmon 적용점 도출
4. **시장 조사**: 국내 모니터링 시장 (와탭, 제니퍼, Datadog Korea) 동향 및 가격 정책
5. **트렌드 분석**: AIOps, OTel Profiling, LLM Observability 등 최신 트렌드

## 수행 방법
- 웹 검색으로 최신 정보 확인
- `monitoring-enhanced.html` 참조하여 기존 리서치와 중복 방지
- 결과는 `docs/` 디렉토리에 마크다운으로 정리
- 비교표, 장단점, OWLmon 적용 전략 포함

## 출력 형식
```markdown
## [주제]
### 요약
### 상세 비교
### OWLmon 적용 전략
### 추가 리서치 필요사항
```

## 주의사항
- 한국어 기본, 기술 용어 영문 병기
- 국내 시장 특성 (공공기관 보안 요구, 카카오톡 알림, 온프레미스 선호) 반영
- `docs/TECH-STACK.md` 업데이트 필요 시 반영
