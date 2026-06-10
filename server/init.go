package main

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seongJae/owlmon/server/alert"
	"github.com/seongJae/owlmon/server/audit"
	"github.com/seongJae/owlmon/server/db"
	"github.com/seongJae/owlmon/server/dpm"
	"github.com/seongJae/owlmon/server/llm"
	"github.com/seongJae/owlmon/server/loginsight"
	"github.com/seongJae/owlmon/server/masking"
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
	SyntheticChecker   *synthetic.Checker
	DPMStore           *db.DPMStore
	DPMPoller          *dpm.Poller
	AuditStore         *audit.Store
	AgentStore         *db.AgentStore
	SpecsStore         *db.SpecsStore
	RulesEngine        *rules.Engine
	LogRuleMatchStore  *db.LogRuleMatchStore
	LogInsightStore    *loginsight.Store // LLM 자동 분석 결과 저장소
	EmailConfig        *alert.EmailConfig
	SNMPPoller         *snmppkg.Poller // 백그라운드 ticker와 HTTP handler가 공유 (한 인스턴스만)
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
			appCtx.AuditStore = audit.NewStore(pool)
			appCtx.LogInsightStore = loginsight.NewStore(pool)
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
// ctx는 앱 수명 컨텍스트 — 서버 종료 시 cancel되어 모든 워커가 함께 멈춘다
// (종전: 워커들이 ctx 없이 돌아 HTTP만 닫히고 체커/폴러는 종료 중에도 계속 동작).
func InitWorkers(ctx context.Context, appCtx *AppContext, checker *alert.Checker, emailCfg *alert.EmailConfig, prometheusURL string) {
	// 1. 이상탐지 및 알림 체커 시작
	checker.Start(ctx, 30*time.Second)

	// 2. SSL 인증서 체크 (6시간 주기)
	checker.SetSSLDomainLister(appCtx.SSLDomainStore)
	checker.StartSSLCheck(ctx, 6*time.Hour)

	// 3. Synthetic 모니터링
	if appCtx.SyntheticStore != nil {
		syntheticChecker := synthetic.NewChecker(appCtx.SyntheticStore, appCtx.SyntheticStore)
		syntheticChecker.SetAlertCallbacks(
			func(m synthetic.Monitor, r synthetic.Result, n int) {
				subject := fmt.Sprintf("[심각] [Synthetic] %s 다운 (%d회 연속 실패)", m.Name, n)
				body := fmt.Sprintf("모니터: %s\nURL: %s\n오류: %s\n응답시간: %dms\n시각: %s",
					m.Name, m.URL, r.Error, r.ResponseTimeMs, r.CheckedAt.Format(time.RFC3339))
				checker.SendAlert(m.Name, "synthetic", "critical", subject, body)
			},
			func(m synthetic.Monitor, r synthetic.Result) {
				subject := fmt.Sprintf("[복구] [Synthetic] %s 복구", m.Name)
				body := fmt.Sprintf("모니터: %s\nURL: %s\n응답시간: %dms\n시각: %s",
					m.Name, m.URL, r.ResponseTimeMs, r.CheckedAt.Format(time.RFC3339))
				checker.SendAlert(m.Name, "synthetic", "info", subject, body)
			},
		)
		syntheticChecker.Start(ctx)
		appCtx.SyntheticChecker = syntheticChecker
		log.Println("Synthetic 모니터링 시작")
	}

	// 4. DPM (DB Performance Monitoring)
	if appCtx.DPMStore != nil {
		masterKey := getEnv("OWLMON_DPM_KEY", "")
		// 키 미설정 시 자동 생성 + 디스크 영속화 (JWT secret 패턴과 동일)
		if masterKey == "" {
			dataDir := getEnv("OWLMON_DATA_DIR", "data")
			_ = os.MkdirAll(dataDir, 0700)
			keyFile := filepath.Join(dataDir, ".dpm.key")
			if b, err := os.ReadFile(keyFile); err == nil {
				masterKey = string(b)
				log.Println("🔑 Persistent DPM key loaded from file")
			} else {
				// 32바이트 임의 키 생성
				rb := make([]byte, 32)
				if _, err := cryptorand.Read(rb); err == nil {
					masterKey = hex.EncodeToString(rb)
					_ = os.WriteFile(keyFile, []byte(masterKey), 0600)
					log.Println("✨ New DPM key generated and saved")
				}
			}
		}
		if masterKey != "" {
			dpmCipher, err := dpm.NewCipher(masterKey)
			if err == nil {
				dpmPoller := dpm.NewPoller(appCtx.DPMStore, appCtx.DPMStore, dpmCipher)
				dpmPoller.SetAlertCallback(func(inst dpm.Instance, severity, subject, body string) {
					checker.SendAlert(inst.Name, "dpm", severity, subject, body)
				})
				dpmPoller.Start(ctx)
				appCtx.DPMPoller = dpmPoller
				log.Println("DPM 폴러 시작")
			} else {
				log.Printf("DPM cipher 생성 실패: %v", err)
			}
		}
	}

	// 5. 월간 보고서 스케줄러
	if emailCfg != nil {
		reporter := report.NewReporter(prometheusURL, emailCfg, appCtx.ConfigStore)
		reporter.Start(ctx)
	}

	// 6. SNMP 폴러 — handler와 백그라운드 ticker가 같은 인스턴스 공유
	if appCtx.SNMPDeviceStore != nil {
		if appCtx.SNMPPoller == nil {
			appCtx.SNMPPoller = snmppkg.NewPoller()
		}
		go func() {
			ticker := time.NewTicker(30 * time.Second) // 60s → 30s (UI 갱신 체감 ↑)
			defer ticker.Stop()
			for {
				devices, err := appCtx.SNMPDeviceStore.List(ctx)
				if err == nil {
					for _, dev := range devices {
						go appCtx.SNMPPoller.Poll(dev)
					}
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	}

	// 7. 로그 자동 정리
	if appCtx.LogStore != nil {
		go func() {
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					deleted, err := appCtx.LogStore.Cleanup(ctx, 30)
					if err == nil && deleted > 0 {
						log.Printf("오래된 로그 %d건 삭제", deleted)
					}
				}
			}
		}()
	}

	// 8. 로그 인사이트 워커 (LLM 자동 분석 — 5분 주기)
	// OWLMON_LOGINSIGHT_ENABLED=true 이고 LLM Provider 활성일 때만 동작.
	if appCtx.LogInsightStore != nil {
		cfg := loginsight.LoadConfig()
		provider := llm.NewProvider()
		analyzer := loginsight.NewAnalyzer(provider, loginsight.WithMasking(func(s string) string {
			return masking.Mask(s, masking.DefaultOptions())
		}))
		worker := loginsight.New(cfg, appCtx.DBPool, analyzer, appCtx.LogInsightStore)
		worker.Start(context.Background())
	}
}
