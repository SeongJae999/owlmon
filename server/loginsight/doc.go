// Package loginsight는 OWLmon 로그를 주기적으로 묶어 LLM으로 분석하고
// 결과(log_insights)를 PostgreSQL에 저장하는 백그라운드 워커 패키지.
//
// 기존 server/llm/ 의 on-demand 호출(/api/llm/explain)을 보완.
// 운영자가 직접 클릭하지 않아도 알아서 ERROR/WARN 로그 패턴을 묶고
// 한국어 인사이트로 PostgreSQL에 영구 저장한다.
//
// 흐름:
//
//	raw logs → severity 필터 → rate limit → 템플릿 클러스터링
//	         → 캐시(24h) → LLM(JSON 모드) → 검증 → log_insights 저장
//
// 사용 (다음 PR에서 추가될 예정):
//
//	w, _ := loginsight.New(cfg, db, provider)
//	w.Start(ctx) // 5분 주기 워커
//
// 본 패키지는 server/llm/ Provider 인터페이스를 재사용한다.
// LLM 백엔드(Ollama/OpenAI) 교체는 환경변수(OWLMON_LLM_PROVIDER)로 처리된다.
package loginsight
