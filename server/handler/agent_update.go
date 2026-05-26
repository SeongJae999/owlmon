package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// AgentUpdateHandler는 agent self-update를 위한 바이너리 호스팅.
//
// 구조:
//   /app/data/agents/
//     ├─ owlmon-agent-linux-amd64       ← 운영자가 docker cp로 업로드
//     ├─ owlmon-agent-darwin-arm64
//     ├─ owlmon-agent-windows.exe
//     └─ .sha256                          ← 자동 생성 (mtime별)
//
// API:
//   GET /api/agent/latest?os=linux&arch=amd64
//      → { "version": "<sha256-12>", "size": N, "sha256": "...", "modified": "..." }
//   GET /api/agent/binary?os=linux&arch=amd64
//      → 바이너리 스트림 (체크섬 헤더 X-SHA256)
//
// 망분리 환경에서 외부 다운로드 없이 OWLmon 서버 자체가 hub 역할.
type AgentUpdateHandler struct {
	storageDir string // /app/data/agents
}

func NewAgentUpdateHandler(storageDir string) *AgentUpdateHandler {
	_ = os.MkdirAll(storageDir, 0755)
	return &AgentUpdateHandler{storageDir: storageDir}
}

// binaryName은 OS/아키텍처별 표준 파일명.
func binaryName(osName, arch string) string {
	if osName == "windows" {
		return "owlmon-agent-windows.exe"
	}
	if arch == "" {
		arch = "amd64"
	}
	return "owlmon-agent-" + osName + "-" + arch
}

// computeSHA256은 파일 sha256을 계산. 작은 파일(15MB)이라 메모리 부담 X.
func computeSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// GetLatest GET /api/agent/latest?os=linux&arch=amd64
func (h *AgentUpdateHandler) GetLatest(w http.ResponseWriter, r *http.Request) {
	osName := r.URL.Query().Get("os")
	arch := r.URL.Query().Get("arch")
	if osName == "" {
		osName = "linux"
	}
	if arch == "" {
		arch = "amd64"
	}

	path := filepath.Join(h.storageDir, binaryName(osName, arch))
	stat, err := os.Stat(path)
	if err != nil {
		http.Error(w, "agent 바이너리 없음 — 운영자가 docker cp로 업로드 필요", http.StatusNotFound)
		return
	}

	sha, size, err := computeSHA256(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]interface{}{
		"os":       osName,
		"arch":     arch,
		"version":  sha[:12], // 짧은 식별자 (앞 12자)
		"sha256":   sha,
		"size":     size,
		"modified": stat.ModTime().UTC().Format("2006-01-02T15:04:05Z"),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GetBinary GET /api/agent/binary?os=linux&arch=amd64
func (h *AgentUpdateHandler) GetBinary(w http.ResponseWriter, r *http.Request) {
	osName := r.URL.Query().Get("os")
	arch := r.URL.Query().Get("arch")
	if osName == "" {
		osName = "linux"
	}
	if arch == "" {
		arch = "amd64"
	}

	path := filepath.Join(h.storageDir, binaryName(osName, arch))
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "agent 바이너리 없음", http.StatusNotFound)
		return
	}
	defer f.Close()
	stat, _ := f.Stat()

	// 체크섬 헤더 — agent 측 검증용
	sha, _, err := computeSHA256(path)
	if err == nil {
		w.Header().Set("X-SHA256", sha)
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+binaryName(osName, arch))
	if stat != nil {
		w.Header().Set("Content-Length", "")
	}
	_, _ = io.Copy(w, f)
}
