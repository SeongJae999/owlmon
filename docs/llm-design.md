# OWLmon — LLM 기반 로그 분석 설계안

> **상태:** 설계 (구현 전) · **작성:** 2026-05
>
> 학교/공공기관 망분리 환경에서 "AI 기반 로그 분석" 차별화를 위한
> 단계별 로드맵. 외부 API 의존 없는 온디바이스 LLM이 핵심.

---

## 1. 배경

### 1.1 현재 OWLmon이 로그를 다루는 방식

- 에이전트가 `/var/log/*` tail → 서버 PostgreSQL 저장
- 키워드 필터 (`error / fatal / panic / warning`) — 단순 문자열 매칭
- 알림: 키워드 매칭 시 이메일 발송

### 1.2 한계

| 한계 | 예시 |
|-----|-----|
| **노이즈** | "warning"이라는 단어가 무해한 로그에도 등장 (예: `warning: deprecated config`) |
| **맥락 부족** | 같은 에러도 빈도/시점에 따라 의미 다름 |
| **원인 추적 X** | 알림은 떴지만 "왜?" "어떻게 대응?" 답이 없음 |
| **자연어 질의 X** | "최근 3일 동안 메모리 관련 이슈만 보고싶다" 같은 요청 불가 |

### 1.3 경쟁사 대응

| 솔루션 | AI 로그 분석 | OWLmon 차별점 |
|-------|------------|--------------|
| Datadog Watchdog | ✅ (외부 API) | ❌ 망분리 불가 |
| New Relic AI | ✅ (외부 API) | ❌ 망분리 불가 |
| Splunk SOAR | ✅ (자체 모델) | 가격 ↑, 한국 지원 ↓ |
| Zabbix | ❌ 없음 | OWLmon이 추가 시 우위 |
| **OWLmon (목표)** | ✅ **온디바이스** | ⭐ **외부 인터넷 없이 AI 분석** |

---

## 2. 타겟 환경 제약

학교·공공기관(주요 고객) 특성:

1. **망분리** — 외부 OpenAI/Claude API 호출 불가
2. **GPU 부재** — 일반 서버. GPU 별도 옵션
3. **자원 한정** — 8~16GB RAM, 일부 32GB
4. **IT 전담 없음** — 모델 학습/튜닝 어려움
5. **데이터 민감** — 학생 정보·내부 시스템 로그 외부 유출 금지

→ **결론: 가벼운 온디바이스 모델 + 선택형 LLM 옵션**이 정답.

---

## 3. 5단계 로드맵 (Phase 0 선행 작업 포함)

```
Phase 0 ──► Phase 1 ──► Phase 2 ──► Phase 3 ──► Phase 4
로그 수집   룰 강화    분류 모델   온디바이스   RAG 검색
정상화      즉시       단기        LLM (중기)   장기
필수 선행                                       + Vector DB
```

> ⚠️ **Phase 0 선행 작업이 없으면 Phase 1~4 모두 의미 없음.** 운영 DB의 logs 테이블이
> 0건이면 룰 매칭 대상도 분류 모델 학습 데이터도 없다. 실측 결과 현재 운영 환경의
> 에이전트가 로그 파일을 못 읽고 있어 우선 해결 필수.

| Phase | 핵심 기능 | 사용자 가치 |
|------|---------|-----------|
| 1 | 정규식 룰셋 + 빈도 임계치 | 노이즈 70% 감소 |
| 2 | 로그 → (정상/이상/심각) 자동 분류 | 우선순위 자동 |
| 3 | "이 로그 분석해줘" → LLM 답변 | 원인·대응 안내 |
| 4 | "지난주 메모리 이슈" 자연어 검색 | 운영자 생산성 ↑ |

---

## 4. Phase별 상세

### Phase 0 — 로그 수집 정상화 (선행 작업)

**현재 문제:**
- `willdev` / `willkomo` (Debian 12 Proxmox): `/var/log/syslog` 파일 없음
  - Debian 12부터 기본 rsyslog 미설치, journald만 사용
- `will-server` (CentOS Stream 8): `/var/log/messages` permission denied
  - `owlmon-agent` 사용자가 root 소유 파일 못 읽음

**해결 (호스트별):**

| 호스트 | 조치 |
|-------|-----|
| **willdev / willkomo** | (A) `apt install rsyslog` — 표준 syslog 파일 생성, **또는** (B) `config.yaml`의 `logs.journald.enabled: true`로 직접 journald 수집 |
| **will-server** | `usermod -aG systemd-journal owlmon-agent` + `logs.journald.enabled: true`, **또는** `/var/log/messages` 권한 부여 |

**검증:**
```sql
SELECT host, COUNT(*) FROM logs GROUP BY host;
```
→ 모든 운영 호스트 행이 있어야 함.

**자원:** 호스트별 systemctl 1~2회. 0분 영향.

---

### Phase 1 — 룰 기반 강화

**기술:** 정규식 + 빈도 임계치 (코드만)

