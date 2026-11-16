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
	"sync"
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
		// DSN이 있는데 DB 없이 뜨면 단일 계정 모드로 바뀌어 viewer 토큰이 관리자가 되므로, 못 붙으면 종료하고 서비스 재시작에 맡긴다
		pool, err := db.Connect(context.Background(), pgDSN)
		for deadline := time.Now().Add(time.Minute); err != nil && time.Now().Before(deadline); {
			log.Printf("PostgreSQL 연결 실패, 재시도: %v", err)
			time.Sleep(2 * time.Second)
			pool, err = db.Connect(context.Background(), pgDSN)
		}
		if err != nil {
			log.Fatalf("PostgreSQL 연결 실패 — 종료: %v", err)
		}
		log.Println("PostgreSQL 연결 성공")
		appCtx.DBPool = pool
		alertCfgStore := db.NewAlertConfigStore(pool)
		if err := alertCfgStore.EnsureColumns(context.Background()); err != nil {
			log.Printf("⚠️ 알림 경로(웹훅·문자) 설정 칸 준비 실패: %v", err)
		}
		appCtx.ConfigStore = alertCfgStore
		appCtx.HistorySaver = db.NewHistorySaver(pool)
		appCtx.HistoryStore = db.NewAlertHistoryStore(pool)
		appCtx.SNMPDeviceStore = db.NewSNMPDeviceStore(pool)
		if err := appCtx.SNMPDeviceStore.EnsureColumns(context.Background()); err != nil {
			log.Printf("SNMP 장비 칸 준비 실패: %v", err)
		}
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
		// 조회 API·챗봇도 이 테이블의 열을 읽으므로 분석 워커가 꺼져 있어도 스키마를 맞춘다
		if err := loginsight.EnsureTables(context.Background(), pool); err != nil {
			log.Printf("로그 인사이트 테이블 준비 실패: %v", err)
		}
		// 룰 엔진 초기화 + 첫 로드
		engine := rules.NewEngine(pool)
		if err := engine.Reload(context.Background()); err != nil {
			log.Printf("로그 룰 로드 실패: %v", err)
		} else {
			log.Printf("로그 룰 %d개 로드 완료", engine.RuleCount())
		}
		appCtx.RulesEngine = engine
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
		// 장비 대장 전체를 1분에 한 번만 읽음 — 수천 대가 한꺼번에 끊기면 호스트마다 조회하던 것이 알림 발송을 붙잡음
		var locMu sync.Mutex
		var locMap map[string]string
		var locAt time.Time
		checker.SetLocator(func(host string) string {
			locMu.Lock()
			defer locMu.Unlock()
			if locMap == nil || time.Since(locAt) > time.Minute {
				c, cancel := context.WithTimeout(ctx, 3*time.Second)
				list, err := appCtx.AssetStore.List(c)
				cancel()
				if err != nil {
					log.Printf("자산 위치 조회 실패: %v", err) // 위치 없이 메일은 그대로 보냄
					if locMap == nil {
						locMap = map[string]string{} // 실패해도 1분은 다시 묻지 않음 — 장애 폭주 때 알림마다 전체 조회하지 않게
					}
				} else {
					locMap = make(map[string]string, len(list))
					for _, a := range list {
						locMap[a.HostName] = db.LocationText(a.Location, a.Rack, a.RackUnit, a.RackSize)
					}
				}
				locAt = time.Now()
			}
			return locMap[host]
		})
	}

	// 장애 중에 끄거나 지우거나 이름을 바꾼 대상은 2차 통보하지 않게 지금 감시 중인 이름을 알려 줌 (Start 전에 설정)
	if appCtx.SyntheticStore != nil {
		checker.SetWatched("synthetic", func(c context.Context) (map[string]bool, error) {
			list, err := appCtx.SyntheticStore.List(c)
			names := map[string]bool{}
			for _, m := range list {
				names[m.Name] = names[m.Name] || m.Enabled
			}
			return names, err
		})
	}
	if appCtx.DPMStore != nil {
		checker.SetWatched("dpm", func(c context.Context) (map[string]bool, error) {
			list, err := appCtx.DPMStore.List(c)
			names := map[string]bool{}
			for _, x := range list {
				names[x.Name] = names[x.Name] || x.Enabled
			}
			return names, err
		})
	}

	// SSL은 '지금 확인'으로 갱신·삭제가 바로 반영된 최신 점검 결과 기준 — 아직 만료·임박(또는 접속 실패로 모름)인 도메인만 감시 중으로 봄
	if appCtx.SSLDomainStore != nil {
		checker.SetWatched("ssl", func(c context.Context) (map[string]bool, error) {
			list, err := appCtx.SSLDomainStore.List(c)
			if err != nil {
				return nil, err
			}
			registered := map[string]bool{}
			for _, d := range list {
				registered[d.Domain] = true
			}
			names := map[string]bool{}
			for _, r := range checker.SSLChecker.GetResults() {
				if registered[r.Domain] && (r.Status == "expired" || r.Status == "critical" || r.Status == "error") {
					names[r.Domain] = true
				}
			}
			return names, nil
		})
	}

	// 위쪽 장비 장애로 함께 끊긴 장비를 원인 알림에 묶는 데 씀 — 저장된 구성도만 (자동 배치는 추정이라 알림을 막는 근거로 안 씀)
	if appCtx.DBPool != nil {
		topoStore := db.NewTopologyStore(appCtx.DBPool)
		checker.SetTopology(func(c context.Context) (alert.Topology, error) {
			l, err := topoStore.Get(c)
			t := alert.Topology{Parent: map[string]string{}, Pair: map[string]string{}}
			for _, d := range l.Devices {
				if d.Parent != "" {
					t.Parent[d.Name] = d.Parent
				}
				if d.Pair != "" {
					t.Pair[d.Name] = d.Pair
					if _, ok := t.Pair[d.Pair]; !ok {
						t.Pair[d.Pair] = d.Name // 짝은 한쪽에만 적혀 있어도 서로 짝
					}
				}
			}
			return t, err
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
			var agentSeen map[string]bool
			var seenAt time.Time
			ticker := time.NewTicker(30 * time.Second) // 60s → 30s (UI 갱신 체감 ↑)
			defer ticker.Stop()
			for {
				devices, err := appCtx.SNMPDeviceStore.List(ctx)
				if err == nil {
					for _, dev := range devices {
						go appCtx.SNMPPoller.Poll(dev)
					}
					// 화면에만 "응답 없음"이 뜨고 알림이 없던 것 — 같은 판정으로 장애 시작·복구를 알림
					if agentSeen == nil || time.Since(seenAt) > 10*time.Minute {
						if m, err := prom.SNMPAgentSeen(prometheusURL); err == nil {
							agentSeen, seenAt = m, time.Now()
						}
					}
					latest, err := prom.SNMPUpAll(prometheusURL)
					switch {
					case agentSeen == nil:
						// 기동 후 한 번도 못 읽음 — 어느 장비가 에이전트 수집인지 몰라 판정하면 오경보가 쏟아짐
						log.Printf("[SNMP] 응답 판정 건너뜀 — Prometheus에서 에이전트 수집 기록을 아직 못 읽음")
					default:
						if err != nil {
							// 잠시 안 되면 서버 폴링 근거만으로 판정 — 에이전트 수집 장비는 근거 없음으로 보류
							log.Printf("[SNMP] Prometheus 조회 실패 — 서버 폴링 결과로만 판정: %v", err)
						}
						samples := make(map[string]snmppkg.Sample, len(latest))
						for name, x := range latest {
							samples[name] = snmppkg.Sample{Up: x.Up, At: x.At}
						}
						notifySNMP(checker, appCtx.SNMPPoller.Observe(devices, samples, agentSeen, time.Now()))
						for _, d := range devices {
							if !d.ServerReach && appCtx.SNMPPoller.Reached(d) {
								if err := appCtx.SNMPDeviceStore.MarkServerReach(ctx, d.ID); err != nil {
									log.Printf("[SNMP] 서버 도달 기록 실패 (%s): %v", d.Name, err)
								}
							}
						}
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
			// 기동 직후에도 1회 — 하루 안에 재배포가 반복되면 정리가 한 번도 안 돈다
			for {
				start := time.Now()
				deleted, err := appCtx.LogStore.Cleanup(ctx, 30)
				if err != nil {
					log.Printf("오래된 로그 정리 실패: %v", err)
				} else if deleted > 0 {
					log.Printf("오래된 로그 %d건 삭제 (%s)", deleted, time.Since(start).Round(time.Millisecond))
				}
				// 웹 감시·DB 성능 결과도 같은 보존 기간 — 정리하는 곳이 없어 1,000곳이면 하루 144만 행씩 쌓였음
				if appCtx.SyntheticStore != nil {
					if n, err := appCtx.SyntheticStore.Cleanup(ctx, 30); err != nil {
						log.Printf("오래된 웹 감시 결과 정리 실패: %v", err)
					} else if n > 0 {
						log.Printf("오래된 웹 감시 결과 %d건 삭제", n)
					}
				}
				if appCtx.DPMStore != nil {
					if err := appCtx.DPMStore.Cleanup(ctx, 30); err != nil {
						log.Printf("오래된 DB 성능 기록 정리 실패: %v", err)
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

func notifySNMP(checker *alert.Checker, events []snmppkg.Event) {
	var downs, ups, noData []alert.Alert
	for _, ev := range events {
		d := ev.Device
		switch {
		case ev.NoData:
			noData = append(noData, alert.Alert{Host: d.Name, Category: "snmp",
				Subject: fmt.Sprintf("[주의] %s 네트워크 장비 상태 수집 끊김", d.Name),
				Body:    fmt.Sprintf("장비: %s\nIP: %s\n수집 끊김 시작: %s\n\n이 장비를 수집하는 에이전트의 값이 들어오지 않고 서버도 직접 닿지 않아 응답 여부를 모릅니다. 수집 에이전트와 Collector 상태를 확인하세요.", d.Name, d.IP, ev.Since.Format("2006-01-02 15:04:05"))})
		case ev.Removed:
			// 장애 중에 지우거나 이름을 바꾼 장비 — 확인할 화면이 없어 옛 이름의 확인·단계 통보 대기만 정리
			checker.ClearAck(d.Name, "snmp", "critical")
			checker.SetNetDown(d.Name, false)
			checker.ForgetQuiet(d.Name)
		case ev.Down:
			checker.SetNetDown(d.Name, true)
			// 이번 장애가 시작되기 전에 눌린 확인만 풂 — 화면에 '응답 없음'이 뜬 뒤 알림 판정 전에 누른 확인은 살려 둠
			if a, ok := checker.AckInfo(d.Name, "snmp", "critical"); ok && a.At.Before(ev.Since) {
				checker.ClearAck(d.Name, "snmp", "critical")
			}
			body := fmt.Sprintf("장비: %s\nIP: %s\n응답 없음 시작: %s\n\nSNMP 응답이 없습니다. 전원·케이블·장비 상태를 확인하세요.", d.Name, d.IP, ev.Since.Format("2006-01-02 15:04:05"))
			if ev.Conflict {
				body += "\n\n서버 직접 폴링은 응답이 없는데 에이전트 수집 값은 응답 중으로 들어옵니다. 서버와 장비 사이 경로 문제이거나, 에이전트 키로 보낸 값이 실제와 다를 수 있습니다."
			}
			downs = append(downs, alert.Alert{Host: d.Name, Category: "snmp",
				Subject: fmt.Sprintf("[위험] %s 네트워크 장비 응답 없음", d.Name), Body: body})
		default:
			checker.SetNetDown(d.Name, false)
			checker.ClearAck(d.Name, "snmp", "critical")
			ups = append(ups, alert.Alert{Host: d.Name, Category: "snmp",
				Subject: fmt.Sprintf("[복구] %s 네트워크 장비 응답 복구", d.Name),
				Body:    fmt.Sprintf("장비: %s\nIP: %s\n장애 시간: 약 %d분\n\nSNMP 응답이 다시 들어옵니다.", d.Name, d.IP, int(time.Since(ev.Since).Minutes()+0.5))})
		}
	}
	// 상위 스위치 하나가 죽으면 아래 장비가 같은 주기에 모두 끊김 — 많으면 한 통으로
	checker.SendDowns("[위험] 네트워크 장비 %d대 응답 없음", downs)
	checker.SendUps("[복구] 네트워크 장비 %d대 응답 복구", ups)
	checker.SendGroup("warning", "[주의] 네트워크 장비 %d대 상태 수집 끊김", noData)
}
