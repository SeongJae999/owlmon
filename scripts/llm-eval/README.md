# LLM 로그 인사이트 평가셋

프롬프트나 모델을 바꿀 때마다 실행해 좋아졌는지, 나빠진 케이스는 없는지 확인한다. 운영 워커와 같은 경로(마스킹 → 프롬프트 → 호스트 상태 블록 → JSON 모드 → Validate)로 분석한다.

```bash
cd server
# 전체 실행 (결과는 results/<시각>-<모델>.json 에 저장)
OWLMON_OLLAMA_URL=http://localhost:11434 go run ./cmd/llmeval -repeat 3

# 최종 판정은 holdout만 / 다른 모델
go run ./cmd/llmeval -split holdout -repeat 3 -model gemma4:26b-a4b

# 두 결과 비교 (개선·회귀 케이스 목록)
go run ./cmd/llmeval -compare ../scripts/llm-eval/results/baseline-gemma4-12b.json,../scripts/llm-eval/results/<새 결과>.json
```

## 파일

| 파일 | 내용 |
|---|---|
| `eval-set.jsonl` | 합성 — 정상 종료·실장애·보안·일시적 |
| `eval-set-prod.jsonl` | 운영 실로그 (2026-06, 2026-11 수집) |
| `eval-set-history.jsonl` | `[패턴 이력]` 주입 (장기 지속) |
| `eval-set-multiline.jsonl` | 원인 묶기로 여러 종류 줄이 한 그룹 |
| `eval-set-hostctx.jsonl` | 호스트 상태(CPU·메모리·디스크·이상탐지)와 연결 |

## 케이스 형식

| 필드 | 뜻 |
|---|---|
| `log` 또는 `lines` | 입력 로그 (한 줄 / 여러 줄) |
| `host`, `count`, `host_ctx`, `hist_days`·`hist_n` | 운영 워커가 붙이는 입력 그대로 재현 |
| `context` | 운영에 아직 없는 맥락(자산 정보 등) — 맥락 활용 능력 측정용 |
| `want_sev` | 정답 심각도. 갈리는 케이스는 `["medium","high"]`처럼 범위 |
| `want_human` | 대응 필요 여부. 생략하면 채점 안 함 |
| `want_category` | 분류 (선택) |
| `must_mention` | 요약·원인·조치에 들어가야 할 말. `"a\|b"` = 둘 중 하나 |
| `must_not` | 들어가면 안 되는 말 (위험한 조치 등) |
| `split` | `dev`(프롬프트 다듬을 때) / `holdout`(최종 판정용 — 프롬프트 예시와 비슷한 케이스 넣지 말 것) |
| `label` | `확정` / `잠정`(사람 확인 전 — 별도 집계) |
| `tags` | 유형별 통과율 집계용 |

통과 = 형식 OK + 심각도 + 대응필요 + 분류 + 반드시 언급 + 금지 표현 없음.