```yaml
# 예시 룰셋
rules:
  - name: "OOM Killer 발동"
    pattern: '/Out of memory.*Kill process/'
    severity: critical
    cooldown: 5m
  - name: "디스크 가득"
    pattern: '/No space left on device/'
    severity: critical
  - name: "SSH 무차별 대입"
    pattern: '/Failed password.*from/'
    threshold: 10/1m   # 1분에 10회 이상
    severity: warning
```

**구현 위치 (코드 진입점):**

```
[에이전트] → POST /api/logs/ingest
              ↓
[server/handler/log.go: LogHandler.Ingest]   ← 룰 평가 hook 추가
              ↓
[server/db/log_store.go: LogStore.Ingest]    ← INSERT 시 매칭 결과 같이 저장
              ↓
PostgreSQL  (logs + log_rule_matches 테이블)
```

- `server/db/log_rules` 테이블 신설 (id, pattern, severity, cooldown, enabled)
- `server/db/log_rule_matches` 테이블 (log_id, rule_id, matched_at) — 알림 트리거
- `server/handler/log.go`의 Ingest에서 룰 매칭 (인메모리 컴파일된 정규식 캐시)
- 웹 UI에서 룰 CRUD (`/rules` 페이지)

**자원:** CPU 거의 X, 메모리 추가 0.

**ROI:** 매우 높음. 가장 먼저.

---

### Phase 2 — 분류 모델 (이상/정상/심각)

**기술:** DistilBERT 또는 TinyBERT (Hugging Face)

| 모델 | 크기 | 메모리 | CPU 추론 |
|-----|-----|------|--------|
| DistilBERT base | 250MB | 500MB | ~50ms |
| TinyBERT | 60MB | 200MB | ~10ms |
| Korean-BERT (학습 후) | 400MB | 1GB | ~80ms |

**구현:**
- Python 사이드카 컨테이너 (`owlmon-classifier`) FastAPI
- Go 서버 → HTTP POST `/classify` → 분류 결과
- 학습 데이터: 운영 1~3개월 로그 수동 라벨링 (또는 룰 매칭으로 자동 약 라벨)

**자원:** 추가 컨테이너 1개, 1GB RAM.

**한계:** 학습 데이터 부족 시 정확도 낮음. 룰셋과 병행 필요.

---

### Phase 3 — 온디바이스 LLM (자연어 분석)

**기술:** Ollama + 한국어 7~8B 모델

| 모델 | 양자화 | VRAM/RAM | 추론 속도 | 한국어 |
|-----|------|--------|--------|------|
| Llama 3.1 8B (Q4) | INT4 | 6GB | GPU 200ms / CPU 8초 | 보통 |
| Qwen 2.5 7B (Q4) | INT4 | 5GB | GPU 150ms / CPU 6초 | ⭐ 우수 |
| Gemma 2 9B (Q4) | INT4 | 6GB | GPU 250ms / CPU 10초 | 보통 |
| **EXAONE 3.5 7.8B** | INT4 | 6GB | 비슷 | ⭐⭐ LG 한국어 특화 |

**구현:**
```
[로그 알림 생성] → [LLM 프롬프트 빌드] → [Ollama POST /api/generate]
                                          ↓
[알림 + AI 분석 결과 같이 표시]   ←──────  "이 로그는 ... 이며,
                                          대응: 1) ... 2) ..."
```

**프롬프트 예시:**
```
당신은 시스템 관리자입니다. 다음 로그를 분석하고 한국어로 답변하세요.

[로그]
2026-05-11 03:24:11 sshd[12345]: Failed password for invalid user admin from 1.2.3.4
... (5회 반복)

[답변 형식]
- 원인:
- 심각도 (low/med/high):
- 권장 대응:
```

**자원 옵션 (실측 기준):**

현재 우리 인프라:
| 호스트 | 코어 | RAM | GPU | LLM 가능? |
|-------|------|-----|-----|---------|
| willdev | 20 | 125 GB | ❌ 없음 | ✅ CPU 추론 (5~10초) |
| will-server | 32 | 62 GB | ❌ 없음 | ✅ CPU 추론 (5~10초) |
| 학교/공공 일반 | 4~8 | 8~16 GB | ❌ 거의 없음 | ⚠️ 느림 (10~30초) |

→ **학교/공공기관 GPU 보급률 매우 낮음**. CPU 우선 전략 필수.

- **A. CPU 추론 (기본 권장)** — 답변 5~10초. **비동기 처리**로 UX 보완: 알림은 즉시, LLM 결과는 1분 내 후속 업데이트
- **B. GPU 서버 옵션** — Tesla T4 / RTX 4060 (8GB VRAM). Premium 라이선스 고객에게만
- **C. 본청 공유 GPU 서버** — 교육청·시청 1대 두고 산하 학교/기관이 HTTP로 호출 (단 망 정책 검토 필요)

**OWLmon 통합:**
- `docker-compose.yml`에 `ollama` 서비스 (profile 분리 — `--profile ai` 옵션)
- 환경변수 `OWLMON_LLM_ENABLED=true` 토글
- 알림 발송 시 백그라운드로 LLM 호출 → 결과 도착 시 알림 업데이트

