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
	"github.com/seongJae/owlmon/server/anomaly"
	"github.com/seongJae/owlmon/server/audit"
	"github.com/seongJae/owlmon/server/db"
	"github.com/seongJae/owlmon/server/dpm"
	"github.com/seongJae/owlmon/server/llm"
	"github.com/seongJae/owlmon/server/loginsight"
	"github.com/seongJae/owlmon/server/masking"
	"github.com/seongJae/owlmon/server/prom"
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
	LogInsightWorker   *loginsight.Worker
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
	// 체커 고루틴이 locate를 읽기 전에 설정 (잠금 없는 필드)
	if appCtx.AssetStore != nil {
		checker.SetLocator(func(host string) string {
			c, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			loc, err := appCtx.AssetStore.LocationOf(c, host)
			if err != nil {
				log.Printf("자산 위치 조회 실패 (%s): %v", host, err) // 위치 없이 메일은 그대로 보냄
			}
			return loc
		})
	}

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
				subject := fmt.Sprintf("[위험] [Synthetic] %s 다운 (%d회 연속 실패)", m.Name, n)
				body := fmt.Sprintf("모니터: %s\nURL: %s\n오류: %s\n응답시간: %dms\n시각: %s",
					m.Name, m.URL, r.Error, r.ResponseTimeMs, r.CheckedAt.Format(time.RFC3339))
				checker.SendAlert(m.Name, "synthetic", "critical", subject, body)
			},
			func(m synthetic.Monitor, r synthetic.Result, wasDown bool) {
				checker.ClearAck(m.Name, "synthetic", "critical")
				if !wasDown {
					return // 다운 메일을 보내지 않은 짧은 실패엔 복구 메일도 보내지 않음
				}
				subject := fmt.Sprintf("[복구] [Synthetic] %s 복구", m.Name)
				body := fmt.Sprintf("모니터: %s\nURL: %s\n응답시간: %dms\n시각: %s",
					m.Name, m.URL, r.ResponseTimeMs, r.CheckedAt.Format(time.RFC3339))
				checker.SendAlert(m.Name, "synthetic", "info", subject, body)
			},
		)
		syntheticChecker.SetFailStartCallback(func(m synthetic.Monitor) {
			checker.ClearAck(m.Name, "synthetic", "critical")
		})
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
					key := hex.EncodeToString(rb)
					// 저장 못 한 키로 암호화하면 재시작 후 복호화 불가 → DPM 비활성화
					if err := os.WriteFile(keyFile, []byte(key), 0600); err != nil {
						log.Printf("⚠️ DPM 키 저장 실패 (%v) — DPM 비활성화. OWLMON_DPM_KEY 설정 또는 %s 쓰기 권한 확인", err, dataDir)
					} else {
						masterKey = key
						log.Println("✨ New DPM key generated and saved")
					}
				}
			}
		}
		if masterKey != "" {
			dpmCipher, err := dpm.NewCipher(masterKey)
			if err == nil {
				dpmPoller := dpm.NewPoller(appCtx.DPMStore, appCtx.DPMStore, dpmCipher)
				dpmPoller.SetAlertCallback(func(inst dpm.Instance, severity, subject, body string) {
					// 연결 실패 메일은 장애가 시작될 때 한 번만 나감 — 지난 장애 때 확인한 것이 이번 메일을 막지 않게 먼저 해제
					if severity == "critical" {
						checker.ClearAck(inst.Name, "dpm", "critical")
					}
					checker.SendAlert(inst.Name, "dpm", severity, subject, body)
				})
				// 복구 때도 풀어 둬야 다음 장애 직후 화면이 지난 확인을 "처리 중"으로 보이지 않음
				dpmPoller.SetRecoverCallback(func(inst dpm.Instance) {
					checker.ClearAck(inst.Name, "dpm", "critical")
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
			if err := appCtx.LogStore.EnsureIndexes(ctx); err != nil {
				log.Printf("로그 인덱스 생성 실패: %v", err)
			}
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
		if op, ok := provider.(*llm.OllamaProvider); ok {
			op.JSON = true // 분류 전용 인스턴스 — 챗봇·설명은 router.go의 별도 인스턴스
		}
		analyzer := loginsight.NewAnalyzer(provider,
			loginsight.WithMasking(func(s string) string {
				return masking.Mask(s, masking.DefaultOptions())
			}),
			// 분석 시점 호스트 상황(메트릭·이상탐지)을 프롬프트에 주입 → 상관 분석
			loginsight.WithContext(func(_ context.Context, host string) string {
				return buildHostContext(prometheusURL, checker.Detector, host)
			}),
		)
		worker := loginsight.New(cfg, appCtx.DBPool, analyzer, appCtx.LogInsightStore)
		worker.Notify = func(severity, subject, body string) {
			checker.SendAlert("owlmon", "monitoring", severity, subject, body)
		}
		worker.Start(ctx)
		appCtx.LogInsightWorker = worker
	}
}

// buildHostContext — 로그 분석 시점의 호스트 상황(메트릭·이상탐지)을 요약한다.
// loginsight 프롬프트에 "[현재 호스트 상태]" 블록으로 붙어 상관 분석을 유도한다.
// 데이터가 없으면 "데이터 없음"으로 표기 (LLM이 과대평가하지 않게).
func buildHostContext(prometheusURL string, detector *anomaly.Detector, host string) string {
	q := func(tmpl string) (float64, bool) {
		rs, err := prom.Query(prometheusURL, fmt.Sprintf(tmpl, host))
		if err != nil || len(rs) == 0 {
			return 0, false
		}
		return rs[0].Value, true
	}
	cpu, okC := q(`system_cpu_usage_percent{host_name=%q}`)
	mem, okM := q(`system_memory_usage_percent{host_name=%q}`)
	dsk, okD := q(`max(system_disk_usage_percent{host_name=%q})`)

	var b strings.Builder
	b.WriteString("[현재 호스트 상태]\n")
	if okC || okM || okD {
		b.WriteString("- 사용률:")
		if okC {
			fmt.Fprintf(&b, " CPU %.0f%%", cpu)
		}
		if okM {
			fmt.Fprintf(&b, " · 메모리 %.0f%%", mem)
		}
		if okD {
			fmt.Fprintf(&b, " · 디스크(최대) %.0f%%", dsk)
		}
		b.WriteByte('\n')
	} else {
		b.WriteString("- 사용률: 데이터 없음\n")
	}
	if detector != nil {
		if ans := detector.GetHostAnomalies(host); len(ans) > 0 {
			b.WriteString("- 이상탐지:")
			for _, a := range ans {
				fmt.Fprintf(&b, " %s(현재 %.0f·평소 %.0f·Z=%.1f)", a.Metric, a.Value, a.Mean, a.ZScore)
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}
