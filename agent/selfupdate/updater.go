// Package selfupdate는 agent가 OWLmon 서버에서 새 바이너리를 자동 다운로드/교체합니다.
//
// 망분리 환경에서 운영자가 일일이 sudo 명령 안 쳐도 되도록 — agent가 알아서.
// 안전장치:
//   • opt-in (config.yaml의 self_update.enabled: true 필요)
//   • sha256 검증 (받은 바이너리 무결성 확인)
//   • 기존 바이너리 백업 (실패 시 복원)
//   • staged: 새 파일 검증 후에만 교체
//
// 동작: 6시간마다 GET /api/agent/latest → 새 버전이면 다운로드 → 교체 → exit
//       (systemd가 자동 재시작 — Restart=always)
package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"time"
)

// Config는 self-update 설정입니다.
type Config struct {
	Enabled       bool          `yaml:"enabled"`
	ServerURL     string        `yaml:"server_url"`     // OWLmon 서버 (logs.server_url과 동일 사용)
	CheckInterval time.Duration `yaml:"check_interval"` // 기본 6h
	BinaryPath    string        `yaml:"binary_path"`    // 기본 /opt/owlmon/owlmon-agent
	AgentKey      string        `yaml:"-"`              // logs.agent_key 재사용
}

// LatestResponse는 /api/agent/latest 응답.
type LatestResponse struct {
	Version  string `json:"version"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
}

// Start는 백그라운드 update loop를 띄웁니다.
func Start(ctx context.Context, cfg Config) {
	if !cfg.Enabled {
		log.Println("[selfupdate] 비활성 (config로 enabled=true 해야 동작)")
		return
	}
	if cfg.CheckInterval == 0 {
		cfg.CheckInterval = 6 * time.Hour
	}
	if cfg.BinaryPath == "" {
		cfg.BinaryPath = "/opt/owlmon/owlmon-agent"
	}

	log.Printf("[selfupdate] 활성 — 서버=%s, 주기=%s", cfg.ServerURL, cfg.CheckInterval)

	go func() {
		// 첫 체크는 5분 대기 (agent 시작 직후 폭주 방지)
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Minute):
		}
		ticker := time.NewTicker(cfg.CheckInterval)
		defer ticker.Stop()
		for {
			if err := checkAndUpdate(cfg); err != nil {
				log.Printf("[selfupdate] 체크 실패: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// checkAndUpdate는 최신 버전 체크 + 필요 시 교체.
func checkAndUpdate(cfg Config) error {
	osName := runtime.GOOS
	arch := runtime.GOARCH

	// 1. 최신 정보 조회
	url := fmt.Sprintf("%s/api/agent/latest?os=%s&arch=%s", cfg.ServerURL, osName, arch)
	req, _ := http.NewRequest("GET", url, nil)
	if cfg.AgentKey != "" {
		req.Header.Set("X-Agent-Key", cfg.AgentKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("latest 조회 실패: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("latest HTTP %d", resp.StatusCode)
	}
	var latest LatestResponse
	if err := json.NewDecoder(resp.Body).Decode(&latest); err != nil {
		return err
	}

	// 2. 현재 바이너리 sha256 — 같으면 skip
	currentSHA, err := fileSHA256(cfg.BinaryPath)
	if err != nil {
		return fmt.Errorf("현재 바이너리 sha 계산 실패: %w", err)
	}
	if currentSHA == latest.SHA256 {
		log.Printf("[selfupdate] 최신 (sha=%s)", currentSHA[:12])
		return nil
	}

	log.Printf("[selfupdate] 새 버전 발견: %s → %s — 다운로드 시작", currentSHA[:12], latest.SHA256[:12])

	// 3. 새 바이너리 다운로드
	dlURL := fmt.Sprintf("%s/api/agent/binary?os=%s&arch=%s", cfg.ServerURL, osName, arch)
	dlReq, _ := http.NewRequest("GET", dlURL, nil)
	if cfg.AgentKey != "" {
		dlReq.Header.Set("X-Agent-Key", cfg.AgentKey)
	}
	dlResp, err := http.DefaultClient.Do(dlReq)
	if err != nil {
		return fmt.Errorf("binary 다운로드 실패: %w", err)
	}
	defer dlResp.Body.Close()
	if dlResp.StatusCode != 200 {
		return fmt.Errorf("binary HTTP %d", dlResp.StatusCode)
	}

	tmpPath := cfg.BinaryPath + ".new"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("임시 파일 생성 실패: %w", err)
	}
	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, hasher), dlResp.Body); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("다운로드 쓰기 실패: %w", err)
	}
	f.Close()
	gotSHA := hex.EncodeToString(hasher.Sum(nil))

	// 4. SHA256 검증 — 손상/위변조 방지
	if gotSHA != latest.SHA256 {
		os.Remove(tmpPath)
		return fmt.Errorf("sha256 불일치 — 다운로드 손상 (기대=%s, 받은=%s)", latest.SHA256, gotSHA)
	}

	// 5. 기존 바이너리 백업 → 교체
	backupPath := cfg.BinaryPath + ".bak"
	_ = os.Rename(cfg.BinaryPath, backupPath) // 실패해도 진행 (백업 없어도 새 거 있으니)
	if err := os.Rename(tmpPath, cfg.BinaryPath); err != nil {
		// rollback
		_ = os.Rename(backupPath, cfg.BinaryPath)
		return fmt.Errorf("교체 실패: %w", err)
	}

	log.Printf("[selfupdate] 교체 완료 — 자체 종료. systemd가 새 버전으로 재시작.")

	// 6. 자체 종료 — systemd Restart=always가 즉시 새 바이너리로 재시작
	// (현재 프로세스는 옛 바이너리 — 종료해야 새 바이너리 로딩)
	time.Sleep(2 * time.Second) // 로그 flush 시간
	os.Exit(0)
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
