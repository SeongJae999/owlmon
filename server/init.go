package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seongJae/owlmon/server/alert"
	"github.com/seongJae/owlmon/server/db"
	"github.com/seongJae/owlmon/server/dpm"
	"github.com/seongJae/owlmon/server/report"
	"github.com/seongJae/owlmon/server/rules"
	snmppkg "github.com/seongJae/owlmon/server/snmp"
	"github.com/seongJae/owlmon/server/synthetic"
)

// AppContext는 서버 운영에 필요한 모든 의존성을 담고 있습니다.
type AppContext struct {
	DBPool             *pgxpool.Pool
	ConfigStore        alert.ConfigStorer
	HistorySaver       alert.HistorySaver
	HistoryStore       *db.AlertHistoryStore
	SNMPDeviceStore    *db.SNMPDeviceStore
	AssetStore         *db.AssetStore
	SSLDomainStore     *db.SSLDomainStore
	LogStore           *db.LogStore
	LogAnnotationStore *db.LogAnnotationStore
	SyntheticStore     *db.SyntheticStore
	DPMStore           *db.DPMStore
	AgentStore         *db.AgentStore
	SpecsStore         *db.SpecsStore
	RulesEngine        *rules.Engine
	LogRuleMatchStore  *db.LogRuleMatchStore
	EmailConfig        *alert.EmailConfig
}

// InitDB는 데이터베이스와 저장소를 초기화합니다.
func InitDB() *AppContext {
	var appCtx AppContext
	pgDSN := getEnv("POSTGRES_DSN", "")

	if pgDSN != "" {
		pool, err := db.Connect(context.Background(), pgDSN)
		if err != nil {
			log.Printf("PostgreSQL 연결 실패: %v", err)
		} else {
			log.Println("PostgreSQL 연결 성공")
			appCtx.DBPool = pool
			appCtx.ConfigStore = db.NewAlertConfigStore(pool)
			appCtx.HistorySaver = db.NewHistorySaver(pool)
			appCtx.HistoryStore = db.NewAlertHistoryStore(pool)
			appCtx.SNMPDeviceStore = db.NewSNMPDeviceStore(pool)
			appCtx.AssetStore = db.NewAssetStore(pool)
			appCtx.SSLDomainStore = db.NewSSLDomainStore(pool)
			appCtx.LogStore = db.NewLogStore(pool)
			appCtx.LogAnnotationStore = db.NewLogAnnotationStore(pool)
			appCtx.SyntheticStore = db.NewSyntheticStore(pool)
			appCtx.DPMStore = db.NewDPMStore(pool)
			appCtx.AgentStore = db.NewAgentStore(pool)
			appCtx.SpecsStore = db.NewSpecsStore(pool)
			appCtx.LogRuleMatchStore = db.NewLogRuleMatchStore(pool)
			// 룰 엔진 초기화 + 첫 로드
			engine := rules.NewEngine(pool)
			if err := engine.Reload(context.Background()); err != nil {
				log.Printf("로그 룰 로드 실패: %v", err)
			} else {
				log.Printf("로그 룰 %d개 로드 완료", engine.RuleCount())
			}
			appCtx.RulesEngine = engine
		}
	}

	// PostgreSQL 미연결 시 파일 기반 폴백
	if appCtx.ConfigStore == nil {
		log.Println("POSTGRES_DSN 미설정 — 알림 설정/히스토리를 파일로 저장")
		dataDir := getEnv("OWLMON_DATA_DIR", "")
		if dataDir == "" {
			exePath, _ := os.Executable()
			if strings.Contains(filepath.ToSlash(exePath), "/tmp/") || strings.Contains(exePath, `\AppData\Local\Temp\`) {
				dataDir = "."
			} else {
				dataDir = filepath.Dir(exePath)
			}
		}
		appCtx.ConfigStore = alert.NewConfigStore(filepath.Join(dataDir, "alert-config.json"))
	}

	return &appCtx
}

// InitWorkers는 백그라운드 워커들을 초기화하고 시작합니다.
func InitWorkers(appCtx *AppContext, checker *alert.Checker, emailCfg *alert.EmailConfig, prometheusURL string) {
	// 1. 이상탐지 및 알림 체커 시작
	checker.Start(30 * time.Second)

	// 2. SSL 인증서 체크 (6시간 주기)
	checker.SetSSLDomainLister(appCtx.SSLDomainStore)
	checker.StartSSLCheck(6 * time.Hour)

	// 3. Synthetic 모니터링
	if appCtx.SyntheticStore != nil {
		syntheticChecker := synthetic.NewChecker(appCtx.SyntheticStore, appCtx.SyntheticStore)
		syntheticChecker.SetAlertCallbacks(
			func(m synthetic.Monitor, r synthetic.Result, n int) {
				subject := fmt.Sprintf("🚨 [Synthetic] %s 다운 (%d회 연속 실패)", m.Name, n)
				body := fmt.Sprintf("모니터: %s\nURL: %s\n오류: %s\n응답시간: %dms\n시각: %s",
					m.Name, m.URL, r.Error, r.ResponseTimeMs, r.CheckedAt.Format(time.RFC3339))
				checker.SendAlert(m.Name, "synthetic", "critical", subject, body)
			},
			func(m synthetic.Monitor, r synthetic.Result) {
				subject := fmt.Sprintf("✅ [Synthetic] %s 복구", m.Name)
				body := fmt.Sprintf("모니터: %s\nURL: %s\n응답시간: %dms\n시각: %s",
					m.Name, m.URL, r.ResponseTimeMs, r.CheckedAt.Format(time.RFC3339))
				checker.SendAlert(m.Name, "synthetic", "info", subject, body)
			},
		)
		syntheticChecker.Start(context.Background())
		log.Println("Synthetic 모니터링 시작")
	}

	// 4. DPM (DB Performance Monitoring)
	if appCtx.DPMStore != nil {
		masterKey := getEnv("OWLMON_DPM_KEY", "")
		if masterKey != "" {
			dpmCipher, err := dpm.NewCipher(masterKey)
			if err == nil {
				dpmPoller := dpm.NewPoller(appCtx.DPMStore, appCtx.DPMStore, dpmCipher)
				dpmPoller.SetAlertCallback(func(inst dpm.Instance, severity, subject, body string) {
					checker.SendAlert(inst.Name, "dpm", severity, subject, body)
				})
				dpmPoller.Start(context.Background())
				log.Println("DPM 폴러 시작")
			}
		}
	}

	// 5. 월간 보고서 스케줄러
	if emailCfg != nil {
		reporter := report.NewReporter(prometheusURL, emailCfg, appCtx.ConfigStore)
		reporter.Start()
	}

	// 6. SNMP 폴러
	if appCtx.SNMPDeviceStore != nil {
		snmpPoller := snmppkg.NewPoller()
		go func() {
			ticker := time.NewTicker(60 * time.Second)
			defer ticker.Stop()
			for {
				devices, err := appCtx.SNMPDeviceStore.List(context.Background())
				if err == nil {
					for _, dev := range devices {
						go snmpPoller.Poll(dev)
					}
				}
				<-ticker.C
			}
		}()
	}

	// 7. 로그 자동 정리
	if appCtx.LogStore != nil {
		go func() {
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for range ticker.C {
				deleted, err := appCtx.LogStore.Cleanup(context.Background(), 30)
				if err == nil && deleted > 0 {
					log.Printf("오래된 로그 %d건 삭제", deleted)
				}
			}
		}()
	}
}
