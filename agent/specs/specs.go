// Package specs는 호스트 하드웨어/OS 스펙을 1회 수집하고 OWLmon 서버로 전송한다.
// gopsutil로 대부분 처리하고, 디스크 모델/회전속도는 Linux의 /sys/block을 직접 읽는다.
package specs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

// CPUInfo는 CPU 요약 정보를 담는다.
type CPUInfo struct {
	Model   string `json:"model"`
	Cores   int    `json:"cores"`   // 논리 코어 수
	Sockets int    `json:"sockets"` // 물리 소켓 수
}

// DiskInfo는 단일 블록 디바이스 정보를 담는다.
type DiskInfo struct {
	Name       string `json:"name"`        // sda, nvme0n1
	SizeBytes  uint64 `json:"size_bytes"`
	Rotational bool   `json:"rotational"`  // true=HDD, false=SSD
	Model      string `json:"model"`       // "Samsung SSD 990 PRO 2TB"
}

// NetworkInfo는 네트워크 인터페이스 요약이다.
type NetworkInfo struct {
	Name string   `json:"name"`
	MAC  string   `json:"mac"`
	IPv4 []string `json:"ipv4"`
}

// Specs는 한 호스트의 전체 스펙 스냅샷이다.
type Specs struct {
	HostName         string        `json:"host_name"`
	CPU              CPUInfo       `json:"cpu"`
	MemoryTotalBytes uint64        `json:"memory_total_bytes"`
	Disks            []DiskInfo    `json:"disks"`
	Networks         []NetworkInfo `json:"networks"`
	OSPrettyName     string        `json:"os_pretty_name"`
	KernelVersion    string        `json:"kernel_version"`
	Virtualization   string        `json:"virtualization"`
	Arch             string        `json:"arch"`
}

// Collect는 호스트 스펙을 1회 수집한다.
func Collect() (*Specs, error) {
	s := &Specs{Arch: runtime.GOARCH}

	if hi, err := host.Info(); err == nil {
		s.HostName = hi.Hostname
		s.OSPrettyName = formatOS(hi.Platform, hi.PlatformVersion)
		s.KernelVersion = hi.KernelVersion
		s.Virtualization = hi.VirtualizationSystem
		if s.Virtualization == "" {
			s.Virtualization = "none"
		}
	} else {
		s.HostName, _ = os.Hostname()
	}

	// /etc/os-release를 우선 (더 정확한 표시명)
	if v := readOSRelease(); v != "" {
		s.OSPrettyName = v
	}

	if cpus, err := cpu.Info(); err == nil && len(cpus) > 0 {
		s.CPU.Model = strings.TrimSpace(cpus[0].ModelName)
		s.CPU.Sockets = len(cpus)
	}
	if logical, err := cpu.Counts(true); err == nil {
		s.CPU.Cores = logical
	}

	if vm, err := mem.VirtualMemory(); err == nil {
		s.MemoryTotalBytes = vm.Total
	}

	s.Disks = collectDisks()
	s.Networks = collectNetworks()

	return s, nil
}

// Send는 수집한 스펙을 서버 /api/agent/specs로 POST한다.
func (s *Specs) Send(ctx context.Context, serverURL, agentKey string) error {
	body, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("스펙 직렬화 실패: %w", err)
	}

	url := strings.TrimRight(serverURL, "/") + "/api/agent/specs"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if agentKey != "" {
		req.Header.Set("X-Agent-Key", agentKey)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("스펙 전송 실패: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("스펙 전송 응답 %d", resp.StatusCode)
	}
	return nil
}

// readOSRelease는 /etc/os-release의 PRETTY_NAME을 읽는다 (Linux 전용).
func readOSRelease() string {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
		}
	}
	return ""
}

// formatOS는 fallback OS 문자열을 만든다.
func formatOS(platform, version string) string {
	if platform == "" {
		return ""
	}
	if version == "" {
		return platform
	}
	return platform + " " + version
}

// collectDisks는 호스트의 물리 디스크 정보를 수집한다.
// Linux: /sys/block 직접 읽기 (모델/회전속도 정확)
// macOS/Windows/기타: gopsutil disk.Partitions로 마운트된 파티션 수집 (모델/회전속도는 미지원)
func collectDisks() []DiskInfo {
	if runtime.GOOS == "linux" {
		return collectDisksLinux()
	}
	return collectDisksFallback()
}

