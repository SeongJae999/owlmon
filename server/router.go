package main

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/seongJae/owlmon/server/alert"
	"github.com/seongJae/owlmon/server/auth"
	"github.com/seongJae/owlmon/server/handler"
	snmppkg "github.com/seongJae/owlmon/server/snmp"
)

// InitRouter는 API 라우팅을 초기화합니다.
func InitRouter(appCtx *AppContext, checker *alert.Checker, jwtSecret, username, passwordHash, prometheusURL string) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	authHandler := handler.NewAuthHandler(username, passwordHash, jwtSecret)
	proxyHandler, _ := handler.NewProxyHandler(prometheusURL)
	alertHandler := handler.NewAlertHandler(appCtx.ConfigStore, checker)
	statusHandler := handler.NewStatusHandler(prometheusURL, appCtx.ConfigStore, checker, appCtx.DBPool)
	anomalyHandler := handler.NewAnomalyHandler(checker.Detector, checker.Predictor)
	agentHandler := handler.NewAgentHandler(appCtx.AgentStore)

	r.Post("/api/auth/login", authHandler.Login)
	r.Get("/api/health", statusHandler.HealthCheck)
	r.Post("/api/agent/register", agentHandler.Register)

	// 스펙 ingest는 에이전트가 호출하는 무인증 엔드포인트 (Agent Key 헤더는 추후 검증 도입 예정)
	if appCtx.SpecsStore != nil {
		specsHandler := handler.NewSpecsHandler(appCtx.SpecsStore)
		r.Post("/api/agent/specs", specsHandler.Ingest)

		// 조회는 인증 그룹에서
		r.Group(func(r chi.Router) {
			r.Use(auth.JWTMiddleware(jwtSecret))
			r.Get("/api/agent/specs", specsHandler.List)
			r.Get("/api/agent/specs/{host}", specsHandler.Get)
		})
	}

	r.Group(func(r chi.Router) {
		r.Use(auth.JWTMiddleware(jwtSecret))
		r.Handle("/api/v1/*", proxyHandler)

		// Alert
		r.Get("/api/alert/config", alertHandler.GetConfig)
		r.Post("/api/alert/config", alertHandler.SetConfig)
		r.Post("/api/alert/ack", alertHandler.AckAlert)
		r.Get("/api/alert/status", statusHandler.GetStatus)
		r.Get("/api/uptime", statusHandler.GetUptime)

		if appCtx.HistoryStore != nil {
			historyHandler := handler.NewHistoryHandler(appCtx.HistoryStore)
			r.Get("/api/alert/history", historyHandler.List)
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

		// SNMP
		if appCtx.SNMPDeviceStore != nil {
			snmpHandler := handler.NewSNMPHandler(appCtx.SNMPDeviceStore, snmppkg.NewPoller())
			r.Get("/api/snmp/devices", snmpHandler.ListDevices)
			r.Post("/api/snmp/devices", snmpHandler.AddDevice)
			r.Delete("/api/snmp/devices/{id}", snmpHandler.DeleteDevice)
			r.Get("/api/snmp/status", snmpHandler.GetStatus)
		}

		// SSL
		if appCtx.SSLDomainStore != nil {
			sslHandler := handler.NewSSLHandler(appCtx.SSLDomainStore, checker.SSLChecker)
			r.Get("/api/ssl/domains", sslHandler.ListDomains)
			r.Post("/api/ssl/domains", sslHandler.AddDomain)
			r.Delete("/api/ssl/domains/{id}", sslHandler.DeleteDomain)
			r.Get("/api/ssl/status", sslHandler.GetStatus)
			r.Post("/api/ssl/check", sslHandler.TriggerCheck)
		}

		// Log
		if appCtx.LogStore != nil {
			logHandler := handler.NewLogHandler(appCtx.LogStore, appCtx.LogAnnotationStore, appCtx.AgentStore, getEnv("OWLMON_AGENT_KEY", ""), appCtx.RulesEngine, appCtx.LogRuleMatchStore, checker.SendAlert)
			r.Get("/api/logs", logHandler.Search)
			r.Get("/api/logs/sources", logHandler.Sources)
			r.Get("/api/logs/annotations", logHandler.ListAnnotations)
			r.Delete("/api/logs/annotations/{id}", logHandler.DeleteAnnotation)
			r.Get("/api/logs/{id}", logHandler.GetByID)
			r.Get("/api/logs/{id}/annotations", logHandler.ListAnnotationsByLog)
			r.Post("/api/logs/{id}/annotate", logHandler.Annotate)
		}

		// Synthetic
		if appCtx.SyntheticStore != nil {
			synthHandler := handler.NewSyntheticHandler(appCtx.SyntheticStore, nil) // Checker is global
			r.Get("/api/synthetic/monitors", synthHandler.ListMonitors)
			r.Post("/api/synthetic/monitors", synthHandler.AddMonitor)
			r.Delete("/api/synthetic/monitors/{id}", synthHandler.DeleteMonitor)
			r.Get("/api/synthetic/status", synthHandler.GetStatus)
		}

		// Agents (Admin)
		r.Get("/api/agents", agentHandler.List)
		r.Post("/api/agents/{id}/status", agentHandler.UpdateStatus)
		r.Delete("/api/agents/{id}", agentHandler.Delete)

		// Log Rules (Admin) — CRUD + 통계
		if appCtx.DBPool != nil && appCtx.RulesEngine != nil {
			rulesHandler := handler.NewRulesHandler(appCtx.DBPool, appCtx.RulesEngine)
			r.Get("/api/log-rules", rulesHandler.List)
			r.Post("/api/log-rules", rulesHandler.Create)
			r.Put("/api/log-rules/{id}", rulesHandler.Update)
			r.Post("/api/log-rules/{id}/toggle", rulesHandler.Toggle)
			r.Delete("/api/log-rules/{id}", rulesHandler.Delete)
			r.Get("/api/log-rules/stats", rulesHandler.MatchStats)
		}
	})

	// Log Ingest (Agent Key Auth)
	if appCtx.LogStore != nil {
		logIngestHandler := handler.NewLogHandler(appCtx.LogStore, appCtx.LogAnnotationStore, appCtx.AgentStore, getEnv("OWLMON_AGENT_KEY", ""), appCtx.RulesEngine, appCtx.LogRuleMatchStore, checker.SendAlert)
		r.Post("/api/logs/ingest", logIngestHandler.Ingest)
	}

	return r
}