---

### Phase 4 — RAG (Retrieval-Augmented Generation)

**기술:** 로그 임베딩 + 벡터 DB + LLM

```
[로그 ingest] → [임베딩 모델] → [pgvector 또는 Qdrant]
                                      ↑
[사용자 질문] → [임베딩] → [유사 로그 검색] → [LLM에 context] → [답변]
```

**예시:**
```
사용자: "지난주 willdev에서 메모리 관련 이슈 있었어?"
시스템: (관련 로그 5개 검색) → LLM 요약
LLM:    "지난주 willdev에서 OOM Killer 3회 발생 (5/13, 5/15, 5/16).
        모두 java 프로세스 (PID 12345)가 메모리 한계 도달 후 종료.
        디스크: ./data/logs 폴더가 60% 차있어 GC 압박 가능성."
```

**자원:** Phase 3 + 임베딩 모델 (200MB) + pgvector 또는 별도 Qdrant.

**복잡도:** 가장 높음. Phase 3 안정화 후 진행.

---

## 5. 자원 요구 종합

| 구성 | Phase 1 | Phase 2 | Phase 3 (CPU) | Phase 3 (GPU) | Phase 4 |
|-----|--------|--------|------------|------------|--------|
| 추가 컨테이너 | 0 | 1 (분류) | 1 (Ollama) | 1 (Ollama) | + 1 (Qdrant) |
| RAM 추가 | 0 | 1 GB | 8 GB | 1 GB | 4 GB |
| VRAM | - | - | - | 6 GB | 6 GB |
| 디스크 | 0 | 1 GB | 8 GB (모델) | 8 GB | 12 GB |
| 추론 속도 | 즉시 | 50ms | 5~10s | 200ms | 200ms+검색 |

---

## 6. 보안 / 데이터 처리

- ✅ 모든 모델 **온디바이스 추론** — 데이터 외부 송신 0
- ✅ 학습 데이터(로그)도 OWLmon 서버 내부 유지
- ⚠️ 모델 다운로드는 Hugging Face 또는 Ollama 레지스트리 — 망분리 환경에선 **사전 빌드 이미지** 제공 필요
- ⚠️ 로그에 PII(주민번호·이메일 등) 포함 시 분류 모델 학습에 직접 사용 X → 마스킹 전처리 필수

---

## 7. 영업 포지셔닝

```
✅ "외부 인터넷 없이도 AI가 로그를 분석"
✅ "외산 도구가 못 해주는 한국어 자연어 질의"
✅ "ChatGPT 같은 분석을 사내에서"
```

가격 옵션:
- 기본: Phase 1 포함 (룰 강화)
- AI 옵션: Phase 2+ 별도 라이선스/모듈
- AI Premium: Phase 3+4 + 모델 업데이트 구독

---

## 8. 의사결정 필요 사항

| 항목 | 옵션 | 권장 |
|-----|------|-----|
| **첫 진입점** | Phase 1만 / Phase 1+2 / Phase 1+3 | Phase 1 → 검증 → Phase 3 |
| **분류 모델 학습 시점** | 즉시 / 운영 데이터 3개월 쌓은 뒤 | **데이터 쌓은 뒤** (정확도 ↑) |
| **GPU vs CPU** | 필수 / 선택형 | **선택형** (Premium 라이선스에 한해) |
| **한국어 LLM 모델** | Llama 3 / Qwen / **EXAONE** | EXAONE — 한국어 + 국산 |
| **외부 LLM 사용 옵션** | 절대 X / 옵트인 허용 | **절대 X** (망분리 시장 우선) |

---

## 9. 다음 단계 (액션 아이템)

### 즉시 (이번 주)
- [ ] **Phase 0 실행** — 운영 호스트 3대 로그 수집 정상화 (rsyslog 설치 또는 journald 활성화 + 권한)
- [ ] 이 문서 사용자/팀 검토 → 방향 확정

### 단기 (1~2주)
- [ ] **Phase 1 PoC** — 룰셋 테이블 + 정규식 매칭 + UI
- [ ] 운영 데이터 1주일 누적 → 룰 후보 추출 (실측 로그 패턴 분석)

### 중기 (1~3개월)
- [ ] EXAONE 3.5 7.8B 한국어 로그 분석 능력 사전 평가 (사용자 맥북 또는 willdev CPU)
- [ ] Phase 2 분류 모델 학습 데이터 라벨링 (룰셋 매칭 결과 활용)

### 장기 (3개월+)
- [ ] GPU 서버 옵션 PoC — Tesla T4 / RTX 4060급 (Premium 라이선스 검토)
- [ ] Phase 4 RAG — pgvector 도입

---

## 부록 A — 참고 자료

- Ollama: https://ollama.com
- EXAONE 3.5: https://huggingface.co/LGAI-EXAONE/EXAONE-3.5-7.8B-Instruct
- pgvector (PostgreSQL): https://github.com/pgvector/pgvector
- DistilBERT: https://huggingface.co/distilbert/distilbert-base-uncased
- Zabbix vs OWLmon AI 비교: (작성 예정)
