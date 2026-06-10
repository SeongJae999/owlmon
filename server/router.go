package main

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/seongJae/owlmon/server/alert"
	"github.com/seongJae/owlmon/server/audit"
	"github.com/seongJae/owlmon/server/auth"
	"github.com/seongJae/owlmon/server/handler"
	"github.com/seongJae/owlmon/server/llm"
	"github.com/seongJae/owlmon/server/report"
	snmppkg "github.com/seongJae/owlmon/server/snmp"
)

// InitRouter는 API 라우팅을 초기화합니다.
func InitRouter(appCtx *AppContext, checker *alert.Checker, jwtSecret, username, passwordHash, prometheusURL string) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	authHandler := handler.NewAuthHandler(username, passwordHash, jwtSecret, appCtx.AuditStore)
	proxyHandler, _ := handler.NewProxyHandler(prometheusURL)
	alertHandler := handler.NewAlertHandler(appCtx.ConfigStore, checker, appCtx.EmailConfig)
	statusHandler := handler.NewStatusHandler(prometheusURL, appCtx.ConfigStore, checker, appCtx.DBPool)
	anomalyHandler := handler.NewAnomalyHandler(checker.Detector, checker.Predictor)
	agentHandler := handler.NewAgentHandler(appCtx.AgentStore)
	reportHandler := handler.NewReportHandler(report.NewReporter(prometheusURL, appCtx.EmailConfig, appCtx.ConfigStore))
	llmHandler := handler.NewLLMHandler(llm.NewProvider(), appCtx.HistoryStore)

	r.Post("/api/auth/login", authHandler.Login)
	r.Get("/api/health", statusHandler.HealthCheck)

	// ── 에이전트 전용 엔드포인트 — X-Agent-Key 인증 ──
	// (종전: specs ingest 무인증, self-update는 JWT 그룹에 있어 에이전트가 항상 401 받던 버그 수정)
	agentKeyAuth := handler.AgentKeyAuth(getEnv("OWLMON_AGENT_KEY", ""), appCtx.AgentStore)
	agentUpdateHandler := handler.NewAgentUpdateHandler("/app/data/agents")
	var specsHandler *handler.SpecsHandler
	if appCtx.SpecsStore != nil {
		specsHandler = handler.NewSpecsHandler(appCtx.SpecsStore)
	}
	r.Group(func(r chi.Router) {
		r.Use(agentKeyAuth)
		if specsHandler != nil {
			r.Post("/api/agent/specs", specsHandler.Ingest)
		}
		// Agent self-update — 에이전트가 X-Agent-Key로 바이너리 확인/다운로드
		r.Get("/api/agent/latest", agentUpdateHandler.GetLatest)
		r.Get("/api/agent/binary", agentUpdateHandler.GetBinary)
	})

	// 스펙 조회는 관리자(JWT) 그룹
	if specsHandler != nil {
		r.Group(func(r chi.Router) {
			r.Use(auth.JWTMiddleware(jwtSecret))
			r.Get("/api/agent/specs", specsHandler.List)
			r.Get("/api/agent/specs/{host}", specsHandler.Get)
		})
	}

	r.Group(func(r chi.Router) {
		r.Use(auth.JWTMiddleware(jwtSecret))
		r.Use(audit.Middleware(appCtx.AuditStore))
		r.Handle("/api/v1/*", proxyHandler)

		// Alert
		r.Get("/api/alert/config", alertHandler.GetConfig)
		r.Post("/api/alert/config", alertHandler.SetConfig)
		r.Get("/api/alert/email-status", alertHandler.GetEmailStatus)
		r.Post("/api/alert/ack", alertHandler.AckAlert)
		r.Get("/api/alert/status", statusHandler.GetStatus)
		r.Get("/api/uptime", statusHandler.GetUptime)

		if appCtx.HistoryStore != nil {
			historyHandler := handler.NewHistoryHandler(appCtx.HistoryStore)
			r.Get("/api/alert/history", historyHandler.List)
			r.Get("/api/alert/history/export", historyHandler.Export)
		}

		// Maintenance
		r.Get("/api/maintenance", alertHandler.GetMaintenance)
		r.Post("/api/maintenance", alertHandler.SetMaintenance)

		// Anomaly
		r.Get("/api/anomaly", anomalyHandler.GetAnomalies)
		r.Get("/api/anomaly/disk", anomalyHandler.GetDiskPredictions)

		// Asset
		if appCtx.AssetStore != nil {
			assetHandler := handler.NewAssetHandler(appCtx.AssetStore)
			r.Get("/api/assets", assetHandler.List)
			r.Put("/api/assets", assetHandler.Upsert)
			r.Delete("/api/assets/{id}", assetHandler.Delete)
		}

		// SNMP — appCtx.SNMPPoller는 init.go의 백그라운드 폴러와 같은 인스턴스
		if appCtx.SNMPDeviceStore != nil {
			if appCtx.SNMPPoller == nil {
				appCtx.SNMPPoller = snmppkg.NewPoller()
			}
			snmpHandler := handler.NewSNMPHandler(appCtx.SNMPDeviceStore, appCtx.SNMPPoller, prometheusURL)
			r.Get("/api/snmp/devices", snmpHandler.ListDevices)
			r.Post("/api/snmp/devices", snmpHandler.AddDevice)
			r.Put("/api/snmp/devices/{id}", snmpHandler.UpdateDevice)
			r.Delete("/api/snmp/devices/{id}", snmpHandler.DeleteDevice)
			r.Get("/api/snmp/status", snmpHandler.GetStatus)
		}

		// SSL
		if appCtx.SSLDomainStore != nil {
			sslHandler := handler.NewSSLHandler(appCtx.SSLDomainStore, checker.SSLChecker)
			r.Get("/api/ssl/domains", sslHandler.ListDomains)
			r.Post("/api/ssl/domains", sslHandler.AddDomain)
			r.Patch("/api/ssl/domains/{id}", sslHandler.UpdateDomain)
			r.Delete("/api/ssl/domains/{id}", sslHandler.DeleteDomain)
			r.Get("/api/ssl/status", sslHandler.GetStatus)
			r.Post("/api/ssl/check", sslHandler.TriggerCheck)
		}

		// Log
		if appCtx.LogStore != nil {
			logHandler := handler.NewLogHandler(appCtx.LogStore, appCtx.LogAnnotationStore, appCtx.AgentStore, getEnv("OWLMON_AGENT_KEY", ""), appCtx.RulesEngine, appCtx.LogRuleMatchStore, checker.SendAlert)
			r.Get("/api/logs", logHandler.Search)
			r.Get("/api/logs/histogram", logHandler.Histogram)
			r.Get("/api/logs/export", logHandler.Export)
			r.Get("/api/logs/sources", logHandler.Sources)
			r.Get("/api/logs/annotations", logHandler.ListAnnotations)
			r.Delete("/api/logs/annotations/{id}", logHandler.DeleteAnnotation)
			r.Get("/api/logs/{id}", logHandler.GetByID)
			r.Get("/api/logs/{id}/annotations", logHandler.ListAnnotationsByLog)
			r.Post("/api/logs/{id}/annotate", logHandler.Annotate)
		}

		// DPM
		if appCtx.DPMStore != nil && appCtx.DPMPoller != nil {
			dpmHandler := handler.NewDPMHandler(appCtx.DPMStore, appCtx.DPMPoller)
			r.Get("/api/dpm/instances", dpmHandler.ListInstances)
			r.Post("/api/dpm/instances", dpmHandler.AddInstance)
			r.Delete("/api/dpm/instances/{id}", dpmHandler.DeleteInstance)
			r.Get("/api/dpm/status", dpmHandler.GetStatus)
			r.Get("/api/dpm/instances/{id}/queries", dpmHandler.GetQueries)
			r.Get("/api/dpm/instances/{id}/metrics", dpmHandler.GetMetricsHistory)
			r.Post("/api/dpm/instances/{id}/check", dpmHandler.TriggerCheck)
		}

		// Synthetic
		if appCtx.SyntheticStore != nil && appCtx.SyntheticChecker != nil {
			synthHandler := handler.NewSyntheticHandler(appCtx.SyntheticStore, appCtx.SyntheticChecker)
			r.Get("/api/synthetic/monitors", synthHandler.ListMonitors)
			r.Post("/api/synthetic/monitors", synthHandler.AddMonitor)
			r.Put("/api/synthetic/monitors/{id}", synthHandler.UpdateMonitor)
			r.Delete("/api/synthetic/monitors/{id}", synthHandler.DeleteMonitor)
			r.Get("/api/synthetic/monitors/{id}/history", synthHandler.GetHistory)
			r.Post("/api/synthetic/monitors/{id}/check", synthHandler.TriggerCheck)
			r.Get("/api/synthetic/status", synthHandler.GetStatus)
		}

		// Agents (Admin)
		// register는 키 발급 엔드포인트 — 무인증이면 누구나 에이전트 행을 무한 생성 가능해 관리자 전용으로 이동
		r.Post("/api/agent/register", agentHandler.Register)
		r.Get("/api/agents", agentHandler.List)
		r.Post("/api/agents/{id}/status", agentHandler.UpdateStatus)
		r.Delete("/api/agents/{id}", agentHandler.Delete)

		// 월간 보고서
		r.Get("/api/report/preview", reportHandler.Preview)
		r.Post("/api/report/send", reportHandler.Send)

		// LLM (로그 설명 + 알림 요약)
		r.Get("/api/llm/status", llmHandler.Status)
		r.Post("/api/llm/explain", llmHandler.ExplainLog)
		r.Post("/api/llm/summary", llmHandler.SummarizeAlerts)

		// 감사 로그 (ISMS-P 변경 추적)
		auditHandler := handler.NewAuditHandler(appCtx.AuditStore)
		r.Get("/api/audit", auditHandler.List)
		r.Get("/api/audit/export", auditHandler.Export)

		// Log Rules (Admin) — CRUD + 통계
		if appCtx.DBPool != nil && appCtx.RulesEngine != nil {
			rulesHandler := handler.NewRulesHandler(appCtx.DBPool, appCtx.RulesEngine)
			r.Get("/api/log-rules", rulesHandler.List)
			r.Post("/api/log-rules", rulesHandler.Create)
			r.Put("/api/log-rules/{id}", rulesHandler.Update)
			r.Post("/api/log-rules/{id}/toggle", rulesHandler.Toggle)
			r.Delete("/api/log-rules/{id}", rulesHandler.Delete)
			r.Get("/api/log-rules/stats", rulesHandler.MatchStats)
			r.Get("/api/log-rules/stats/detailed", rulesHandler.MatchStatsDetailed)
		}
	})

	// Log Ingest (Agent Key Auth)
	if appCtx.LogStore != nil {
		logIngestHandler := handler.NewLogHandler(appCtx.LogStore, appCtx.LogAnnotationStore, appCtx.AgentStore, getEnv("OWLMON_AGENT_KEY", ""), appCtx.RulesEngine, appCtx.LogRuleMatchStore, checker.SendAlert)
		r.Post("/api/logs/ingest", logIngestHandler.Ingest)
	}

	return r
}