// collectDisksLinux는 sysfs로 정확한 디스크 정보를 가져온다.
// 가상 디바이스(loop, ram 등)는 제외.
func collectDisksLinux() []DiskInfo {
	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		return nil
	}
	var disks []DiskInfo
	for _, e := range entries {
		name := e.Name()
		// 가상 디바이스 제외
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || strings.HasPrefix(name, "dm-") {
			continue
		}
		base := filepath.Join("/sys/block", name)

		d := DiskInfo{Name: name}
		if v := readUint64(filepath.Join(base, "size")); v > 0 {
			d.SizeBytes = v * 512 // sysfs size는 512바이트 섹터 단위
		}
		if v := readString(filepath.Join(base, "queue/rotational")); v != "" {
			d.Rotational = v == "1"
		}
		if v := readString(filepath.Join(base, "device/model")); v != "" {
			d.Model = v
		}
		disks = append(disks, d)
	}
	return disks
}

// collectDisksFallback은 Linux 외 OS(macOS/Windows/BSD)에서 마운트된 파티션을 수집한다.
// gopsutil disk.Partitions 기반 — 모델명·회전속도는 정확하게 알 수 없어 빈 값/false로 둔다.
func collectDisksFallback() []DiskInfo {
	partitions, err := disk.Partitions(false)
	if err != nil {
		return nil
	}
	var disks []DiskInfo
	seen := map[string]bool{}
	for _, p := range partitions {
		// 가상/임시 파일시스템 제외
		if isVirtualFs(p.Fstype) {
			continue
		}

		usage, err := disk.Usage(p.Mountpoint)
		if err != nil || usage.Total == 0 {
			continue
		}

		// (크기 + fstype) 기준 dedup — APFS 컨테이너처럼 같은 물리 디스크의
		// 여러 논리 볼륨이 보일 때 한 줄로 합침
		key := fmt.Sprintf("%d:%s", usage.Total, p.Fstype)
		if seen[key] {
			continue
		}
		seen[key] = true

		// Model 자리에 파일시스템 표시 (정확한 디스크 모델은 OS별 외부 명령 필요)
		d := DiskInfo{
			Name:       cleanDeviceName(p.Device),
			SizeBytes:  usage.Total,
			Rotational: false, // macOS/Windows는 대부분 SSD — 보수적으로 false (UI 'SSD' 배지)
			Model:      strings.ToUpper(p.Fstype),
		}
		disks = append(disks, d)
	}
	return disks
}

// isVirtualFs는 모니터링 가치 없는 가상 파일시스템을 판별한다.
func isVirtualFs(fs string) bool {
	switch strings.ToLower(fs) {
	case "tmpfs", "devfs", "devtmpfs", "proc", "sysfs", "cgroup", "cgroup2",
		"overlay", "squashfs", "fuse.gvfsd-fuse", "autofs", "tracefs", "debugfs",
		"securityfs", "pstore", "bpf", "configfs", "mqueue", "hugetlbfs":
		return true
	}
	// fuse.* 류 (snap 등)
	if strings.HasPrefix(strings.ToLower(fs), "fuse.") {
		return true
	}
	return false
}

// cleanDeviceName은 OS별로 다른 형식의 디바이스 경로를 짧게 정리한다.
// 예) "/dev/disk1s5s1" → "disk1s5s1",  "C:\\" → "C:"
func cleanDeviceName(s string) string {
	s = strings.TrimPrefix(s, "/dev/")
	s = strings.TrimSuffix(s, "\\")
	return s
}

// collectNetworks는 활성 인터페이스의 MAC/IPv4를 수집한다.
func collectNetworks() []NetworkInfo {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var nets []NetworkInfo
	for _, iface := range ifs {
		// loopback 제외 + 비활성 제외
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		var ipv4 []string
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				ipv4 = append(ipv4, ipnet.String())
			}
		}
		if len(ipv4) == 0 {
			continue // IPv4 없는 인터페이스는 스킵
		}
		nets = append(nets, NetworkInfo{
			Name: iface.Name,
			MAC:  iface.HardwareAddr.String(),
			IPv4: ipv4,
		})
	}
	return nets
}

// readString은 sysfs 파일을 읽고 trim해서 반환.
func readString(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// readUint64는 sysfs 파일을 읽고 uint64로 파싱.
func readUint64(path string) uint64 {
	s := readString(path)
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}
