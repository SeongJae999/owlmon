# LLM 로그 인사이트 평가셋

프롬프트를 바꿀 때마다 실행해 정확도 하락(회귀)을 확인한다. 운영 워커와 같은 경로(프롬프트 + JSON 모드 + Validate)로 분석한다.

```bash
cd server
OWLMON_OLLAMA_URL=http://localhost:11434 OWLMON_OLLAMA_MODEL=gemma4:12b \
  go run ./cmd/llmeval ../scripts/llm-eval/eval-set.jsonl
```

| 파일 | 내용 |
|---|---|
| `eval-set.jsonl` | 합성 케이스 20건 (정상 종료·실장애·1회성) |
| `eval-set-prod.jsonl` | 운영 실로그 10건 (라벨 `[잠정]` 포함) |
| `eval-set-history.jsonl` | `[패턴 이력]` 주입 케이스 6건 (`hist_days`, `hist_n`) |

`LLMEVAL_PLAIN=1`이면 JSON 모드를 끄고 실행한다.
