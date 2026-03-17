---
name: alert-design
description: OWLmon 알림 시스템 설계 및 채널 연동 가이드
user_invocable: true
---

# Alert System Design Skill

OWLmon 알림(Alerting) 시스템 설계 및 알림 채널 연동을 지원합니다.

## 수행 범위

### 알림 파이프라인
- **임계값 평가**: 메트릭 기반 조건 판정
- **디바운싱**: 채터링 방지 (N분 이상 지속 시에만 발동)
- **중복 제거**: 동일 호스트+메트릭 알림 1건만
- **심각도 분류**: Critical / Warning / Info
- **에스컬레이션**: 미응답 시 상위 담당자 전달

### 알림 채널 연동 (국내 특화)
| 채널 | API | 용도 |
|------|-----|------|
| 카카오톡 알림톡 | 비즈메시지 API | 고객사 공식 알림 |
| 카카오워크 | Webhook | 사내 팀 알림 |
| Slack | Webhook / Bot API | IT 기업 내부 |
| Telegram | Bot API | 개인/소규모 팀 |
| SMS | NHN Cloud / Twilio | Critical 알림 |
| Email | SMTP | 공식 기록용 |
| 전화 | Twilio Voice API | 최상위 긴급 |

### 알림 4원칙
1. **중복 제거**: 같은 문제는 1건만
2. **디바운싱**: 일시적 스파이크 무시
3. **심각도 분류**: 시간대별 채널 라우팅
4. **액션 가이드**: 원인 추정 + 해결 방법 함께 제공

## 알림 메시지 템플릿
```
[OWLmon {심각도}] {호스트명}

문제: {메트릭명} {현재값} ({임계값} 초과)
지속: {지속 시간}
원인 추정: {AI 분석 결과}
조치 가이드: {권장 조치}
대시보드: {링크}
```

## 코드 구조
```
src/alert/
├── evaluator.py      # 임계값 평가 엔진
├── dedup.py          # 중복 제거
├── debounce.py       # 디바운싱
├── escalation.py     # 에스컬레이션 로직
├── router.py         # 심각도별 채널 라우팅
└── channels/
    ├── kakao_talk.py   # 카카오톡 알림톡
    ├── kakao_work.py   # 카카오워크 Webhook
    ├── slack.py        # Slack Webhook
    ├── telegram.py     # Telegram Bot
    ├── sms.py          # SMS (NHN Cloud)
    └── email.py        # SMTP
```

## 참고
- Google SRE Book Ch.6: "알림이 울릴 때마다 긴급 대응할 수 있어야 한다"
- Zabbix 관리자 불만: 알림 폭탄, 원인 미제시 → OWLmon이 해결할 핵심 차별점
